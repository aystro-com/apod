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
	host     string
	port     string
	user     string
	password string
	basePath string
	hostKey  string
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

	return &SFTPStorage{
		host:     host,
		port:     port,
		user:     user,
		password: password,
		basePath: basePath,
		// Optional pinned host key in authorized_keys / known_hosts line
		// format (e.g. "ssh-ed25519 AAAA..."). When set, the server's key is
		// verified against it, preventing MITM/credential capture.
		hostKey: config["host_key"],
	}, nil
}

func (s *SFTPStorage) connect() (*sftp.Client, error) {
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

	return client, nil
}

// hostKeyCallback returns a verifying callback when a host key is pinned,
// otherwise it falls back to accept-any with a loud warning. Pin a host key in
// the storage config ("host_key") to protect backups from MITM.
func (s *SFTPStorage) hostKeyCallback() (ssh.HostKeyCallback, error) {
	if strings.TrimSpace(s.hostKey) == "" {
		log.Printf("WARNING: sftp storage %q has no pinned host_key — connection is vulnerable to MITM; set 'host_key' in the storage config", s.host)
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
