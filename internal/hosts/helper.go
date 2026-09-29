package hosts

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// helperTLDs are the only TLDs the privileged helper will write. They are all
// reserved / non-public, so a non-admin user cannot use the helper to hijack
// real internet names machine-wide. Other TLDs still work via UAC elevation.
var helperTLDs = map[string]bool{
	"test": true, "localhost": true, "local": true, "internal": true,
	"lan": true, "home": true, "example": true, "invalid": true,
	"corp": true, "localdev": true,
}

// configuredTLD reads the tld from <home>/config.json ("test" by default).
func configuredTLD(home string) string {
	b, err := readRegular(filepath.Join(home, "config.json"))
	if err != nil {
		return "test"
	}
	var c struct {
		TLD string `json:"tld"`
	}
	if json.Unmarshal(b, &c) != nil || c.TLD == "" {
		return "test"
	}
	return c.TLD
}

// RunHelper runs the hosts helper loop until the process exits.
func RunHelper(home string) error {
	return RunHelperContext(context.Background(), home)
}

// RunHelperContext watches <home>/run/hosts.json (fsnotify plus a 2s poll) and
// applies validated requests until ctx is cancelled.
func RunHelperContext(ctx context.Context, home string) error {
	if home == "" {
		return fmt.Errorf("hosts helper: home not set")
	}
	runDir := filepath.Join(home, "run")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("hosts helper: %w", err)
	}
	var last time.Time
	if a, err := readApplied(runDir); err == nil {
		last = a.RequestedAt
	}
	process := func() {
		req, err := readPending(runDir)
		if err != nil || req.RequestedAt.IsZero() || !req.RequestedAt.After(last) {
			return
		}
		last = req.RequestedAt
		tld := configuredTLD(home)
		if !helperTLDs[tld] {
			res := AppliedResult{RequestedAt: req.RequestedAt, AppliedAt: time.Now().UTC(),
				Error: fmt.Sprintf("tld %q is not allowed for the helper service", tld)}
			_ = writeApplied(runDir, res)
			return
		}
		if err := applyPending(runDir, tld); err != nil {
			log.Printf("ampls-helper: %v", err)
		} else {
			log.Printf("ampls-helper: applied %d domains", len(req.Domains))
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
