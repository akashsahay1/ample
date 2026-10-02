package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"apnoro/internal/paths"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "apnoro-certs-test")
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
	if ca.Subject.CommonName != CAName || !constrained(ca) {
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
	for _, d := range []string{"", "..\\evil", "a/b.test", "single", "*.x.test", "google.com", "blog.dev", "test.com"} {
		if _, _, err := EnsureSiteCert(d); err == nil {
			t.Errorf("accepted %q", d)
		}
	}
}

func TestIsCATrustedNoPanic(t *testing.T) {
	_ = IsCATrusted() // freshly generated CA must not be trusted, but must not crash
}

// A leaked CA key must not be able to mint certificates that verify for real
// internet names, for IPs other than loopback, or for code signing.
func TestCANameConstraints(t *testing.T) {
	if err := EnsureCA(); err != nil {
		t.Fatal(err)
	}
	ca, key, err := LoadCA()
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	mint := func(dns []string, ips []net.IP, eku x509.ExtKeyUsage) *x509.Certificate {
		t.Helper()
		k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		tpl := &x509.Certificate{
			SerialNumber: big.NewInt(time.Now().UnixNano()),
			Subject:      pkix.Name{CommonName: "x"},
			DNSNames:     dns, IPAddresses: ips,
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{eku},
		}
		der, err := x509.CreateCertificate(rand.Reader, tpl, ca, &k.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		c, _ := x509.ParseCertificate(der)
		return c
	}
	verify := func(c *x509.Certificate, eku x509.ExtKeyUsage) error {
		_, err := c.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{eku}})
		return err
	}
	if err := verify(mint([]string{"blog.test", "*.blog.test"}, nil, x509.ExtKeyUsageServerAuth), x509.ExtKeyUsageServerAuth); err != nil {
		t.Errorf(".test leaf rejected: %v", err)
	}
	if err := verify(mint([]string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1"), net.IPv6loopback}, x509.ExtKeyUsageServerAuth), x509.ExtKeyUsageServerAuth); err != nil {
		t.Errorf("localhost leaf rejected: %v", err)
	}
	for _, dns := range [][]string{{"www.google.com"}, {"blog.test", "login.microsoftonline.com"}, {"test.com"}} {
		if verify(mint(dns, nil, x509.ExtKeyUsageServerAuth), x509.ExtKeyUsageServerAuth) == nil {
			t.Errorf("leaf for %v verified", dns)
		}
	}
	if verify(mint([]string{"a.test"}, []net.IP{net.ParseIP("8.8.8.8")}, x509.ExtKeyUsageServerAuth), x509.ExtKeyUsageServerAuth) == nil {
		t.Error("leaf with public IP verified")
	}
	if verify(mint([]string{"a.test"}, nil, x509.ExtKeyUsageCodeSigning), x509.ExtKeyUsageCodeSigning) == nil {
		t.Error("code-signing leaf verified")
	}
}

func TestUnconstrainedCAReplaced(t *testing.T) {
	// simulate a CA from an older build without constraints
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "old"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(5 * 365 * 24 * time.Hour),
		KeyUsage: x509.KeyUsageCertSign, BasicConstraintsValid: true, IsCA: true,
	}
	der, _ := x509.CreateCertificate(rand.Reader, tpl, tpl, &k.PublicKey, k)
	kder, _ := x509.MarshalPKCS8PrivateKey(k)
	if err := writeSecretPEM(CAKeyPath(), "PRIVATE KEY", kder); err != nil {
		t.Fatal(err)
	}
	if err := writePEM(CAPath(), "CERTIFICATE", der); err != nil {
		t.Fatal(err)
	}
	if err := EnsureCA(); err != nil {
		t.Fatal(err)
	}
	ca, _, err := LoadCA()
	if err != nil || !constrained(ca) || ca.Subject.CommonName != CAName {
		t.Fatalf("old CA not replaced: %v %v", err, ca.Subject)
	}
}

func TestWritePEMDoesNotFollowPlantedTemp(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.crt")
	// a predictable temp name from older code must be irrelevant now
	os.WriteFile(p+".tmp", []byte("planted"), 0o644)
	if err := writePEM(p, "CERTIFICATE", []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p + ".tmp"); string(b) != "planted" {
		t.Fatal("planted temp file was written through")
	}
	if b, _ := os.ReadFile(p); !strings.Contains(string(b), "BEGIN CERTIFICATE") {
		t.Fatal("target not written")
	}
}
