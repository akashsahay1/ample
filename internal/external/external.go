// Package external detects other local PHP stacks installed on this machine
// (XAMPP, Laravel Herd, Laragon, WampServer), tells which of them hold the ports
// Apnoro needs, stops them on request and reads their site configuration so the
// importer can bring projects over.
//
// Everything here is read-only except Stop, which only ever terminates processes
// whose executable lives inside the environment's own directory.
package external

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"apnoro/internal/api"
	"apnoro/internal/paths"
	"apnoro/internal/services"
)

// EnvApnoro is returned by Classify/PHPOnPath for Apnoro's own executables.
const EnvApnoro = "apnoro"

// Kinds lists the environments Detect looks for, in display order.
var Kinds = []string{api.EnvXAMPP, api.EnvHerd, api.EnvLaragon, api.EnvWAMP}

// Proc is a running process.
type Proc struct {
	PID   int
	PPID  int
	Name  string // image name, "httpd.exe"
	Exe   string // full path when it could be queried ("" otherwise)
	Ports []int  // TCP ports it listens on
}

// guiNames are control panels / tray apps: they are never "servers" and Stop
// leaves them alone (the user closes them).
var guiNames = map[string]bool{
	"xampp-control.exe": true, "herd.exe": true, "herdhelper.exe": true, "elevate.exe": true,
	"laragon.exe": true, "wampmanager.exe": true, "aestan_tray_menu.exe": true,
	"nvm.exe": true, "uninstall herd.exe": true,
}

// IsServer reports whether p looks like a server process of a dev stack
// (anything that is not the stack's GUI/tray app).
func IsServer(p Proc) bool { return !guiNames[strings.ToLower(p.Name)] }

// Roots returns every install/config root found for kind (existing directories).
func Roots(kind string) []string { return dedupPaths(candidateRoots(kind)) }

// Root returns the first root found for kind.
func Root(kind string) (string, bool) {
	r := Roots(kind)
	if len(r) == 0 {
		return "", false
	}
	return r[0], true
}

// HerdInstallDirs are the locations of the Herd desktop app (nginx, services).
func HerdInstallDirs() []string { return dedupPaths(herdInstallCandidates()) }

// allRoots maps kind -> roots, including Herd's program directory.
func allRoots() map[string][]string {
	m := map[string][]string{}
	for _, k := range Kinds {
		m[k] = Roots(k)
	}
	m[api.EnvHerd] = append(m[api.EnvHerd], HerdInstallDirs()...)
	return m
}

// Detect finds every supported environment installed on this machine.
func Detect() ([]api.ExternalEnv, error) {
	procs := Processes()
	phpPath, phpEnv := PHPOnPath()
	var out []api.ExternalEnv
	for _, k := range Kinds {
		for _, root := range Roots(k) {
			e := describe(k, root, procs)
			if phpPath != "" && phpEnv == k && Under(phpPath, root) {
				e.OnPath = true
				e.Notes = append(e.Notes, "Its php ("+phpPath+") is first on PATH, so `php` in a terminal runs "+e.Name+"'s PHP instead of Apnoro's.")
			}
			out = append(out, e)
		}
	}
	return out, nil
}

// describe builds the ExternalEnv for one root. procs may be nil (tests).
func describe(kind, root string, procs []Proc) api.ExternalEnv {
	e := api.ExternalEnv{Kind: kind, Path: root, CanImport: true}
	roots := []string{root}
	switch kind {
	case api.EnvXAMPP:
		x := ReadXAMPP(root)
		e.Name = "XAMPP"
		if x.Version != "" {
			e.Name += " " + x.Version
		}
		e.PHP = x.PHPVersion
		e.Databases = len(DataDirDatabases(filepath.Join(root, "mysql", "data"))) > 0
		if x.MariaDBVersion != "" {
			e.Notes = append(e.Notes, "Ships MariaDB "+x.MariaDBVersion+"; databases are converted for MySQL 8.4 during import.")
		}
	case api.EnvHerd:
		h, _ := ReadHerd(root)
		e.Name = "Laravel Herd"
		roots = append(roots, HerdInstallDirs()...)
		if p, ok := h.PHPFull(h.ActivePHP); ok {
			e.PHP = p
		} else {
			e.PHP = h.ActivePHP
		}
		if len(h.PHP) > 0 {
			var vs []string
			for _, p := range h.PHP {
				vs = append(vs, p.Full)
			}
			e.Notes = append(e.Notes, "Bundled PHP: "+strings.Join(vs, ", "))
		}
		for _, s := range h.Services {
			if s == "mysql" || s == "mariadb" {
				e.Databases = true
			}
		}
		if len(h.Services) > 0 {
			e.Notes = append(e.Notes, "Herd Pro services configured: "+strings.Join(h.Services, ", "))
		}
	case api.EnvLaragon:
		e.Name = "Laragon"
		l := ReadLaragon(root)
		e.PHP = l.newestPHP()
		e.Databases = l.hasData
	case api.EnvWAMP:
		e.Name = "WampServer"
		w := ReadWAMP(root)
		e.PHP = w.newestPHP()
		e.Databases = w.hasData
	}
	if s, _, _, err := SourceSites(kind, root); err == nil {
		e.Sites = len(s)
	}
	running := EnvProcesses(procs, roots)
	portSet := map[int]bool{}
	for _, p := range running {
		if !IsServer(p) {
			continue
		}
		e.Running = true
		for _, port := range p.Ports {
			portSet[port] = true
		}
		n := strings.ToLower(p.Name)
		if n == "mysqld.exe" || n == "mariadbd.exe" || n == "mysqld" || n == "mariadbd" {
			e.Databases = true
		}
	}
	for p := range portSet {
		e.Ports = append(e.Ports, p)
	}
	sort.Ints(e.Ports)
	e.CanStop = e.Running
	return e
}

// EnvProcesses filters procs to those whose executable is inside one of roots.
func EnvProcesses(procs []Proc, roots []string) []Proc {
	var out []Proc
	for _, p := range procs {
		if p.Exe == "" {
			continue
		}
		for _, r := range roots {
			if Under(p.Exe, r) {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// Under reports whether path is inside dir (or equal to it), case-insensitively
// on Windows-style paths.
func Under(path, dir string) bool {
	if path == "" || dir == "" {
		return false
	}
	p := normPath(path)
	d := normPath(dir)
	if p == d {
		return true
	}
	if !strings.HasSuffix(d, "/") {
		d += "/"
	}
	return strings.HasPrefix(p, d)
}

func normPath(p string) string {
	p = strings.ReplaceAll(filepath.Clean(p), `\`, "/")
	return strings.ToLower(p)
}

func dedupPaths(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range in {
		if p == "" {
			continue
		}
		p = filepath.Clean(p)
		if seen[normPath(p)] || !isDir(p) {
			continue
		}
		seen[normPath(p)] = true
		out = append(out, p)
	}
	return out
}

// Classify returns the environment kind owning exe ("" when unknown,
// EnvApnoro for Apnoro's own binaries).
func Classify(exe string) string { return classifyPath(exe, allRoots()) }

func classifyPath(exe string, roots map[string][]string) string {
	if exe == "" {
		return ""
	}
	if Under(exe, paths.Home()) || Under(exe, paths.BinDir()) {
		return EnvApnoro
	}
	for _, k := range Kinds {
		for _, r := range roots[k] {
			if Under(exe, r) {
				return k
			}
		}
	}
	// Heuristic on path segments for non-default install locations.
	segs := strings.Split(normPath(exe), "/")
	for _, s := range segs[:max(0, len(segs)-1)] {
		switch s {
		case "xampp":
			return api.EnvXAMPP
		case "herd":
			return api.EnvHerd
		case "laragon":
			return api.EnvLaragon
		case "wamp", "wamp64":
			return api.EnvWAMP
		}
	}
	return ""
}

// PortOwner identifies the process listening on port. ok is false when the
// port is free. Service is left empty (the caller knows which service needs it).
func PortOwner(port int) (api.PortConflict, bool) {
	return portOwnerIn(port, Processes(), allRoots())
}

func portOwnerIn(port int, procs []Proc, roots map[string][]string) (api.PortConflict, bool) {
	for _, p := range procs {
		for _, lp := range p.Ports {
			if lp != port {
				continue
			}
			c := api.PortConflict{Port: port, Process: p.Name, Path: p.Exe}
			c.Env = classifyPath(p.Exe, roots)
			if p.PID == 4 {
				c.Process = "System (http.sys: IIS / World Wide Web Publishing or another Windows web service)"
			}
			return c, true
		}
	}
	if inUse, name := services.PortInUse(port); inUse {
		return api.PortConflict{Port: port, Process: name}, true
	}
	return api.PortConflict{}, false
}

// Conflicts reports which of the given ports (service name -> port) are held
// by programs other than Apnoro itself.
func Conflicts(ports map[string]int) []api.PortConflict {
	procs := Processes()
	roots := allRoots()
	var out []api.PortConflict
	names := make([]string, 0, len(ports))
	for n := range ports {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, svc := range names {
		port := ports[svc]
		if port <= 0 {
			continue
		}
		c, ok := portOwnerIn(port, procs, roots)
		if !ok || c.Env == EnvApnoro {
			continue
		}
		c.Service = svc
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Port < out[j].Port })
	return out
}

// PHPOnPath returns the first php executable on PATH and the environment it
// belongs to (a kind, EnvApnoro, or "" when unknown).
func PHPOnPath() (path string, env string) {
	path = findOnPath("php")
	if path == "" {
		return "", ""
	}
	return path, Classify(path)
}

func findOnPath(name string) string {
	exts := []string{""}
	if isWindows {
		exts = nil
		pe := os.Getenv("PATHEXT")
		if pe == "" {
			pe = ".COM;.EXE;.BAT;.CMD"
		}
		for _, e := range strings.Split(pe, ";") {
			if e = strings.TrimSpace(e); e != "" {
				exts = append(exts, strings.ToLower(e))
			}
		}
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		dir = strings.Trim(dir, `"`)
		if dir == "" {
			continue
		}
		for _, e := range exts {
			p := filepath.Join(dir, name+e)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}
	return ""
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

var (
	phpVerMu    sync.Mutex
	phpVerCache = map[string]string{}
)

// phpVersionOf runs `php -r "echo PHP_VERSION;"` (hidden, 5s timeout), cached
// per executable and modification time.
func phpVersionOf(exe string) string {
	st, err := os.Stat(exe)
	if err != nil || st.IsDir() {
		return ""
	}
	key := exe + "|" + st.ModTime().String()
	phpVerMu.Lock()
	v, ok := phpVerCache[key]
	phpVerMu.Unlock()
	if ok {
		return v
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-n", "-r", "echo PHP_VERSION;")
	services.Hide(cmd)
	out, err := cmd.Output()
	if err == nil {
		v = strings.TrimSpace(string(out))
		if len(v) > 32 {
			v = ""
		}
	}
	phpVerMu.Lock()
	phpVerCache[key] = v
	phpVerMu.Unlock()
	return v
}
