package tlsconf

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSelfSigned(t *testing.T) {
	tests := []struct {
		name      string
		hosts     []string
		verifyDNS string
		verifyIP  string
	}{
		{name: "default localhost", hosts: nil, verifyDNS: "localhost"},
		{name: "single dns", hosts: []string{"mail.example.com"}, verifyDNS: "mail.example.com"},
		{name: "ip san", hosts: []string{"127.0.0.1"}, verifyIP: "127.0.0.1"},
		{name: "mixed", hosts: []string{"mail.example.com", "10.0.0.1"}, verifyDNS: "mail.example.com", verifyIP: "10.0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := SelfSigned(tt.hosts...)
			if err != nil {
				t.Fatalf("SelfSigned: %v", err)
			}
			if cfg.MinVersion != tls.VersionTLS12 {
				t.Errorf("MinVersion = %x, want %x", cfg.MinVersion, tls.VersionTLS12)
			}
			if len(cfg.Certificates) != 1 {
				t.Fatalf("got %d certificates, want exactly 1", len(cfg.Certificates))
			}

			leaf, err := x509.ParseCertificate(cfg.Certificates[0].Certificate[0])
			if err != nil {
				t.Fatalf("ParseCertificate: %v", err)
			}

			now := time.Now()
			if now.Before(leaf.NotBefore) || now.After(leaf.NotAfter) {
				t.Errorf("cert not currently valid: NotBefore=%v NotAfter=%v", leaf.NotBefore, leaf.NotAfter)
			}

			if tt.verifyDNS != "" {
				if err := leaf.VerifyHostname(tt.verifyDNS); err != nil {
					t.Errorf("VerifyHostname(%q): %v", tt.verifyDNS, err)
				}
			}
			if tt.verifyIP != "" {
				if err := leaf.VerifyHostname(tt.verifyIP); err != nil {
					t.Errorf("VerifyHostname(%q): %v", tt.verifyIP, err)
				}
			}
		})
	}
}

func TestSelfSignedEmptyHost(t *testing.T) {
	if _, err := SelfSigned(""); err == nil {
		t.Fatal("expected error for empty host, got nil")
	}
}

func TestConfig(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile, err := WritePEM(dir)
	if err != nil {
		t.Fatalf("WritePEM: %v", err)
	}

	tests := []struct {
		name      string
		certFile  string
		keyFile   string
		hosts     []string
		wantNil   bool
		wantCerts int
	}{
		{name: "disabled", wantNil: true},
		{name: "only certFile falls through to nil", certFile: certFile, wantNil: true},
		{name: "files", certFile: certFile, keyFile: keyFile, wantCerts: 1},
		{name: "self-signed hosts", hosts: []string{"localhost"}, wantCerts: 1},
		{name: "files preferred over hosts", certFile: certFile, keyFile: keyFile, hosts: []string{"localhost"}, wantCerts: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Config(tt.certFile, tt.keyFile, tt.hosts...)
			if err != nil {
				t.Fatalf("Config: %v", err)
			}
			if tt.wantNil {
				if cfg != nil {
					t.Fatalf("expected nil config, got %+v", cfg)
				}
				return
			}
			if cfg == nil {
				t.Fatal("expected non-nil config")
			}
			if len(cfg.Certificates) != tt.wantCerts {
				t.Errorf("got %d certificates, want %d", len(cfg.Certificates), tt.wantCerts)
			}
		})
	}
}

func TestWritePEMRoundTrip(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile, err := WritePEM(dir)
	if err != nil {
		t.Fatalf("WritePEM: %v", err)
	}
	if filepath.Dir(certFile) != dir || filepath.Dir(keyFile) != dir {
		t.Errorf("files not written to dir: %s %s", certFile, keyFile)
	}
	for _, f := range []string{certFile, keyFile} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("stat %s: %v", f, err)
		}
	}

	cfg, err := FromFiles(certFile, keyFile)
	if err != nil {
		t.Fatalf("FromFiles: %v", err)
	}
	if len(cfg.Certificates) != 1 {
		t.Fatalf("got %d certificates, want 1", len(cfg.Certificates))
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %x, want %x", cfg.MinVersion, tls.VersionTLS12)
	}
}

func TestFromFilesMissing(t *testing.T) {
	if _, err := FromFiles("/nonexistent/cert.pem", "/nonexistent/key.pem"); err == nil {
		t.Fatal("expected error for missing files, got nil")
	}
}

// TestHandshake proves a SelfSigned config actually serves TLS by running a tiny
// in-memory handshake over net.Pipe.
func TestHandshake(t *testing.T) {
	serverCfg, err := SelfSigned("localhost")
	if err != nil {
		t.Fatalf("SelfSigned: %v", err)
	}

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	const payload = "hello over tls"
	errCh := make(chan error, 1)

	go func() {
		srv := tls.Server(serverConn, serverCfg)
		defer srv.Close()
		if err := srv.Handshake(); err != nil {
			errCh <- err
			return
		}
		if _, err := io.WriteString(srv, payload); err != nil {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	cli := tls.Client(clientConn, &tls.Config{InsecureSkipVerify: true})
	if err := cli.Handshake(); err != nil {
		t.Fatalf("client handshake: %v", err)
	}

	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(cli, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != payload {
		t.Errorf("got %q, want %q", buf, payload)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("server: %v", err)
	}

	if v := cli.ConnectionState().Version; v < tls.VersionTLS12 {
		t.Errorf("negotiated version %x below TLS 1.2", v)
	}
}
