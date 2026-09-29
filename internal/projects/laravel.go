package projects

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func createLaravel(ctx context.Context, r Request, target string, progress ProgressFunc) error {
	if r.PHP == "" {
		return fmt.Errorf("projects: laravel: no PHP executable given")
	}
	if _, err := os.Stat(r.PHP); err != nil {
		return fmt.Errorf("projects: laravel: php: %w", err)
	}
	if _, err := os.Stat(r.ComposerPhar); err != nil {
		return fmt.Errorf("projects: laravel: composer.phar: %w", err)
	}
	env := envWithPHPFirst(filepath.Dir(r.PHP))

	progress("Downloading Laravel with Composer (this can take a few minutes)...", 5)
	cmd := exec.CommandContext(ctx, r.PHP, r.ComposerPhar, "create-project", "laravel/laravel", r.Name,
		"--prefer-dist", "--no-interaction", "--no-ansi")
	cmd.Dir = r.Dir
	cmd.Env = env
	if err := runStreaming(cmd, progress); err != nil {
		return fmt.Errorf("projects: composer create-project: %w", err)
	}

	progress("Configuring .env", 90)
	envPath := filepath.Join(target, ".env")
	b, err := os.ReadFile(envPath)
	if err != nil {
		return fmt.Errorf("projects: laravel: read .env: %w", err)
	}
	vals := LaravelEnvValues(r)
	out := UpdateEnv(string(b), vals)
	if err := os.WriteFile(envPath, []byte(out), 0o644); err != nil {
		return fmt.Errorf("projects: laravel: write .env: %w", err)
	}

	if r.DB != "" {
		progress("Running migrations on MySQL database "+r.DB, 95)
		mctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		m := exec.CommandContext(mctx, r.PHP, "artisan", "migrate", "--force", "--no-interaction", "--no-ansi")
		m.Dir = target
		m.Env = env
		if err := runStreaming(m, progress); err != nil {
			// not fatal: MySQL may not be running yet
			progress("Migrations skipped (run `php artisan migrate` later): "+firstLine(err.Error()), 97)
		}
	}
	return nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// LaravelEnvValues returns the .env keys to set for r, in order.
func LaravelEnvValues(r Request) [][2]string {
	tld := r.TLD
	if tld == "" {
		tld = "test"
	}
	scheme := "http"
	if r.Secure {
		scheme = "https"
	}
	vals := [][2]string{{"APP_URL", scheme + "://" + r.Name + "." + tld}}
	if r.DB != "" {
		port := r.DBPort
		if port == 0 {
			port = 3306
		}
		vals = append(vals,
			[2]string{"DB_CONNECTION", "mysql"},
			[2]string{"DB_HOST", "127.0.0.1"},
			[2]string{"DB_PORT", strconv.Itoa(port)},
			[2]string{"DB_DATABASE", r.DB},
			[2]string{"DB_USERNAME", "root"},
			[2]string{"DB_PASSWORD", r.DBPassword},
		)
	}
	return vals
}

var envKeyRe = regexp.MustCompile(`^\s*(#\s*)?([A-Za-z_][A-Za-z0-9_]*)\s*=`)

// quoteEnv quotes a .env value when needed.
func quoteEnv(v string) string {
	if v == "" {
		return ""
	}
	if strings.ContainsAny(v, " \t#\"'$\\=`") {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`).Replace(v) + `"`
	}
	return v
}

// UpdateEnv sets keys in a .env file. For each key the first active line is
// replaced; if there is none, the first commented-out "# KEY=" line is
// uncommented and replaced; otherwise the key is appended. Line endings of the
// input are preserved.
func UpdateEnv(content string, vals [][2]string) string {
	nl := "\n"
	if strings.Contains(content, "\r\n") {
		nl = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	trailing := len(lines) > 0 && lines[len(lines)-1] == ""
	if trailing {
		lines = lines[:len(lines)-1]
	}
	for _, kv := range vals {
		key, line := kv[0], kv[0]+"="+quoteEnv(kv[1])
		active, commented := -1, -1
		for i, l := range lines {
			m := envKeyRe.FindStringSubmatch(l)
			if m == nil || m[2] != key {
				continue
			}
			if m[1] == "" && active < 0 {
				active = i
			} else if m[1] != "" && commented < 0 {
				commented = i
			}
		}
		switch {
		case active >= 0:
			lines[active] = line
		case commented >= 0:
			lines[commented] = line
		default:
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, nl) + nl
}
