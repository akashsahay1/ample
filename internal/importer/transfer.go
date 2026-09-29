package importer

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ampls/internal/paths"
	"ampls/internal/services"
)

func quoteIdent(name string) string { return "`" + strings.ReplaceAll(name, "`", "``") + "`" }

// dumpTool returns the mysqldump to use for s: the source's own when known,
// else AMPLS's.
func dumpTool(s *server) string {
	if s.BinDir != "" {
		for _, n := range []string{"mysqldump", "mariadb-dump"} {
			if p := filepath.Join(s.BinDir, paths.Exe(n)); fileExists(p) {
				return p
			}
		}
	}
	return filepath.Join(paths.MySQLDir(), "bin", paths.Exe("mysqldump"))
}

var helpCache sync.Map // tool path -> lower-cased `--help` output

func toolHelp(tool string) string {
	if v, ok := helpCache.Load(tool); ok {
		return v.(string)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, "--no-defaults", "--help")
	services.Hide(cmd)
	out, _ := cmd.CombinedOutput()
	h := strings.ToLower(string(out))
	helpCache.Store(tool, h)
	return h
}

// toolFlavor reports whether a mysqldump is MariaDB's and which optional flags it knows.
func toolFlavor(tool string) (maria bool, supports func(flag string) bool) {
	h := toolHelp(tool)
	maria = strings.Contains(h, "mariadb")
	return maria, func(flag string) bool { return strings.Contains(h, flag) }
}

// DumpArgs builds mysqldump's arguments (after --defaults-file). supports
// reports whether the tool knows an option (nil = assume yes).
func DumpArgs(database string, toolMaria, serverMaria bool, supports func(flag string) bool) []string {
	if supports == nil {
		supports = func(string) bool { return true }
	}
	args := []string{"--protocol=TCP", "--single-transaction", "--routines", "--triggers", "--events",
		"--default-character-set=utf8mb4", "--skip-add-locks", "--no-tablespaces", "--hex-blob"}
	if !toolMaria {
		if serverMaria {
			if supports("column-statistics") {
				args = append(args, "--column-statistics=0") // MySQL 8 mysqldump vs MariaDB
			}
		} else if supports("set-gtid-purged") {
			args = append(args, "--set-gtid-purged=OFF")
		}
	}
	return append(args, database)
}

// MapCollation converts a MariaDB collation to a MySQL 8.4 one ("" stays "").
func MapCollation(c string) string {
	out, _ := FilterLine([]byte(c), true)
	return string(out)
}

// prepareTarget creates (or recreates when overwrite) the AMPLS database with
// the source's default charset/collation. existed reports it was already there.
func prepareTarget(ctx context.Context, dst *server, name, cs, coll string, overwrite bool) (existed bool, err error) {
	err = dst.with(ctx, func(ctx context.Context, db *sql.DB) error {
		var n int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?", name).Scan(&n); err != nil {
			return err
		}
		existed = n > 0
		if existed && !overwrite {
			return nil
		}
		if existed {
			if _, err := db.ExecContext(ctx, "DROP DATABASE "+quoteIdent(name)); err != nil {
				return err
			}
		}
		stmt := "CREATE DATABASE " + quoteIdent(name)
		if cs != "" && coll != "" {
			if _, err := db.ExecContext(ctx, stmt+" CHARACTER SET "+cs+" COLLATE "+MapCollation(coll)); err == nil {
				return nil
			}
		}
		_, err := db.ExecContext(ctx, stmt+" CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
		return err
	})
	if err != nil {
		return existed, fmt.Errorf("AMPLS MySQL: prepare %s: %w", name, err)
	}
	return existed, nil
}

type countingReader struct {
	r io.Reader
	n *atomic.Int64
}

func (c countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n.Add(int64(n))
	return n, err
}

// copyDatabase streams mysqldump(src) | FilterDump | mysql(dst).
// onBytes is called periodically with the number of dump bytes processed.
func copyDatabase(parent context.Context, src, dst *server, name string, serverMaria bool, onBytes func(int64)) error {
	tool := dumpTool(src)
	if !fileExists(tool) {
		return fmt.Errorf("mysqldump not found (%s)", tool)
	}
	mysqlExe := filepath.Join(paths.MySQLDir(), "bin", paths.Exe("mysql"))
	if !fileExists(mysqlExe) {
		return fmt.Errorf("AMPLS mysql client not found (%s)", mysqlExe)
	}
	srcCnf, err := clientCnf(src)
	if err != nil {
		return err
	}
	defer os.Remove(srcCnf)
	dstCnf, err := clientCnf(dst)
	if err != nil {
		return err
	}
	defer os.Remove(dstCnf)

	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	toolMaria, supports := toolFlavor(tool)
	dump := exec.CommandContext(ctx, tool, append([]string{"--defaults-file=" + srcCnf}, DumpArgs(name, toolMaria, serverMaria, supports)...)...)
	var dumpErr, impErr bytes.Buffer
	dump.Stderr = &limited{b: &dumpErr}
	services.Hide(dump)
	dumpOut, err := dump.StdoutPipe()
	if err != nil {
		return err
	}
	imp := exec.CommandContext(ctx, mysqlExe, "--defaults-file="+dstCnf, "--protocol=TCP",
		"--default-character-set=utf8mb4", "--max-allowed-packet=1G", name)
	impW := &limited{b: &impErr}
	imp.Stdout = impW
	imp.Stderr = impW
	services.Hide(imp)
	impIn, err := imp.StdinPipe()
	if err != nil {
		return err
	}
	if err := imp.Start(); err != nil {
		return fmt.Errorf("start mysql: %w", err)
	}
	if err := dump.Start(); err != nil {
		impIn.Close()
		imp.Wait()
		return fmt.Errorf("start mysqldump: %w", err)
	}

	var counter atomic.Int64
	done := make(chan struct{})
	if onBytes != nil {
		go func() {
			t := time.NewTicker(500 * time.Millisecond)
			defer t.Stop()
			for {
				select {
				case <-done:
					return
				case <-t.C:
					onBytes(counter.Load())
				}
			}
		}()
	}
	filterErr := FilterDump(countingReader{r: dumpOut, n: &counter}, impIn, serverMaria || toolMaria)
	close(done)
	impIn.Close()
	if filterErr != nil {
		// mysql died (broken pipe): stop the dump too.
		cancel()
		io.Copy(io.Discard, dumpOut)
	}
	dErr := dump.Wait()
	iErr := imp.Wait()
	switch {
	case parent.Err() != nil:
		return parent.Err()
	case iErr != nil:
		return fmt.Errorf("import into AMPLS failed: %v: %s", iErr, lastLines(impErr.String(), 4))
	case dErr != nil:
		return fmt.Errorf("mysqldump failed: %v: %s", dErr, lastLines(dumpErr.String(), 4))
	case filterErr != nil:
		return fmt.Errorf("transfer: %w", filterErr)
	}
	return nil
}

// limited keeps the first 64 KiB written (stderr capture).
type limited struct{ b *bytes.Buffer }

func (l *limited) Write(p []byte) (int, error) {
	if room := 64*1024 - l.b.Len(); room > 0 {
		if len(p) > room {
			l.b.Write(p[:room])
		} else {
			l.b.Write(p)
		}
	}
	return len(p), nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(s, "\r", "")), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}
