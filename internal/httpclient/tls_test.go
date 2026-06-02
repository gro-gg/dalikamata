package httpclient_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codeberg.org/aeforged/dalikamata/internal/httpclient"
)

// selfSignedPEM generates a minimal self-signed CA certificate and returns its
// PEM encoding. Used to populate the test cert directory.
func selfSignedPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-ca"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func writePEM(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestNewHTTPClient_EmptyCertsDir(t *testing.T) {
	cl, err := httpclient.NewHTTPClient("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cl == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNewHTTPClient_ValidCertFile(t *testing.T) {
	dir := t.TempDir()
	writePEM(t, dir, "ca.pem", selfSignedPEM(t))

	cl, err := httpclient.NewHTTPClient(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cl == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNewHTTPClient_CertFileExtensions(t *testing.T) {
	pem := selfSignedPEM(t)
	for _, ext := range []string{".pem", ".crt", ".cer"} {
		t.Run(ext, func(t *testing.T) {
			dir := t.TempDir()
			writePEM(t, dir, "ca"+ext, pem)
			cl, err := httpclient.NewHTTPClient(dir)
			if err != nil {
				t.Fatalf("ext %s: unexpected error: %v", ext, err)
			}
			if cl == nil {
				t.Fatalf("ext %s: expected non-nil client", ext)
			}
		})
	}
}

func TestNewHTTPClient_NonCertFilesIgnored(t *testing.T) {
	dir := t.TempDir()
	writePEM(t, dir, "ca.pem", selfSignedPEM(t))
	writePEM(t, dir, "readme.txt", []byte("not a cert"))

	_, err := httpclient.NewHTTPClient(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewHTTPClient_EmptyDir_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	_, err := httpclient.NewHTTPClient(dir)
	if err == nil {
		t.Fatal("expected error for empty certs dir, got nil")
	}
}

func TestNewHTTPClient_InvalidPEM_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	writePEM(t, dir, "bad.pem", []byte("this is not a PEM certificate"))

	_, err := httpclient.NewHTTPClient(dir)
	if err == nil {
		t.Fatal("expected error for invalid PEM, got nil")
	}
}

func TestNewHTTPClient_NonexistentDir_ReturnsError(t *testing.T) {
	_, err := httpclient.NewHTTPClient("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for nonexistent dir, got nil")
	}
}
