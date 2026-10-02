package hosts

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// RunHelper runs the hosts helper loop until the process exits.
func RunHelper(home string) error {
	return RunHelperContext(context.Background(), home)
}

// RunHelperContext watches <home>/run/hosts.json (fsnotify plus a 2s poll) and
// applies validated requests until ctx is cancelled.
//
// Everything under home is writable by unprivileged users, so the helper
// treats it as untrusted input: it never reads the TLD from config.json or the
// request (only the fixed AllowedTLDs are written), reads the request with a
// size cap and without following links, and pins the run directory (refusing
// junctions/symlinks anywhere in its path) while it reads the request and
// writes run/hosts.applied.json, so its LocalSystem writes cannot be
// redirected elsewhere.
func RunHelperContext(ctx context.Context, home string) error {
	if home == "" || !filepath.IsAbs(home) {
		return fmt.Errorf("hosts helper: home must be an absolute path (got %q)", home)
	}
	home = filepath.Clean(home)
	runDir := filepath.Join(home, "run")
	if release, err := lockDir(home); err == nil {
		if _, err := os.Lstat(runDir); errors.Is(err, os.ErrNotExist) {
			_ = os.Mkdir(runDir, 0o755)
		}
		release()
	} else {
		log.Printf("apnoro-helper: %v", err)
	}
	var last time.Time
	if release, err := lockDir(runDir); err == nil {
		if a, err := readApplied(runDir); err == nil {
			last = a.RequestedAt
		}
		release()
	}
	lastErr := ""
	logOnce := func(err error) {
		if msg := err.Error(); msg != lastErr {
			lastErr = msg
			log.Printf("apnoro-helper: %s", msg)
		}
	}
	process := func() {
		release, err := lockDir(runDir)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				logOnce(err)
			}
			return
		}
		defer release()
		req, err := readPending(runDir)
		// Equal (not After): a bogus far-future timestamp must not block later requests.
		if err != nil || req.RequestedAt.IsZero() || req.RequestedAt.Equal(last) {
			return
		}
		last = req.RequestedAt
		if err := applyRequest(runDir, req); err != nil {
			logOnce(err)
		} else {
			lastErr = ""
			log.Printf("apnoro-helper: applied %d domains", len(req.Domains))
		}
	}

	var events <-chan fsnotify.Event
	var errs <-chan error
	if w, err := fsnotify.NewWatcher(); err == nil {
		defer w.Close()
		if w.Add(runDir) == nil {
			events, errs = w.Events, w.Errors
		}
	}
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	process()
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			if filepath.Base(ev.Name) == requestFile {
				time.Sleep(50 * time.Millisecond)
				process()
			}
		case _, ok := <-errs:
			if !ok {
				errs = nil
			}
		case <-tick.C:
			process()
		}
	}
}
