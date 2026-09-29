// Package projects scaffolds new Laravel, WordPress and blank PHP projects.
package projects

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ampls/internal/api"
	"ampls/internal/sites"
)

type Request struct {
	Kind, Name, Dir string // Dir = parent directory
	PHP             string // php.exe to use (absolute)
	ComposerPhar    string
	DB              string // database name to wire into .env / wp-config ("" = none)
	DBPort          int
	DBPassword      string
	// Optional (additions to the contract): used for APP_URL.
	TLD    string // "" = "test"
	Secure bool   // https APP_URL
}

// ProgressFunc receives human-readable progress; pct is 0..100 or -1 (indeterminate).
type ProgressFunc = func(msg string, pct float64)

// Create scaffolds the project into Dir/Name and returns its path.
func Create(ctx context.Context, r Request, progress func(msg string, pct float64)) (projectPath string, err error) {
	if progress == nil {
		progress = func(string, float64) {}
	}
	if !sites.ValidName(r.Name) {
		return "", fmt.Errorf("projects: invalid name %q (use lowercase letters, digits and dashes)", r.Name)
	}
	if r.Dir == "" {
		return "", errors.New("projects: parent directory is required")
	}
	if r.DBPort == 0 {
		r.DBPort = 3306
	}
	if r.TLD == "" {
		r.TLD = "test"
	}
	dir, err := filepath.Abs(r.Dir)
	if err != nil {
		return "", fmt.Errorf("projects: %w", err)
	}
	target := filepath.Join(dir, r.Name)
	if err := checkTarget(target); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("projects: create %s: %w", dir, err)
	}
	r.Dir = dir

	switch r.Kind {
	case api.KindLaravel:
		err = createLaravel(ctx, r, target, progress)
	case api.KindWordPress:
		err = createWordPress(ctx, r, target, progress)
	case api.KindBlank, "":
		err = createBlank(r, target)
	default:
		return "", fmt.Errorf("projects: unknown kind %q", r.Kind)
	}
	if err != nil {
		return "", err
	}
	progress("Project "+r.Name+" created", 100)
	return target, nil
}

// checkTarget refuses an existing non-empty directory (or a file).
func checkTarget(target string) error {
	st, err := os.Stat(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("projects: %w", err)
	}
	if !st.IsDir() {
		return fmt.Errorf("projects: %s already exists", target)
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return fmt.Errorf("projects: %w", err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("projects: %s already exists and is not empty", target)
	}
	return nil
}

func appURL(r Request) string {
	scheme := "http"
	if r.Secure {
		scheme = "https"
	}
	return scheme + "://" + r.Name + "." + r.TLD
}

// runStreaming runs cmd with a hidden window and reports every output line.
func runStreaming(cmd *exec.Cmd, progress ProgressFunc) error {
	hideWindow(cmd)
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	cmd.WaitDelay = 10 * time.Second
	var tail []string
	var mu sync.Mutex
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			mu.Lock()
			tail = append(tail, line)
			if len(tail) > 15 {
				tail = tail[1:]
			}
			mu.Unlock()
			progress(line, -1)
		}
		io.Copy(io.Discard, pr)
	}()
	err := cmd.Start()
	if err == nil {
		err = cmd.Wait()
	}
	pw.Close()
	<-done
	if err != nil {
		mu.Lock()
		defer mu.Unlock()
		return fmt.Errorf("%w\n%s", err, strings.Join(tail, "\n"))
	}
	return nil
}

// envWithPHPFirst returns the environment with phpDir first on PATH.
func envWithPHPFirst(phpDir string) []string {
	env := os.Environ()
	out := make([]string, 0, len(env)+2)
	found := false
	for _, kv := range env {
		i := strings.IndexByte(kv, '=')
		if i > 0 && strings.EqualFold(kv[:i], "PATH") {
			out = append(out, kv[:i+1]+phpDir+string(os.PathListSeparator)+kv[i+1:])
			found = true
			continue
		}
		out = append(out, kv)
	}
	if !found {
		out = append(out, "PATH="+phpDir)
	}
	return append(out, "COMPOSER_NO_INTERACTION=1")
}
