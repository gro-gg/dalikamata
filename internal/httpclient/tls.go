package httpclient

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// NewHTTPClient returns an *http.Client that trusts the system CA pool
// plus any .pem/.crt/.cer files found in certsDir.
// If certsDir is empty, returns &http.Client{} unchanged.
// If certsDir is non-empty but no cert files are found, returns an error.
func NewHTTPClient(certsDir string) (*http.Client, error) {
	if certsDir == "" {
		return &http.Client{}, nil
	}

	entries, err := os.ReadDir(certsDir)
	if err != nil {
		return nil, fmt.Errorf("reading CA certs directory %q: %w", certsDir, err)
	}

	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("loading system certificate pool: %w", err)
	}

	loaded := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".pem" && ext != ".crt" && ext != ".cer" {
			continue
		}
		pemPath := filepath.Join(certsDir, entry.Name())
		pemData, err := os.ReadFile(pemPath) //nolint:gosec // user-supplied CA cert directory by design
		if err != nil {
			return nil, fmt.Errorf("reading CA cert file %q: %w", pemPath, err)
		}
		if !pool.AppendCertsFromPEM(pemData) {
			return nil, fmt.Errorf("CA cert file %q contained no valid PEM certificate blocks", pemPath)
		}
		loaded++
	}

	if loaded == 0 {
		return nil, fmt.Errorf("no CA certificate files (.pem, .crt, .cer) found in %q", certsDir)
	}

	return &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}, nil
}
