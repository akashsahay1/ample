package php

import (
	"regexp"
	"strconv"
	"strings"
)

// Compare compares dotted versions numerically: -1 if a<b, 0 if equal, 1 if a>b.
// Missing components count as 0 ("8.3" == "8.3.0"); "8.10" > "8.9".
func Compare(a, b string) int {
	pa, pb := splitVersion(a), splitVersion(b)
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	return 0
}

func splitVersion(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	var out []int
	for _, p := range strings.Split(v, ".") {
		// leading digits only ("0RC1" -> 0)
		j := 0
		for j < len(p) && p[j] >= '0' && p[j] <= '9' {
			j++
		}
		n, _ := strconv.Atoi(p[:j])
		out = append(out, n)
		if j < len(p) {
			break
		}
	}
	return out
}

// MinorOf returns "8.3" for "8.3.12".
func MinorOf(full string) string {
	p := strings.Split(strings.TrimSpace(full), ".")
	if len(p) < 2 {
		return full
	}
	return p[0] + "." + p[1]
}

var minorRe = regexp.MustCompile(`^[0-9]{1,2}\.[0-9]{1,2}$`)

// ValidMinor reports whether s looks like "8.3".
func ValidMinor(s string) bool { return minorRe.MatchString(s) }

// IsEOL reports whether a PHP minor no longer receives security support
// (as of 2026 everything below 8.2).
func IsEOL(minor string) bool { return Compare(MinorOf(minor), "8.2") < 0 }

var zipVersionRe = regexp.MustCompile(`(?i)php-([0-9]+\.[0-9]+\.[0-9]+)`)

// versionFromZipName extracts "8.5.11" from "php-8.5.11-nts-Win32-vs17-x64.zip".
func versionFromZipName(name string) string {
	m := zipVersionRe.FindStringSubmatch(name)
	if m == nil {
		return ""
	}
	return m[1]
}
