package storage

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"strings"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// startSFTPServer brings up an in-process SSH server with the sftp subsystem,
// rooted at the real filesystem (so absolute paths under a temp dir work). It
// returns the host, port and the server's public key in authorized_keys line
// format (for host-key pinning).
func startSFTPServer(t *testing.T, user, password string) (host, port, hostKeyLine string) {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	hostKeyLine = string(ssh.MarshalAuthorizedKey(signer.PublicKey()))

	cfg := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == user && string(pass) == password {
				return nil, nil
			}
			return nil, errAuth
		},
	}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveSFTPConn(conn, cfg)
		}
	}()

	h, p, _ := net.SplitHostPort(ln.Addr().String())
	return h, p, hostKeyLine
}

var errAuth = &authError{}

type authError struct{}

func (*authError) Error() string { return "auth failed" }

func serveSFTPConn(nConn net.Conn, cfg *ssh.ServerConfig) {
	sshConn, chans, reqs, err := ssh.NewServerConn(nConn, cfg)
	if err != nil {
		return
	}
	defer sshConn.Close()
	go ssh.DiscardRequests(reqs)

	for newCh := range chans {
		if newCh.ChannelType() != "session" {
			newCh.Reject(ssh.UnknownChannelType, "only sessions")
			continue
		}
		ch, requests, err := newCh.Accept()
		if err != nil {
			continue
		}
		go func(in <-chan *ssh.Request) {
			for req := range in {
				// Accept the "sftp" subsystem request.
				ok := req.Type == "subsystem" && len(req.Payload) >= 4 &&
					string(req.Payload[4:]) == "sftp"
				if req.WantReply {
					req.Reply(ok, nil)
				}
			}
		}(requests)

		server, err := sftp.NewServer(ch)
		if err != nil {
			continue
		}
		go func() { _ = server.Serve(); ch.Close() }()
	}
}

func newSFTPStorage(t *testing.T, base string) *SFTPStorage {
	t.Helper()
	host, port, hostKey := startSFTPServer(t, "u", "pw")
	s, err := NewSFTP(map[string]string{
		"host": host, "port": port, "user": "u", "password": "pw",
		"path": base, "host_key": hostKey,
	})
	if err != nil {
		t.Fatalf("NewSFTP: %v", err)
	}
	return s
}

func TestSFTPRoundTrip(t *testing.T) {
	base := t.TempDir()
	s := newSFTPStorage(t, base)
	ctx := context.Background()

	if err := s.Upload(ctx, "site/a.zip", bytes.NewReader([]byte("backup-bytes"))); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	var buf bytes.Buffer
	if err := s.Download(ctx, "site/a.zip", &buf); err != nil {
		t.Fatalf("Download: %v", err)
	}
	if buf.String() != "backup-bytes" {
		t.Errorf("downloaded %q, want backup-bytes", buf.String())
	}
}

func TestSFTPListAndDelete(t *testing.T) {
	base := t.TempDir()
	s := newSFTPStorage(t, base)
	ctx := context.Background()

	s.Upload(ctx, "site/a.zip", bytes.NewReader([]byte("a")))
	s.Upload(ctx, "site/b.zip", bytes.NewReader([]byte("b")))

	keys, err := s.List(ctx, "site/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("got %d keys, want 2: %v", len(keys), keys)
	}

	if err := s.Delete(ctx, "site/a.zip"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	keys, _ = s.List(ctx, "site/")
	if len(keys) != 1 {
		t.Errorf("after delete got %d keys, want 1", len(keys))
	}
}

func TestSFTPHostKeyPinningRejectsWrongKey(t *testing.T) {
	base := t.TempDir()
	host, port, _ := startSFTPServer(t, "u", "pw")

	// Pin a DIFFERENT key than the server actually presents.
	_, otherPriv, _ := ed25519.GenerateKey(rand.Reader)
	otherSigner, _ := ssh.NewSignerFromKey(otherPriv)
	wrongLine := string(ssh.MarshalAuthorizedKey(otherSigner.PublicKey()))

	s, err := NewSFTP(map[string]string{
		"host": host, "port": port, "user": "u", "password": "pw",
		"path": base, "host_key": wrongLine,
	})
	if err != nil {
		t.Fatalf("NewSFTP: %v", err)
	}

	err = s.Upload(context.Background(), "x.zip", strings.NewReader("x"))
	if err == nil {
		t.Fatal("connection with a mismatched pinned host key must fail (MITM guard)")
	}
}

func TestSFTPWrongPasswordFails(t *testing.T) {
	base := t.TempDir()
	host, port, hostKey := startSFTPServer(t, "u", "pw")
	s, _ := NewSFTP(map[string]string{
		"host": host, "port": port, "user": "u", "password": "WRONG",
		"path": base, "host_key": hostKey,
	})
	var buf bytes.Buffer
	if err := s.Download(context.Background(), "x", &buf); err == nil {
		t.Error("expected auth failure with wrong password")
	}
}
