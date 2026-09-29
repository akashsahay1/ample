package php

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"ampls/internal/download"
)

const (
	ReleasesJSON = "https://windows.php.net/downloads/releases/releases.json"
	ReleasesBase = "https://windows.php.net/downloads/releases/"
	ArchivesBase = "https://windows.php.net/downloads/releases/archives/"
)

// Release is an installable PHP build (NTS x64).
type Release struct {
	Minor, Full, URL, SHA256 string
	Size                     int64
}

// archived holds final builds of EOL minors in case releases.json drops them
// (verified against the archives index, 2026-09).
var archived = []Release{
	{Minor: "8.1", Full: "8.1.34", URL: ArchivesBase + "php-8.1.34-nts-Win32-vs16-x64.zip", Size: 31 << 20},
	{Minor: "8.0", Full: "8.0.30", URL: ArchivesBase + "php-8.0.30-nts-Win32-vs16-x64.zip", Size: 26 << 20},
	{Minor: "7.4", Full: "7.4.33", URL: ArchivesBase + "php-7.4.33-nts-Win32-vc15-x64.zip", Size: 25 << 20},
}

// Available lists installable NTS x64 builds, newest first.
func Available(ctx context.Context) ([]Release, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := download.Get(ctx, ReleasesJSON)
	if err != nil {
		return nil, fmt.Errorf("php: fetch releases: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("php: fetch releases: %w", err)
	}
	rels, err := parseReleases(b)
	if err != nil {
		return nil, err
	}
	return mergeArchived(rels), nil
}

type zipInfo struct {
	Path   string `json:"path"`
	Size   string `json:"size"`
	SHA256 string `json:"sha256"`
}

// parseReleases parses windows.php.net releases.json.
func parseReleases(b []byte) ([]Release, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(b, &top); err != nil {
		return nil, fmt.Errorf("php: parse releases.json: %w", err)
	}
	var out []Release
	for minor, raw := range top {
		if !ValidMinor(minor) || Compare(minor, "7.4") < 0 {
			continue
		}
		var entry map[string]json.RawMessage
		if err := json.Unmarshal(raw, &entry); err != nil {
			continue
		}
		var full string
		_ = json.Unmarshal(entry["version"], &full)
		// deterministic pick: newest toolchain key wins (vs17 > vs16)
		keys := make([]string, 0, len(entry))
		for k := range entry {
			if strings.HasPrefix(k, "nts-") && strings.HasSuffix(k, "-x64") {
				keys = append(keys, k)
			}
		}
		sort.Sort(sort.Reverse(sort.StringSlice(keys)))
		for _, k := range keys {
			var build struct {
				Zip zipInfo `json:"zip"`
			}
			if err := json.Unmarshal(entry[k], &build); err != nil || build.Zip.Path == "" {
				continue
			}
			if full == "" {
				full = versionFromZipName(build.Zip.Path)
			}
			out = append(out, Release{
				Minor:  minor,
				Full:   full,
				URL:    ReleasesBase + build.Zip.Path,
				SHA256: strings.ToLower(build.Zip.SHA256),
				Size:   parseSize(build.Zip.Size),
			})
			break
		}
	}
	sortReleases(out)
	return out, nil
}

func mergeArchived(rels []Release) []Release {
	have := map[string]bool{}
	for _, r := range rels {
		have[r.Minor] = true
	}
	for _, a := range archived {
		if !have[a.Minor] {
			rels = append(rels, a)
		}
	}
	sortReleases(rels)
	return rels
}

func sortReleases(r []Release) {
	sort.Slice(r, func(i, j int) bool { return Compare(r[i].Minor, r[j].Minor) > 0 })
}

// parseSize converts "25.02MB" / "900KB" / "12345" to bytes.
func parseSize(s string) int64 {
	s = strings.ToUpper(strings.TrimSpace(s))
	mult := 1.0
	for _, u := range []struct {
		suf string
		m   float64
	}{{"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10}, {"B", 1}} {
		if strings.HasSuffix(s, u.suf) {
			mult = u.m
			s = strings.TrimSpace(strings.TrimSuffix(s, u.suf))
			break
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(f * mult)
}

// archiveURL maps a releases/ URL to its archives/ equivalent (older builds
// move there once superseded).
func archiveURL(url string) string {
	if strings.HasPrefix(url, ReleasesBase) && !strings.HasPrefix(url, ArchivesBase) {
		return ArchivesBase + strings.TrimPrefix(url, ReleasesBase)
	}
	return ""
}
