// Package certs maintains the AMPLS local certificate authority and the
// per-site TLS certificates it signs (pure Go crypto/x509).
package certs

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"ampls/internal/paths"
)

const (
	caValidity   = 10 * 365 * 24 * time.Hour
	leafValidity = 825 * 24 * time.Hour
	renewBefore  = 30 * 24 * time.Hour
)

var mu sync.Mutex

// CAPath is the PEM CA certificate (certs/ca.crt).
func CAPath() string { return filepath.Join(paths.CertsDir(), "ca.crt") }

// CAKeyPath is the PEM CA private key (certs/ca.key).
func CAKeyPath() string { return filepath.Join(paths.CertsDir(), "ca.key") }

func sitesDir() string { return filepath.Join(paths.CertsDir(), "sites") }

var domainRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

// SiteCertPaths returns the certificate and key paths for domain.
func SiteCertPaths(domain string) (certFile, keyFile string) {
	return filepath.Join(sitesDir(), domain+".crt"), filepath.Join(sitesDir(), domain+".key")
}

func username() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		n := u.Username
		if i := strings.LastIndexAny(n, `\/`); i >= 0 {
			n = n[i+1:]
		}
		return n
	}
	for _, k := range []string{"USERNAME", "USER"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return "user"
}

func serial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
}

func writePEM(path, typ string, der []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}), perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readCert(path string) (*x509.Certificate, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	blk, _ := pem.Decode(b)
	if blk == nil || blk.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("%s: no certificate PEM block", path)
	}
	return x509.ParseCertificate(blk.Bytes)
}

func readKey(path string) (crypto.Signer, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	blk, _ := pem.Decode(b)
	if blk == nil {
		return nil, fmt.Errorf("%s: no PEM block", path)
	}
	var k any
	switch blk.Type {
	case "EC PRIVATE KEY":
		k, err = x509.ParseECPrivateKey(blk.Bytes)
	case "RSA PRIVATE KEY":
		k, err = x509.ParsePKCS1PrivateKey(blk.Bytes)
	default:
		k, err = x509.ParsePKCS8PrivateKey(blk.Bytes)
	}
	if err != nil {
		return nil, err
	}
	s, ok := k.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("%s: unsupported key type", path)
	}
	return s, nil
}

func samePublicKey(a crypto.PublicKey, b crypto.PublicKey) bool {
	type eq interface{ Equal(crypto.PublicKey) bool }
	e, ok := a.(eq)
	return ok && e.Equal(b)
}

// LoadCA returns the CA certificate and key.
func LoadCA() (*x509.Certificate, crypto.Signer, error) {
	c, err := readCert(CAPath())
	if err != nil {
		return nil, nil, err
	}
	k, err := readKey(CAKeyPath())
	if err != nil {
		return nil, nil, err
	}
	if !samePublicKey(k.Public(), c.PublicKey) {
		return nil, nil, errors.New("certs: CA key does not match certificate")
	}
	return c, k, nil
}

// EnsureCA creates certs/ca.crt + ca.key unless a valid pair already exists.
func EnsureCA() error {
	mu.Lock()
	defer mu.Unlock()
	return ensureCA()
}

func ensureCA() error {
	if c, _, err := LoadCA(); err == nil && c.IsCA && time.Until(c.NotAfter) > renewBefore {
		return nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("certs: generate CA key: %w", err)
	}
	sn, err := serial()
	if err != nil {
		return err
	}
	now := time.Now()
	name := "AMPLS Local CA " + username()
	tpl := &x509.Certificate{
		SerialNumber:          sn,
		Subject:               pkix.Name{CommonName: name, Organization: []string{"AMPLS local development CA"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(caValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("certs: create CA: %w", err)
	}
	kder, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	if err := writePEM(CAKeyPath(), "PRIVATE KEY", kder, 0o600); err != nil {
		return fmt.Errorf("certs: write CA key: %w", err)
	}
	if err := writePEM(CAPath(), "CERTIFICATE", der, 0o644); err != nil {
		return fmt.Errorf("certs: write CA: %w", err)
	}
	return nil
}

// EnsureSiteCert returns a certificate for domain and *.domain signed by the
// local CA, reusing the existing one while it is valid for more than 30 days
// and was issued by the current CA.
func EnsureSiteCert(domain string) (certFile, keyFile string, err error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if len(domain) > 253 || !domainRe.MatchString(domain) {
		return "", "", fmt.Errorf("certs: invalid domain %q", domain)
	}
	mu.Lock()
	defer mu.Unlock()
	if err := ensureCA(); err != nil {
		return "", "", err
	}
	ca, caKey, err := LoadCA()
	if err != nil {
		return "", "", fmt.Errorf("certs: load CA: %w", err)
	}
	certFile, keyFile = SiteCertPaths(domain)
	if leafOK(certFile, keyFile, ca, domain) {
		return certFile, keyFile, nil
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("certs: generate key: %w", err)
	}
	sn, err := serial()
	if err != nil {
		return "", "", err
	}
	now := time.Now()
	notAfter := now.Add(leafValidity)
	if notAfter.After(ca.NotAfter) {
		notAfter = ca.NotAfter
	}
	tpl := &x509.Certificate{
		SerialNumber: sn,
		Subject:      pkix.Name{CommonName: domain, Organization: []string{"AMPLS development certificate"}},
		DNSNames:     []string{domain, "*." + domain},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, ca, &key.PublicKey, caKey)
	if err != nil {
		return "", "", fmt.Errorf("certs: sign %s: %w", domain, err)
	}
	kder, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", err
	}
	if err := writePEM(keyFile, "PRIVATE KEY", kder, 0o600); err != nil {
		return "", "", fmt.Errorf("certs: write key: %w", err)
	}
	if err := writePEM(certFile, "CERTIFICATE", der, 0o644); err != nil {
		return "", "", fmt.Errorf("certs: write cert: %w", err)
	}
	return certFile, keyFile, nil
}

func leafOK(certFile, keyFile string, ca *x509.Certificate, domain string) bool {
	c, err := readCert(certFile)
	if err != nil {
		return false
	}
	k, err := readKey(keyFile)
	if err != nil || !samePublicKey(k.Public(), c.PublicKey) {
		return false
	}
	if time.Until(c.NotAfter) <= renewBefore || c.CheckSignatureFrom(ca) != nil {
		return false
	}
	return c.VerifyHostname(domain) == nil
}

// RemoveSiteCert deletes the certificate and key for domain.
func RemoveSiteCert(domain string) error {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if !domainRe.MatchString(domain) {
		return fmt.Errorf("certs: invalid domain %q", domain)
	}
	c, k := SiteCertPaths(domain)
	for _, p := range []string{c, k} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("certs: remove: %w", err)
		}
	}
	return nil
}
