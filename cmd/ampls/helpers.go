package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/tabwriter"

	"ampls/internal/api"
	"ampls/internal/config"
	"ampls/internal/core"
	"ampls/internal/php"
	"ampls/internal/sites"
)

// ---------- backend ----------

var (
	backendOnce sync.Once
	backendInst *core.Core
)

// backend returns the shared core instance (created after --home is applied).
func backend() *core.Core {
	backendOnce.Do(func() { backendInst = core.New() })
	return backendInst
}

// loadConfig loads config.json and fills in the effective default PHP version
// (configured default if installed, else the newest installed).
func loadConfig() (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	inst, _ := php.List()
	found := false
	for _, i := range inst {
		if i.Minor == cfg.DefaultPHP {
			found = true
		}
	}
	if !found && len(inst) > 0 {
		cfg.DefaultPHP = inst[0].Minor
	}
	return cfg, nil
}

var errNotInSite = errors.New("not inside an AMPLS site; run `ampls link` first")

// siteForCwd returns the site containing the current directory.
func siteForCwd() (api.Site, error) {
	cfg, err := loadConfig()
	if err != nil {
		return api.Site{}, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return api.Site{}, err
	}
	s, ok := sites.FindByPath(cfg, cwd)
	if !ok {
		return api.Site{}, errNotInSite
	}
	return s, nil
}

// resolveSite returns the named site, or the site for the current directory when name is "".
func resolveSite(name string) (api.Site, error) {
	if name == "" {
		return siteForCwd()
	}
	cfg, err := loadConfig()
	if err != nil {
		return api.Site{}, err
	}
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.TrimSuffix(name, "."+cfg.TLD)
	s, ok := sites.Find(cfg, name)
	if !ok {
		return api.Site{}, fmt.Errorf("no site named %q (see `ampls sites`)", name)
	}
	return s, nil
}

func firstArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

// absDir returns the absolute form of dir, or the current directory when dir is "".
func absDir(dir string) (string, error) {
	if dir == "" {
		return os.Getwd()
	}
	return filepath.Abs(dir)
}

// normMinor turns "8.3.12" or "php8.3" into "8.3".
func normMinor(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	v = strings.TrimPrefix(v, "php")
	v = strings.TrimPrefix(v, "@")
	parts := strings.Split(v, ".")
	if len(parts) > 2 {
		parts = parts[:2]
	}
	return strings.Join(parts, ".")
}

// ---------- output ----------

func newTable() *tabwriter.Writer {
	return tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
}

func row(w *tabwriter.Writer, cols ...string) {
	fmt.Fprintln(w, strings.Join(cols, "\t"))
}

var colorOn bool

func paint(code, s string) string {
	if !colorOn {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func green(s string) string  { return paint("32", s) }
func red(s string) string    { return paint("31", s) }
func yellow(s string) string { return paint("33", s) }
func dim(s string) string    { return paint("2", s) }
func bold(s string) string   { return paint("1", s) }

const (
	symOK   = "✓"
	symFail = "✗"
)

func ok(format string, a ...any) {
	fmt.Println(green(symOK) + " " + fmt.Sprintf(format, a...))
}

func warn(format string, a ...any) {
	fmt.Fprintln(os.Stderr, yellow("warning: ")+fmt.Sprintf(format, a...))
}

// confirm asks a y/N question on stdin.
func confirm(question string) bool {
	fmt.Print(question + " [y/N] ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}

// progressBar renders api.Progress events on a single terminal line.
type progressBar struct {
	lastLen int
	lastMsg string
	lines   bool // print one line per message instead of a bar (non-terminal output)
}

func newProgressBar() *progressBar { return &progressBar{lines: !isTerminal()} }

func (p *progressBar) update(ev api.Progress) {
	if ev.Done {
		p.clear()
		return
	}
	if p.lines {
		if ev.Message != "" && ev.Message != p.lastMsg {
			fmt.Println("  " + ev.Message)
			p.lastMsg = ev.Message
		}
		return
	}
	const width = 28
	var bar string
	if ev.Percent < 0 {
		bar = "[" + strings.Repeat("·", width) + "]  ... "
	} else {
		pct := ev.Percent
		if pct > 100 {
			pct = 100
		}
		n := int(pct / 100 * width)
		bar = fmt.Sprintf("[%s%s] %3.0f%% ", strings.Repeat("█", n), strings.Repeat("░", width-n), pct)
	}
	msg := ev.Message
	if len(msg) > 42 {
		msg = msg[:41] + "…"
	}
	line := "  " + bar + " " + msg
	n := len([]rune(line))
	pad := ""
	if n < p.lastLen {
		pad = strings.Repeat(" ", p.lastLen-n)
	}
	fmt.Print("\r" + line + pad)
	p.lastLen = n
}

func (p *progressBar) clear() {
	if p.lastLen > 0 {
		fmt.Print("\r" + strings.Repeat(" ", p.lastLen) + "\r")
		p.lastLen = 0
	}
}

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func yesNo(b bool) string {
	if b {
		return symOK // no color codes inside tables: they break tabwriter alignment
	}
	return ""
}

func humanBytes(n int64) string {
	switch {
	case n <= 0:
		return ""
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
