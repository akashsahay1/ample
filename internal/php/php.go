// Package php manages side-by-side PHP versions under paths.PHPRoot().
package php

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ampls/internal/download"
	"ampls/internal/paths"
	"ampls/internal/services"
)

// VersionFile inside each version dir caches the full version ("8.3.12").
const VersionFile = ".ampls-version"

// Installed is a PHP version present on disk.
type Installed struct {
	Minor, Full, Dir string
}

func CLIPath(minor string) string { return filepath.Join(paths.PHPDir(minor), paths.Exe("php")) }
func CGIPath(minor string) string { return filepath.Join(paths.PHPDir(minor), paths.Exe("php-cgi")) }
func IniPath(minor string) string { return filepath.Join(paths.PHPDir(minor), "php.ini") }

// List scans paths.PHPRoot() for installed versions, newest first.
func List() ([]Installed, error) {
	entries, err := os.ReadDir(paths.PHPRoot())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("php: list: %w", err)
	}
	var out []Installed
	for _, e := range entries {
		if !e.IsDir() || !ValidMinor(e.Name()) {
			continue
		}
		minor := e.Name()
		if _, err := os.Stat(CLIPath(minor)); err != nil {
			continue
		}
		out = append(out, Installed{Minor: minor, Full: fullVersion(minor), Dir: paths.PHPDir(minor)})
	}
	sort.Slice(out, func(i, j int) bool { return Compare(out[i].Minor, out[j].Minor) > 0 })
	return out, nil
}

func fullVersion(minor string) string {
	vf := filepath.Join(paths.PHPDir(minor), VersionFile)
	if b, err := os.ReadFile(vf); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			return s
		}
	}
	v := queryVersion(CLIPath(minor))
	if v == "" {
		return minor
	}
	_ = os.WriteFile(vf, []byte(v+"\n"), 0o644)
	return v
}

// queryVersion runs php.exe -n -r "echo PHP_VERSION;".
func queryVersion(exe string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-n", "-r", "echo PHP_VERSION;")
	services.Hide(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Install downloads and installs the latest build of minor.
func Install(ctx context.Context, minor string, progress func(msg string, pct float64)) error {
	if progress == nil {
		progress = func(string, float64) {}
	}
	if !ValidMinor(minor) {
		return fmt.Errorf("php: invalid version %q", minor)
	}
	progress("Looking up PHP "+minor, -1)
	rels, err := Available(ctx)
	if err != nil {
		return err
	}
	var rel *Release
	for i := range rels {
		if rels[i].Minor == minor {
			rel = &rels[i]
			break
		}
	}
	if rel == nil {
		return fmt.Errorf("php: version %s is not available for download", minor)
	}
	zipPath := filepath.Join(paths.DownloadsDir(), filepath.Base(rel.URL))
	if !cachedOK(zipPath, rel.SHA256) {
		progress(fmt.Sprintf("Downloading PHP %s", rel.Full), 0)
		cb := func(done, total int64) {
			if total <= 0 {
				total = rel.Size
			}
			if total > 0 {
				progress(fmt.Sprintf("Downloading PHP %s", rel.Full), 85*float64(done)/float64(total))
			}
		}
		err := download.File(ctx, rel.URL, zipPath, cb)
		if err != nil {
			if alt := archiveURL(rel.URL); alt != "" && ctx.Err() == nil {
				err = download.File(ctx, alt, zipPath, cb)
			}
		}
		if err != nil {
			return fmt.Errorf("php: %w", err)
		}
		if rel.SHA256 != "" {
			progress("Verifying download", 86)
			sum, err := download.SHA256File(zipPath)
			if err != nil {
				return fmt.Errorf("php: verify: %w", err)
			}
			if !strings.EqualFold(sum, rel.SHA256) {
				os.Remove(zipPath)
				return fmt.Errorf("php: checksum mismatch for %s", filepath.Base(zipPath))
			}
		}
	}
	progress("Extracting PHP "+rel.Full, 88)
	if _, err := installZip(zipPath, minor, rel.Full); err != nil {
		return err
	}
	progress("PHP "+rel.Full+" installed", 100)
	return nil
}

func cachedOK(zipPath, sha string) bool {
	if sha == "" {
		return false
	}
	if _, err := os.Stat(zipPath); err != nil {
		return false
	}
	sum, err := download.SHA256File(zipPath)
	return err == nil && strings.EqualFold(sum, sha)
}

// InstallFromZip installs a local PHP zip (e.g. the one bundled by the installer).
// The version is taken from the file name, falling back to running php.exe.
func InstallFromZip(zipPath string) (Installed, error) {
	full := versionFromZipName(filepath.Base(zipPath))
	minor := ""
	if full != "" {
		minor = MinorOf(full)
	}
	return installZip(zipPath, minor, full)
}

// installZip extracts into a staging dir, then swaps it into place, keeping an
// existing php.ini.
func installZip(zipPath, minor, full string) (Installed, error) {
	if err := os.MkdirAll(paths.PHPRoot(), 0o755); err != nil {
		return Installed{}, fmt.Errorf("php: %w", err)
	}
	stage, err := os.MkdirTemp(paths.PHPRoot(), ".staging-")
	if err != nil {
		return Installed{}, fmt.Errorf("php: %w", err)
	}
	defer os.RemoveAll(stage)
	if err := download.Unzip(zipPath, stage, true); err != nil {
		return Installed{}, fmt.Errorf("php: %w", err)
	}
	cli := filepath.Join(stage, paths.Exe("php"))
	if _, err := os.Stat(cli); err != nil {
		return Installed{}, fmt.Errorf("php: %s does not contain %s", filepath.Base(zipPath), paths.Exe("php"))
	}
	if full == "" {
		full = queryVersion(cli)
		if full == "" {
			return Installed{}, fmt.Errorf("php: cannot determine version of %s", filepath.Base(zipPath))
		}
	}
	if minor == "" {
		minor = MinorOf(full)
	}
	dir := paths.PHPDir(minor)
	oldIni := IniPath(minor)
	if b, err := os.ReadFile(oldIni); err == nil {
		_ = os.WriteFile(filepath.Join(stage, "php.ini"), b, 0o644)
	}
	if _, err := os.Stat(dir); err == nil {
		trash := dir + ".old-" + fmt.Sprint(time.Now().UnixNano())
		if err := os.Rename(dir, trash); err != nil {
			return Installed{}, fmt.Errorf("php: %s is in use (stop Apache first): %w", minor, err)
		}
		defer os.RemoveAll(trash)
	}
	if err := os.Rename(stage, dir); err != nil {
		return Installed{}, fmt.Errorf("php: move into place: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, VersionFile), []byte(full+"\n"), 0o644); err != nil {
		return Installed{}, fmt.Errorf("php: %w", err)
	}
	if err := EnsureIni(minor); err != nil {
		return Installed{}, err
	}
	return Installed{Minor: minor, Full: full, Dir: dir}, nil
}

// Remove deletes an installed version.
func Remove(minor string) error {
	if !ValidMinor(minor) {
		return fmt.Errorf("php: invalid version %q", minor)
	}
	dir := paths.PHPDir(minor)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("php: %s is not installed", minor)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("php: remove %s (is it in use?): %w", minor, err)
	}
	return nil
}
