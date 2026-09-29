//go:build windows

package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestSecretKeyACL(t *testing.T) {
	p := filepath.Join(t.TempDir(), "k.key")
	if err := writeSecretPEM(p, "PRIVATE KEY", []byte{1}); err != nil {
		t.Fatal(err)
	}
	sd, err := windows.GetNamedSecurityInfo(p, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	sddl := sd.String()
	ctl, _, _ := sd.Control()
	if ctl&windows.SE_DACL_PROTECTED == 0 {
		t.Errorf("DACL not protected: %s", sddl)
	}
	sid, _ := currentUserSID()
	for _, bad := range []string{";;;BU)", ";;;AU)", ";;;WD)", ";;;S-1-5-32-545)", ";;;S-1-5-11)"} {
		if strings.Contains(sddl, bad) {
			t.Errorf("key readable by broad group %s: %s", bad, sddl)
		}
	}
	if !strings.Contains(sddl, sid) {
		t.Errorf("owner user %s missing: %s", sid, sddl)
	}
	if b, err := os.ReadFile(p); err != nil || !strings.Contains(string(b), "PRIVATE KEY") {
		t.Fatalf("current user cannot read key: %v", err)
	}
}

// ---- CryptoAPI chain validation with our CA as the only trusted root ----

var (
	crypt32                           = windows.NewLazySystemDLL("crypt32.dll")
	procCertCreateCertificateChainEng = crypt32.NewProc("CertCreateCertificateChainEngine")
	procCertFreeCertificateChainEng   = crypt32.NewProc("CertFreeCertificateChainEngine")
)

type chainEngineConfig struct {
	cbSize                    uint32
	hRestrictedRoot           windows.Handle
	hRestrictedTrust          windows.Handle
	hRestrictedOther          windows.Handle
	cAdditionalStore          uint32
	rghAdditionalStore        uintptr
	dwFlags                   uint32
	dwUrlRetrievalTimeout     uint32
	MaximumCachedCertificates uint32
	CycleDetectionModulus     uint32
	hExclusiveRoot            windows.Handle
	hExclusiveTrustedPeople   windows.Handle
	dwExclusiveFlags          uint32
}

const (
	certTrustInvalidNameConstraints       = 0x00000800
	certTrustHasNotSupportedNameConstrain = 0x00002000
	certTrustHasNotDefinedNameConstraint  = 0x00004000
	certTrustHasNotPermittedNameConstrain = 0x00008000
	certTrustHasExcludedNameConstraint    = 0x00010000
	certTrustIsNotValidForUsage           = 0x00000010
	nameConstraintErrors                  = certTrustInvalidNameConstraints | certTrustHasNotSupportedNameConstrain |
		certTrustHasNotDefinedNameConstraint | certTrustHasNotPermittedNameConstrain | certTrustHasExcludedNameConstraint
)

// capiChainStatus builds leaf's chain with CryptoAPI trusting only ca and
// returns the chain's TrustStatus.ErrorStatus.
func capiChainStatus(t *testing.T, ca, leaf *x509.Certificate, usage string) uint32 {
	t.Helper()
	if err := procCertCreateCertificateChainEng.Find(); err != nil {
		t.Skip(err)
	}
	root, err := windows.CertOpenStore(windows.CERT_STORE_PROV_MEMORY, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CertCloseStore(root, 0)
	caCtx, err := windows.CertCreateCertificateContext(windows.X509_ASN_ENCODING|windows.PKCS_7_ASN_ENCODING, &ca.Raw[0], uint32(len(ca.Raw)))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CertFreeCertificateContext(caCtx)
	if err := windows.CertAddCertificateContextToStore(root, caCtx, windows.CERT_STORE_ADD_ALWAYS, nil); err != nil {
		t.Fatal(err)
	}
	cfg := chainEngineConfig{hExclusiveRoot: root}
	cfg.cbSize = uint32(unsafe.Sizeof(cfg))
	var engine windows.Handle
	if r, _, e := procCertCreateCertificateChainEng.Call(uintptr(unsafe.Pointer(&cfg)), uintptr(unsafe.Pointer(&engine))); r == 0 {
		t.Skipf("CertCreateCertificateChainEngine: %v", e)
	}
	defer procCertFreeCertificateChainEng.Call(uintptr(engine))

	leafCtx, err := windows.CertCreateCertificateContext(windows.X509_ASN_ENCODING|windows.PKCS_7_ASN_ENCODING, &leaf.Raw[0], uint32(len(leaf.Raw)))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CertFreeCertificateContext(leafCtx)
	oid, _ := windows.BytePtrFromString(usage)
	para := windows.CertChainPara{}
	para.Size = uint32(unsafe.Sizeof(para))
	para.RequestedUsage.Type = windows.USAGE_MATCH_TYPE_AND
	para.RequestedUsage.Usage.Length = 1
	para.RequestedUsage.Usage.UsageIdentifiers = &oid
	var chain *windows.CertChainContext
	if err := windows.CertGetCertificateChain(engine, leafCtx, nil, 0, &para, 0, 0, &chain); err != nil {
		t.Fatal(err)
	}
	defer windows.CertFreeCertificateChain(chain)
	return chain.TrustStatus.ErrorStatus
}

func TestCAConstraintsAcceptedByCryptoAPI(t *testing.T) {
	if err := EnsureCA(); err != nil {
		t.Fatal(err)
	}
	ca, key, err := LoadCA()
	if err != nil {
		t.Fatal(err)
	}
	certFile, _, err := EnsureSiteCert("capi.test")
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := readCert(certFile)
	if err != nil {
		t.Fatal(err)
	}
	const serverAuth = "1.3.6.1.5.5.7.3.1"
	const codeSigning = "1.3.6.1.5.5.7.3.3"
	if st := capiChainStatus(t, ca, leaf, serverAuth); st != 0 {
		t.Fatalf("CryptoAPI rejected a genuine .test leaf: ErrorStatus=%#x", st)
	}
	// a leaf for a real name minted with the (leaked) CA key must fail name constraints
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	evilTpl := &x509.Certificate{
		SerialNumber: big.NewInt(42), Subject: pkix.Name{CommonName: "www.microsoft.com"},
		DNSNames:  []string{"www.microsoft.com"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageCodeSigning},
	}
	der, err := x509.CreateCertificate(rand.Reader, evilTpl, ca, &k.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	evil, _ := x509.ParseCertificate(der)
	if st := capiChainStatus(t, ca, evil, serverAuth); st&nameConstraintErrors == 0 {
		t.Fatalf("CryptoAPI accepted www.microsoft.com from the local CA: ErrorStatus=%#x", st)
	}
	if st := capiChainStatus(t, ca, evil, codeSigning); st&certTrustIsNotValidForUsage == 0 {
		t.Fatalf("CryptoAPI accepted code signing from the local CA: ErrorStatus=%#x", st)
	}
}
