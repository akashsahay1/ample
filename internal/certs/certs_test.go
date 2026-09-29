package certs

import (
	"crypto/x509"
	"os"
	"strings"
	"testing"
	"time"

	"ampls/internal/paths"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "ampls-certs-test")
	if err != nil {
		panic(err)
	}
	paths.SetHome(dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func TestCAAndLeaf(t *testing.T) {
	if err := EnsureCA(); err != nil {
		t.Fatal(err)
	}
	ca, _, err := LoadCA()
	if err != nil {
		t.Fatal(err)
	}
	if !ca.IsCA || !ca.MaxPathLenZero || ca.KeyUsage&x509.KeyUsageCertSign == 0 || ca.KeyUsage&x509.KeyUsageCRLSign == 0 {
		t.Fatalf("bad CA: %+v", ca)
	}
	if !strings.HasPrefix(ca.Subject.CommonName, "AMPLS Local CA ") {
		t.Fatalf("CN %q", ca.Subject.CommonName)
	}
	if d := time.Until(ca.NotAfter); d < 9*365*24*time.Hour {
		t.Fatalf("CA validity %v", d)
	}
	// EnsureCA is idempotent
	if err := EnsureCA(); err != nil {
		t.Fatal(err)
	}
	ca2, _, _ := LoadCA()
	if !ca2.Equal(ca) {
		t.Fatal("CA regenerated")
	}

	certFile, keyFile, err := EnsureSiteCert("blog.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keyFile); err != nil {
		t.Fatal(err)
	}
	leaf, err := readCert(certFile)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	for _, host := range []string{"blog.test", "api.blog.test"} {
		if _, err := leaf.Verify(x509.VerifyOptions{DNSName: host, Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
			t.Errorf("verify %s: %v", host, err)
		}
	}
	if _, err := leaf.Verify(x509.VerifyOptions{DNSName: "other.test", Roots: pool}); err == nil {
		t.Error("verified for wrong host")
	}
	if d := leaf.NotAfter.Sub(leaf.NotBefore); d < 824*24*time.Hour || d > 826*24*time.Hour {
		t.Errorf("leaf validity %v", d)
	}

	// reused while valid
	c2, _, err := EnsureSiteCert("blog.test")
	if err != nil {
		t.Fatal(err)
	}
	leaf2, _ := readCert(c2)
	if !leaf2.Equal(leaf) {
		t.Error("leaf not reused")
	}

	// a new CA invalidates the leaf
	os.Remove(CAPath())
	if err := EnsureCA(); err != nil {
		t.Fatal(err)
	}
	c3, _, _ := EnsureSiteCert("blog.test")
	leaf3, _ := readCert(c3)
	if leaf3.Equal(leaf) {
		t.Error("leaf not reissued after CA change")
	}

	if err := RemoveSiteCert("blog.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(certFile); !os.IsNotExist(err) {
		t.Error("cert not removed")
	}
	if RemoveSiteCert("blog.test") != nil {
		t.Error("remove of missing cert should be nil")
	}
}

func TestInvalidDomain(t *testing.T) {
	for _, d := range []string{"", "..\\evil", "a/b.test", "single", "*.x.test"} {
		if _, _, err := EnsureSiteCert(d); err == nil {
			t.Errorf("accepted %q", d)
		}
	}
}

func TestIsCATrustedNoPanic(t *testing.T) {
	_ = IsCATrusted() // freshly generated CA must not be trusted, but must not crash
}
