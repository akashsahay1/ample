// Package download fetches files over HTTP and extracts zip archives.
package download

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// UserAgent is sent with every request; some mirrors (Apache Lounge,
// windows.php.net) reject Go's default user agent.
const UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36 Apnoro/1.0"

// ProgressFunc reports bytes downloaded so far and the total (-1 if unknown).
type ProgressFunc func(done, total int64)

var client = &http.Client{Timeout: 0}

// Get performs a GET with the Apnoro user agent and returns the body (caller closes).
func Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "*/*")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return resp, nil
}

// File downloads url to dest, writing to dest+".part" and renaming on success.
func File(ctx context.Context, url, dest string, progress ProgressFunc) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("download: %w", err)
	}
	resp, err := Get(ctx, url)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	part := dest + ".part"
	f, err := os.Create(part)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	total := resp.ContentLength
	var done int64
	buf := make([]byte, 256*1024)
	last := time.Time{}
	var werr error
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				werr = err
				break
			}
			done += int64(n)
			if progress != nil && time.Since(last) > 100*time.Millisecond {
				progress(done, total)
				last = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			werr = rerr
			break
		}
	}
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil && total > 0 && done != total {
		werr = fmt.Errorf("short read: got %d of %d bytes", done, total)
	}
	if werr != nil {
		os.Remove(part)
		return fmt.Errorf("download %s: %w", url, werr)
	}
	if progress != nil {
		progress(done, total)
	}
	os.Remove(dest)
	if err := os.Rename(part, dest); err != nil {
		return fmt.Errorf("download: %w", err)
	}
	return nil
}

// SHA256File returns the lowercase hex sha256 of a file.
func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Unzip extracts src into dest. With stripTopDir, if every entry lives under a
// single top-level directory, that directory's contents are extracted into dest.
func Unzip(src, dest string, stripTopDir bool) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("unzip: %w", err)
	}
	defer zr.Close()

	prefix := ""
	if stripTopDir {
		prefix = commonTopDir(zr.File)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("unzip: %w", err)
	}
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	for _, zf := range zr.File {
		name := strings.ReplaceAll(zf.Name, `\`, "/")
		if prefix != "" {
			name = strings.TrimPrefix(name, prefix)
		}
		if name == "" || name == "/" {
			continue
		}
		target := filepath.Join(absDest, filepath.FromSlash(name))
		if target != absDest && !strings.HasPrefix(target, absDest+string(os.PathSeparator)) {
			return fmt.Errorf("unzip: illegal path %q", zf.Name)
		}
		if zf.FileInfo().IsDir() || strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("unzip: %w", err)
			}
			continue
		}
		if err := extractFile(zf, target); err != nil {
			return fmt.Errorf("unzip %s: %w", zf.Name, err)
		}
	}
	return nil
}

func extractFile(zf *zip.File, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	mode := zf.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode|0o200)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// UnzipSubdir extracts only the entries under sub (e.g. "Apache24") into dest,
// stripping that prefix. Used for archives with extra root files next to the
// payload directory (Apache Lounge zips contain ReadMe.txt + Apache24/).
func UnzipSubdir(src, sub, dest string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("unzip: %w", err)
	}
	defer zr.Close()
	prefix := strings.Trim(strings.ReplaceAll(sub, `\`, "/"), "/") + "/"
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(absDest, 0o755); err != nil {
		return fmt.Errorf("unzip: %w", err)
	}
	found := false
	for _, zf := range zr.File {
		name := strings.ReplaceAll(zf.Name, `\`, "/")
		if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
			continue
		}
		found = true
		name = name[len(prefix):]
		if name == "" {
			continue
		}
		target := filepath.Join(absDest, filepath.FromSlash(name))
		if !strings.HasPrefix(target, absDest+string(os.PathSeparator)) {
			return fmt.Errorf("unzip: illegal path %q", zf.Name)
		}
		if zf.FileInfo().IsDir() || strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := extractFile(zf, target); err != nil {
			return fmt.Errorf("unzip %s: %w", zf.Name, err)
		}
	}
	if !found {
		return fmt.Errorf("unzip: %s not found in %s", sub, src)
	}
	return nil
}

// ExtractOne extracts the first entry whose base name equals name
// (case-insensitive, e.g. "mod_fcgid.so") to the file dest.
func ExtractOne(src, name, dest string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("unzip: %w", err)
	}
	defer zr.Close()
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Base(filepath.FromSlash(strings.ReplaceAll(zf.Name, `\`, "/"))), name) {
			if err := extractFile(zf, dest); err != nil {
				return fmt.Errorf("unzip %s: %w", zf.Name, err)
			}
			return nil
		}
	}
	return fmt.Errorf("unzip: %s not found in %s", name, src)
}

// commonTopDir returns "dir/" if all entries share one top-level directory.
func commonTopDir(files []*zip.File) string {
	top := ""
	for _, f := range files {
		name := strings.TrimLeft(strings.ReplaceAll(f.Name, `\`, "/"), "/")
		i := strings.Index(name, "/")
		if i < 0 {
			// a file at the root: only allowed if it is the dir entry itself
			if f.FileInfo().IsDir() && (top == "" || top == name+"/") {
				top = name + "/"
				continue
			}
			return ""
		}
		d := name[:i+1]
		if top == "" {
			top = d
		} else if top != d {
			return ""
		}
	}
	return top
}
