package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type SFTPStorage struct {
	host         string
	port         string
	user         string
	password     string
	basePath     string
	hostKey      string
	insecureHost bool
}

// sftpConn bundles the sftp client with its underlying ssh connection so a
// single Close() tears down both — the ssh conn was previously leaked per op.
type sftpConn struct {
	*sftp.Client
	ssh *ssh.Client
}

func (c *sftpConn) Close() error {
	err := c.Client.Close()
	c.ssh.Close()
	return err
}

func NewSFTP(config map[string]string) (*SFTPStorage, error) {
	host := config["host"]
	if host == "" {
		return nil, fmt.Errorf("sftp: host is required")
	}
	port := config["port"]
	if port == "" {
		port = "22"
	}
	user := config["user"]
	if user == "" {
		return nil, fmt.Errorf("sftp: user is required")
	}
	password := config["password"]
	basePath := config["path"]
	if basePath == "" {
		basePath = "/backups"
	}

	insecure := strings.EqualFold(strings.TrimSpace(config["insecure_skip_host_key"]), "true")
	hostKey := strings.TrimSpace(config["host_key"])
	// Fail closed: a missing host key means the server's identity can't be
	// verified, so the SFTP password and the backup stream are exposed to a MITM.
	// Require either a pinned key or an explicit opt-in.
	if hostKey == "" && !insecure {
		return nil, fmt.Errorf("sftp: host_key is required (pin the server's key in 'host_key', " +
			"or set 'insecure_skip_host_key=true' to accept any key — not recommended)")
	}

	return &SFTPStorage{
		host:     host,
		port:     port,
		user:     user,
		password: password,
		basePath: basePath,
		// Optional pinned host key in authorized_keys / known_hosts line
		// format (e.g. "ssh-ed25519 AAAA..."). When set, the server's key is
		// verified against it, preventing MITM/credential capture.
		hostKey:      hostKey,
		insecureHost: insecure,
	}, nil
}

func (s *SFTPStorage) connect() (*sftpConn, error) {
	hostKeyCallback, err := s.hostKeyCallback()
	if err != nil {
		return nil, err
	}
	config := &ssh.ClientConfig{
		User: s.user,
		Auth: []ssh.AuthMethod{
			ssh.Password(s.password),
		},
		HostKeyCallback: hostKeyCallback,
	}

	addr := fmt.Sprintf("%s:%s", s.host, s.port)
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("sftp connect: %w", err)
	}

	client, err := sftp.NewClient(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("sftp client: %w", err)
	}

	return &sftpConn{Client: client, ssh: conn}, nil
}

// hostKeyCallback returns a verifying callback when a host key is pinned, or the
// accept-any callback only when the operator explicitly opted in (NewSFTP
// already rejects the no-key/no-opt-in case).
func (s *SFTPStorage) hostKeyCallback() (ssh.HostKeyCallback, error) {
	if s.hostKey == "" {
		log.Printf("WARNING: sftp storage %q is using insecure_skip_host_key — the connection is MITM-able", s.host)
		return ssh.InsecureIgnoreHostKey(), nil
	}
	pk, _, _, _, err := ssh.ParseAuthorizedKey([]byte(s.hostKey))
	if err != nil {
		return nil, fmt.Errorf("sftp: invalid host_key: %w", err)
	}
	return ssh.FixedHostKey(pk), nil
}

func (s *SFTPStorage) Upload(_ context.Context, key string, reader io.Reader) error {
	client, err := s.connect()
	if err != nil {
		return err
	}
	defer client.Close()

	remotePath, err := safeJoin(s.basePath, key)
	if err != nil {
		return err
	}
	client.MkdirAll(filepath.Dir(remotePath))

	f, err := client.Create(remotePath)
	if err != nil {
		return fmt.Errorf("sftp create: %w", err)
	}
	defer f.Close()

	_, err = io.Copy(f, reader)
	return err
}

func (s *SFTPStorage) Download(_ context.Context, key string, writer io.Writer) error {
	client, err := s.connect()
	if err != nil {
		return err
	}
	defer client.Close()

	remotePath, err := safeJoin(s.basePath, key)
	if err != nil {
		return err
	}
	f, err := client.Open(remotePath)
	if err != nil {
		return fmt.Errorf("sftp open: %w", err)
	}
	defer f.Close()

	_, err = io.Copy(writer, f)
	return err
}

func (s *SFTPStorage) Delete(_ context.Context, key string) error {
	client, err := s.connect()
	if err != nil {
		return err
	}
	defer client.Close()

	remotePath, err := safeJoin(s.basePath, key)
	if err != nil {
		return err
	}
	return client.Remove(remotePath)
}

func (s *SFTPStorage) List(_ context.Context, prefix string) ([]string, error) {
	client, err := s.connect()
	if err != nil {
		return nil, err
	}
	defer client.Close()

	dir := filepath.Join(s.basePath, prefix)
	entries, err := client.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("sftp readdir: %w", err)
	}

	var keys []string
	for _, entry := range entries {
		if !entry.IsDir() {
			keys = append(keys, strings.TrimPrefix(filepath.Join(prefix, entry.Name()), "/"))
		}
	}
	return keys, nil
}
