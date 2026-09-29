package projects

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// WordPressURL is the archive downloaded for new WordPress projects.
var WordPressURL = "https://wordpress.org/latest.zip"

func createWordPress(ctx context.Context, r Request, target string, progress ProgressFunc) error {
	tmp, err := os.CreateTemp("", "ampls-wordpress-*.zip")
	if err != nil {
		return fmt.Errorf("projects: wordpress: %w", err)
	}
	tmpName := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpName)

	progress("Downloading WordPress", 0)
	if err := downloadFile(ctx, WordPressURL, tmpName, func(done, total int64) {
		if total > 0 {
			progress(fmt.Sprintf("Downloading WordPress (%.1f / %.1f MB)", float64(done)/1e6, float64(total)/1e6), float64(done)/float64(total)*70)
		}
	}); err != nil {
		return fmt.Errorf("projects: wordpress: download: %w", err)
	}

	progress("Extracting WordPress", 75)
	if err := unzipStrip(tmpName, target, "wordpress/"); err != nil {
		os.RemoveAll(target)
		return fmt.Errorf("projects: wordpress: extract: %w", err)
	}

	if r.DB != "" {
		progress("Writing wp-config.php", 95)
		sample, err := os.ReadFile(filepath.Join(target, "wp-config-sample.php"))
		if err != nil {
			return fmt.Errorf("projects: wordpress: %w", err)
		}
		port := r.DBPort
		if port == 0 {
			port = 3306
		}
		cfg, err := WPConfig(string(sample), r.DB, "root", r.DBPassword, "127.0.0.1:"+strconv.Itoa(port))
		if err != nil {
			return fmt.Errorf("projects: wordpress: %w", err)
		}
		if err := os.WriteFile(filepath.Join(target, "wp-config.php"), []byte(cfg), 0o644); err != nil {
			return fmt.Errorf("projects: wordpress: write wp-config.php: %w", err)
		}
	}
	return nil
}

func downloadFile(ctx context.Context, url, dest string, progress func(done, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AMPLS")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	var done int64
	buf := make([]byte, 256*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				f.Close()
				return err
			}
			done += int64(n)
			progress(done, resp.ContentLength)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return rerr
		}
	}
	if resp.ContentLength > 0 && done != resp.ContentLength {
		f.Close()
		return fmt.Errorf("incomplete download (%d of %d bytes)", done, resp.ContentLength)
	}
	return f.Close()
}

// unzipStrip extracts src into dest, removing prefix from entry names and
// skipping entries outside it. Protects against zip-slip.
func unzipStrip(src, dest, prefix string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	root, _ := filepath.Abs(dest)
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, `\`, "/")
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		name = strings.TrimPrefix(name, prefix)
		if name == "" {
			continue
		}
		p := filepath.Join(root, filepath.FromSlash(name))
		if p != root && !strings.HasPrefix(p, root+string(filepath.Separator)) {
			return fmt.Errorf("illegal path in archive: %s", f.Name)
		}
		if f.FileInfo().IsDir() || strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := extractFile(f, p); err != nil {
			return err
		}
	}
	return nil
}

func extractFile(f *zip.File, p string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(p, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// wpSaltKeys are the secret keys defined in wp-config-sample.php.
var wpSaltKeys = []string{
	"AUTH_KEY", "SECURE_AUTH_KEY", "LOGGED_IN_KEY", "NONCE_KEY",
	"AUTH_SALT", "SECURE_AUTH_SALT", "LOGGED_IN_SALT", "NONCE_SALT",
}

const saltChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()-_[]{}<>~+=,.;:/?|"

func randomSalt(n int) (string, error) {
	b := make([]byte, n)
	max := big.NewInt(int64(len(saltChars)))
	for i := range b {
		x, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = saltChars[x.Int64()]
	}
	return string(b), nil
}

// phpSingleQuote escapes s for a PHP single-quoted string.
func phpSingleQuote(s string) string {
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s)
}

func defineRe(key string) *regexp.Regexp {
	return regexp.MustCompile(`define\(\s*'` + key + `'\s*,\s*'[^']*'\s*\);`)
}

// WPConfig turns wp-config-sample.php content into a wp-config.php with the
// given database settings and freshly generated salts.
func WPConfig(sample, dbName, dbUser, dbPassword, dbHost string) (string, error) {
	set := func(s, key, val string) (string, error) {
		re := defineRe(key)
		if !re.MatchString(s) {
			return s, fmt.Errorf("wp-config-sample.php: %s not found", key)
		}
		repl := "define( '" + key + "', '" + phpSingleQuote(val) + "' );"
		done := false
		return re.ReplaceAllStringFunc(s, func(m string) string {
			if done {
				return m
			}
			done = true
			return repl
		}), nil
	}
	var err error
	out := sample
	for _, kv := range [][2]string{{"DB_NAME", dbName}, {"DB_USER", dbUser}, {"DB_PASSWORD", dbPassword}, {"DB_HOST", dbHost}} {
		if out, err = set(out, kv[0], kv[1]); err != nil {
			return "", err
		}
	}
	for _, k := range wpSaltKeys {
		salt, err := randomSalt(64)
		if err != nil {
			return "", err
		}
		if out, err = set(out, k, salt); err != nil {
			return "", err
		}
	}
	return out, nil
}
