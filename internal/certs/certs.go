// Package certs maintains the Apnoro local certificate authority and the
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
	"net"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"apnoro/internal/paths"
)

const (
	caValidity   = 10 * 365 * 24 * time.Hour
	leafValidity = 825 * 24 * time.Hour
	renewBefore  = 30 * 24 * time.Hour
)

var mu sync.Mutex

// CAName is the CA certificate's exact Common Name.
const CAName = "Apnoro Local CA"

// PermittedDomains are the DNS name constraints of the CA: a constraint of
// "test" matches test and every name below it (RFC 5280 4.2.1.10).
var PermittedDomains = []string{"test", "localhost"}

func permittedIPRanges() []*net.IPNet {
	return []*net.IPNet{
		{IP: net.IPv4(127, 0, 0, 0).To4(), Mask: net.CIDRMask(8, 32)},
		{IP: net.IPv6loopback, Mask: net.CIDRMask(128, 128)},
	}
}

// permitted reports whether domain is inside PermittedDomains.
func permitted(domain string) bool {
	for _, p := range PermittedDomains {
		if domain == p || strings.HasSuffix(domain, "."+p) {
			return true
		}
	}
	return false
}

// constrained reports whether c carries the name and EKU constraints ensureCA
// puts on a new CA.
func constrained(c *x509.Certificate) bool {
	if !c.PermittedDNSDomainsCritical || len(c.PermittedIPRanges) == 0 || !c.MaxPathLenZero {
		return false
	}
	if len(c.ExtKeyUsage) != 1 || c.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		return false
	}
	have := map[string]bool{}
	for _, d := range c.PermittedDNSDomains {
		have[d] = true
	}
	for _, d := range PermittedDomains {
		if !have[d] {
			return false
		}
	}
	return len(c.PermittedDNSDomains) == len(PermittedDomains)
}

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

// writePEM writes a public PEM file via a fresh, exclusively created temp file
// (never following a link planted at a predictable name) and a rename.
func writePEM(path, typ string, der []byte) error {
	return writeAtomic(path, pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}), false)
}

// writeSecretPEM writes a private key readable only by the current user,
// SYSTEM and Administrators (a protected DACL on Windows, 0600 elsewhere). The
// data directory is writable by all local users, so the inherited ACL must
// not be used for keys.
func writeSecretPEM(path, typ string, der []byte) error {
	return writeAtomic(path, pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}), true)
}

func writeAtomic(path string, data []byte, secret bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var f *os.File
	var err error
	for i := 0; i < 10; i++ {
		var suffix [8]byte
		if _, err = rand.Read(suffix[:]); err != nil {
			return err
		}
		tmp := fmt.Sprintf("%s.%x.tmp", path, suffix)
		if f, err = createExclusive(tmp, secret); err == nil || !errors.Is(err, os.ErrExist) {
			break
		}
	}
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
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
	// An existing CA without the name/EKU constraints (created by an older
	// build), or still named after the former product (AMPLS Local CA), is
	// replaced; it then has to be trusted again and site certs are re-issued.
	if c, _, err := LoadCA(); err == nil && c.IsCA && time.Until(c.NotAfter) > renewBefore && constrained(c) && c.Subject.CommonName == CAName {
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
	tpl := &x509.Certificate{
		SerialNumber: sn,
		// The CN is exactly CAName so the uninstaller can remove it with
		// `certutil -delstore Root "Apnoro Local CA"`; the user goes in the OU.
		Subject: pkix.Name{
			CommonName:         CAName,
			Organization:       []string{"Apnoro local development CA"},
			OrganizationalUnit: []string{username()},
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(caValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
		// Constrain the CA so a leaked ca.key (it lives in the data directory)
		// can only mint TLS server certificates for *.test / localhost, never
		// for real sites or for code signing.
		ExtKeyUsage:                 []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		PermittedDNSDomainsCritical: true,
		PermittedDNSDomains:         PermittedDomains,
		PermittedIPRanges:           permittedIPRanges(),
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("certs: create CA: %w", err)
	}
	kder, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	if err := writeSecretPEM(CAKeyPath(), "PRIVATE KEY", kder); err != nil {
		return fmt.Errorf("certs: write CA key: %w", err)
	}
	if err := writePEM(CAPath(), "CERTIFICATE", der); err != nil {
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
	if !permitted(domain) {
		return "", "", fmt.Errorf("certs: %q is outside the local CA's permitted names (%s)", domain, strings.Join(PermittedDomains, ", "))
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
		Subject:      pkix.Name{CommonName: domain, Organization: []string{"Apnoro development certificate"}},
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
	if err := writeSecretPEM(keyFile, "PRIVATE KEY", kder); err != nil {
		return "", "", fmt.Errorf("certs: write key: %w", err)
	}
	if err := writePEM(certFile, "CERTIFICATE", der); err != nil {
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
