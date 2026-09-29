package importer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	driver "github.com/go-sql-driver/mysql"

	"ampls/internal/api"
	"ampls/internal/external"
	"ampls/internal/paths"
	"ampls/internal/services"
)

// server is a MySQL/MariaDB server databases are read from (or written to).
type server struct {
	Host, User, Password string
	Port                 int
	BinDir               string // client tools matching the server ("" = AMPLS's)
	Desc                 string // for messages
	stop                 func() error
}

func (s *server) close() error {
	if s == nil || s.stop == nil {
		return nil
	}
	err := s.stop()
	s.stop = nil
	return err
}

func (s *server) open() (*sql.DB, error) {
	cfg := driver.NewConfig()
	cfg.User = s.User
	cfg.Passwd = s.Password
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	cfg.Timeout = 5 * time.Second
	cfg.ReadTimeout = 60 * time.Second
	cfg.WriteTimeout = 60 * time.Second
	cfg.AllowNativePasswords = true
	c, err := driver.NewConnector(cfg)
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(c)
	db.SetMaxOpenConns(1)
	return db, nil
}

func (s *server) with(ctx context.Context, fn func(ctx context.Context, db *sql.DB) error) error {
	db, err := s.open()
	if err != nil {
		return fmt.Errorf("%s: %w", s.Desc, err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return fn(ctx, db)
}

// version returns VERSION() and whether it is MariaDB.
func (s *server) version(ctx context.Context) (string, bool, error) {
	var v string
	err := s.with(ctx, func(ctx context.Context, db *sql.DB) error {
		return db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&v)
	})
	return v, strings.Contains(strings.ToLower(v), "mariadb"), err
}

// databases lists user databases (system schemas, phpmyadmin and test excluded).
func (s *server) databases(ctx context.Context) ([]api.Database, error) {
	var out []api.Database
	err := s.with(ctx, func(ctx context.Context, db *sql.DB) error {
		rows, err := db.QueryContext(ctx, `SELECT s.SCHEMA_NAME, COUNT(t.TABLE_NAME),
  COALESCE(SUM(t.DATA_LENGTH + t.INDEX_LENGTH), 0)
FROM information_schema.SCHEMATA s
LEFT JOIN information_schema.TABLES t ON t.TABLE_SCHEMA = s.SCHEMA_NAME
GROUP BY s.SCHEMA_NAME ORDER BY s.SCHEMA_NAME`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d api.Database
			var size sql.NullFloat64
			if err := rows.Scan(&d.Name, &d.Tables, &size); err != nil {
				return err
			}
			if external.SystemDatabases[strings.ToLower(d.Name)] {
				continue
			}
			d.SizeBytes = int64(size.Float64)
			out = append(out, d)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("%s: list databases: %w", s.Desc, err)
	}
	return out, nil
}

// charset returns a database's default character set and collation.
func (s *server) charset(ctx context.Context, name string) (cs, coll string) {
	_ = s.with(ctx, func(ctx context.Context, db *sql.DB) error {
		return db.QueryRowContext(ctx, "SELECT DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?", name).Scan(&cs, &coll)
	})
	return cs, coll
}

// fromRequest builds a server from explicit credentials.
func fromRequest(src *api.MySQLSource, desc string) *server {
	s := &server{Host: src.Host, Port: src.Port, User: src.User, Password: src.Password, Desc: desc}
	if s.Host == "" || strings.EqualFold(s.Host, "localhost") {
		s.Host = "127.0.0.1"
	}
	if s.Port == 0 {
		s.Port = 3306
	}
	if s.User == "" {
		s.User = "root"
	}
	return s
}

// errNoSource means no server could be found or started for kind.
var errNoSource = errors.New("no MySQL server available")

// resolveSource finds the server to read databases from. When start is true
// and the environment's server is stopped (XAMPP), a temporary one is started
// on a free port; the caller must close() it.
func resolveSource(ctx context.Context, kind string, src *api.MySQLSource, d Deps, start bool) (*server, error) {
	var s *server
	switch {
	case src != nil && (src.Port != 0 || kind == api.EnvMySQL):
		s = fromRequest(src, kind+" MySQL")
		if c, ok := external.PortOwner(s.Port); ok && c.Path != "" && isLoopback(s.Host) {
			if c.Env == external.EnvAMPLS {
				return nil, fmt.Errorf("port %d is AMPLS's own MySQL; enter the other server's port", s.Port)
			}
			s.BinDir = toolDirFor(c.Path)
		}
	case kind == api.EnvMySQL:
		return nil, fmt.Errorf("mysql import: connection details are required")
	default:
		root, ok := external.Root(kind)
		if !ok {
			return nil, fmt.Errorf("%s is not installed", kind)
		}
		if p, ok := external.RunningMySQL(kind); ok {
			s = &server{Host: "127.0.0.1", Port: p.Ports[0], User: "root", Desc: kind + " MySQL", BinDir: toolDirFor(p.Exe)}
			if src != nil {
				if src.User != "" {
					s.User = src.User
				}
				s.Password = src.Password
			}
			break
		}
		if kind != api.EnvXAMPP || !start {
			return nil, errNoSource
		}
		t, err := startXAMPPServer(ctx, root, src)
		if err != nil {
			return nil, err
		}
		s = t
	}
	if s.BinDir == "" && kind != api.EnvMySQL {
		if root, ok := external.Root(kind); ok {
			if dirs := external.MySQLBinDirs(kind, root); len(dirs) > 0 {
				s.BinDir = dirs[0]
			}
		}
	}
	if isLoopback(s.Host) && s.Port == d.MySQLPort {
		if c, ok := external.PortOwner(s.Port); ok && c.Env == external.EnvAMPLS {
			s.close()
			return nil, fmt.Errorf("port %d is AMPLS's own MySQL", s.Port)
		}
	}
	return s, nil
}

func isLoopback(h string) bool {
	return h == "127.0.0.1" || h == "::1" || strings.EqualFold(h, "localhost")
}

// toolDirFor returns the directory of a server executable if it has a mysqldump next to it.
func toolDirFor(serverExe string) string {
	if serverExe == "" {
		return ""
	}
	dir := filepath.Dir(serverExe)
	if fileExists(filepath.Join(dir, paths.Exe("mysqldump"))) || fileExists(filepath.Join(dir, paths.Exe("mariadb-dump"))) {
		return dir
	}
	return ""
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// startXAMPPServer runs XAMPP's own mysqld (MariaDB) on a free loopback port
// for the duration of the import. XAMPP's my.ini is used when present so
// InnoDB settings (log file size etc.) match its data files — a mismatch would
// make MariaDB rewrite XAMPP's redo logs — and the command line overrides the
// port, bind address, pid file and error log so nothing of XAMPP's is touched.
func startXAMPPServer(ctx context.Context, root string, src *api.MySQLSource) (*server, error) {
	bin := filepath.Join(root, "mysql", "bin")
	mysqld := filepath.Join(bin, paths.Exe("mysqld"))
	if !fileExists(mysqld) {
		return nil, fmt.Errorf("xampp: %s not found", mysqld)
	}
	port, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("xampp: %w", err)
	}
	if err := os.MkdirAll(paths.TmpDir(), 0o755); err != nil {
		return nil, fmt.Errorf("xampp: %w", err)
	}
	logFile := filepath.Join(paths.TmpDir(), "xampp-mysqld-import.log")
	pidFile := filepath.Join(paths.TmpDir(), "xampp-mysqld-import.pid")
	_ = os.Remove(logFile)
	var args []string
	if ini := filepath.Join(bin, "my.ini"); fileExists(ini) {
		args = append(args, "--defaults-file="+ini)
	} else {
		args = append(args, "--no-defaults")
	}
	args = append(args,
		"--basedir="+filepath.Join(root, "mysql"),
		"--datadir="+filepath.Join(root, "mysql", "data"),
		// XAMPP's my.ini holds absolute paths; keep them right if XAMPP was moved.
		"--innodb-data-home-dir="+filepath.Join(root, "mysql", "data"),
		"--innodb-log-group-home-dir="+filepath.Join(root, "mysql", "data"),
		"--plugin-dir="+filepath.Join(root, "mysql", "lib", "plugin"),
		"--port="+strconv.Itoa(port),
		"--bind-address=127.0.0.1",
		"--skip-networking=0",
		"--log-error="+logFile,
		"--pid-file="+pidFile,
	)
	cmd := exec.Command(mysqld, args...)
	cmd.Dir = bin
	services.Hide(cmd)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("xampp: start mysqld: %w", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	s := &server{Host: "127.0.0.1", Port: port, User: "root", BinDir: bin, Desc: "XAMPP MySQL (temporary)"}
	if src != nil {
		if src.User != "" {
			s.User = src.User
		}
		s.Password = src.Password
	}
	s.stop = func() error {
		select {
		case <-exited:
			return nil
		default:
		}
		admin := filepath.Join(bin, paths.Exe("mysqladmin"))
		if fileExists(admin) {
			if cnf, err := clientCnf(s); err == nil {
				sctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				c := exec.CommandContext(sctx, admin, "--defaults-file="+cnf, "--protocol=TCP", "shutdown")
				services.Hide(c)
				_ = c.Run()
				cancel()
				os.Remove(cnf)
			}
		}
		select {
		case <-exited:
			return nil
		case <-time.After(30 * time.Second):
		}
		_ = cmd.Process.Kill() // only this temporary process
		<-exited
		return nil
	}

	deadline := time.Now().Add(90 * time.Second)
	for {
		select {
		case err := <-exited:
			exited <- err
			return nil, fmt.Errorf("xampp: mysqld exited during start-up: %s", tailFile(logFile, 5))
		case <-ctx.Done():
			s.close()
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
		if c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 300*time.Millisecond); err == nil {
			c.Close()
			if _, _, err := s.version(ctx); err == nil {
				return s, nil
			} else if strings.Contains(err.Error(), "Access denied") {
				s.close()
				return nil, fmt.Errorf("xampp: MySQL root login failed (XAMPP's root has a password?) — enter XAMPP's MySQL credentials: %w", err)
			}
		}
		if time.Now().After(deadline) {
			s.close()
			return nil, fmt.Errorf("xampp: mysqld did not become ready: %s", tailFile(logFile, 5))
		}
	}
}

// clientCnf writes a [client] option file (password never on the command
// line), like internal/mysql does. Caller removes it.
func clientCnf(s *server) (string, error) {
	if err := os.MkdirAll(paths.TmpDir(), 0o755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(paths.TmpDir(), "import-*.cnf")
	if err != nil {
		return "", err
	}
	esc := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	fmt.Fprintf(f, "[client]\nuser=\"%s\"\npassword=\"%s\"\nhost=%s\nport=%d\n",
		esc.Replace(s.User), esc.Replace(s.Password), s.Host, s.Port)
	return f.Name(), f.Close()
}

func tailFile(file string, n int) string {
	b, err := os.ReadFile(file)
	if err != nil {
		return "(no log)"
	}
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(string(b), "\r", "")), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}
