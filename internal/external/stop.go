package external

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"apnoro/internal/api"
	"apnoro/internal/paths"
	"apnoro/internal/services"
)

// EnvRoots returns every directory whose processes belong to kind
// (install/config roots, plus Herd's program folder).
func EnvRoots(kind string) []string {
	r := Roots(kind)
	if kind == api.EnvHerd {
		r = append(r, HerdInstallDirs()...)
	}
	return r
}

func isMySQLServer(p Proc) bool {
	switch strings.ToLower(p.Name) {
	case "mysqld.exe", "mariadbd.exe", "mysqld", "mariadbd":
		return true
	}
	return false
}

// RunningMySQL returns the first MySQL/MariaDB server of kind that listens on a port.
func RunningMySQL(kind string) (Proc, bool) {
	for _, p := range EnvProcesses(Processes(), EnvRoots(kind)) {
		if isMySQLServer(p) && len(p.Ports) > 0 {
			return p, true
		}
	}
	return Proc{}, false
}

// Stop stops the servers of an environment. Only processes whose executable
// lives inside that environment's directories are touched; GUI/tray apps are
// left running.
//
// XAMPP's apache_stop.bat / mysql_stop.bat are deliberately NOT used: the first
// kills every httpd.exe on the machine by image name (Apnoro's included) and the
// second shuts down whatever answers on the default port.
func Stop(kind string) error {
	roots := EnvRoots(kind)
	if len(roots) == 0 {
		return fmt.Errorf("stop %s: not installed", kind)
	}
	if kind == api.EnvHerd {
		herdCLIStop() // best effort; needs the Herd app running
	}
	procs := serverProcs(roots)
	if len(procs) == 0 {
		return nil
	}
	// Graceful MySQL shutdown first (avoids crash recovery next start).
	for _, p := range procs {
		if !isMySQLServer(p) || len(p.Ports) == 0 {
			continue
		}
		admin := filepath.Join(filepath.Dir(p.Exe), paths.Exe("mysqladmin"))
		if !isFile(admin) {
			admin = filepath.Join(filepath.Dir(p.Exe), paths.Exe("mariadb-admin"))
		}
		if isFile(admin) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			cmd := exec.CommandContext(ctx, admin, "--no-defaults", "--protocol=TCP", "-h", "127.0.0.1",
				"-P", strconv.Itoa(p.Ports[0]), "-u", "root", "--connect-timeout=3", "shutdown")
			services.Hide(cmd)
			_ = cmd.Run() // fails when root has a password; we fall back to terminate
			cancel()
			waitExit(p.PID, 20*time.Second)
		}
	}
	// Terminate what is left: parents before children so Apache's control
	// process cannot respawn its worker.
	procs = serverProcs(roots)
	pids := map[int]bool{}
	for _, p := range procs {
		pids[p.PID] = true
	}
	sort.SliceStable(procs, func(i, j int) bool { return !pids[procs[i].PPID] && pids[procs[j].PPID] })
	var denied []string
	var errs []error
	for _, p := range procs {
		if !pidAlive(p.PID) {
			continue
		}
		if err := killPID(p.PID); err != nil {
			if errors.Is(err, ErrAccessDenied) {
				denied = append(denied, p.Name)
				continue
			}
			errs = append(errs, fmt.Errorf("%s (pid %d): %w", p.Name, p.PID, err))
		}
	}
	if len(denied) > 0 {
		errs = append(errs, fmt.Errorf("%s: access denied — they probably run as Windows services or as administrator; stop them from the %s control panel or services.msc",
			strings.Join(uniq(denied), ", "), kind))
	}
	if len(errs) > 0 {
		return fmt.Errorf("stop %s: %w", kind, errors.Join(errs...))
	}
	return nil
}

func serverProcs(roots []string) []Proc {
	var out []Proc
	for _, p := range EnvProcesses(Processes(), roots) {
		if IsServer(p) && p.PID != os.Getpid() {
			out = append(out, p)
		}
	}
	return out
}

func waitExit(pid int, d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) && pidAlive(pid) {
		time.Sleep(250 * time.Millisecond)
	}
}

// herdCLIStop runs `herd stop` with Herd's own PHP (not via herd.bat, which
// resolves `php` from PATH and could pick Apnoro's shim).
func herdCLIStop() {
	root, ok := Root(api.EnvHerd)
	if !ok {
		return
	}
	h, _ := ReadHerd(root)
	phar := filepath.Join(root, "bin", "herd.phar")
	if len(h.PHP) == 0 || !isFile(phar) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(h.PHP[0].Dir, paths.Exe("php")), phar, "stop")
	cmd.Dir = filepath.Join(root, "bin")
	services.Hide(cmd)
	_ = cmd.Run()
}

func uniq(s []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range s {
		if !seen[strings.ToLower(v)] {
			seen[strings.ToLower(v)] = true
			out = append(out, v)
		}
	}
	return out
}

// MySQLBinDirs returns directories holding an environment's MySQL/MariaDB
// client tools (mysqldump, mysqladmin), newest first where versioned.
func MySQLBinDirs(kind, root string) []string {
	var out []string
	add := func(dir string) {
		if isFile(filepath.Join(dir, paths.Exe("mysqldump"))) || isFile(filepath.Join(dir, paths.Exe("mariadb-dump"))) {
			out = append(out, dir)
		}
	}
	versioned := func(parent string) {
		ents, _ := os.ReadDir(parent)
		var names []string
		for _, e := range ents {
			if e.IsDir() {
				names = append(names, e.Name())
			}
		}
		sort.Slice(names, func(i, j int) bool { return names[i] > names[j] })
		for _, n := range names {
			add(filepath.Join(parent, n, "bin"))
		}
	}
	switch kind {
	case api.EnvXAMPP:
		add(filepath.Join(root, "mysql", "bin"))
	case api.EnvLaragon:
		versioned(filepath.Join(root, "bin", "mysql"))
		versioned(filepath.Join(root, "bin", "mariadb"))
	case api.EnvWAMP:
		versioned(filepath.Join(root, "bin", "mysql"))
		versioned(filepath.Join(root, "bin", "mariadb"))
	case api.EnvHerd:
		versioned(filepath.Join(root, "bin"))
		versioned(filepath.Join(root, "services"))
	}
	return out
}
