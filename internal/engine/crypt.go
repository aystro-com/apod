package engine

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/scrypt"
)

// backupEncMagic prefixes encrypted backup archives so encrypted and legacy
// plaintext backups can be told apart on read.
const backupEncMagic = "APODENC1"

// passEncMagic prefixes passphrase-encrypted exports. Exports move between hosts
// (where the instance key is unavailable), so they use a passphrase-derived key
// instead of the instance backup key.
const passEncMagic = "APODENCP"

// isPassphraseEncrypted reports whether data is a passphrase-encrypted archive.
func isPassphraseEncrypted(data []byte) bool {
	return bytes.HasPrefix(data, []byte(passEncMagic))
}

// scryptParams are deliberately strong; an export is encrypted/decrypted once.
const (
	scryptN = 1 << 15
	scryptR = 8
	scryptP = 1
)

// encryptWithPassphrase encrypts data with AES-256-GCM under a scrypt-derived
// key, prefixing the magic, the salt and the nonce.
func encryptWithPassphrase(passphrase string, plaintext []byte) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key, err := scrypt.Key([]byte(passphrase), salt, scryptN, scryptR, scryptP, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := append([]byte(passEncMagic), salt...)
	out = append(out, nonce...)
	return gcm.Seal(out, nonce, plaintext, nil), nil
}

// decryptWithPassphrase reverses encryptWithPassphrase. Data without the magic
// is returned unchanged (a plaintext export).
func decryptWithPassphrase(passphrase string, data []byte) ([]byte, error) {
	if !isPassphraseEncrypted(data) {
		return data, nil
	}
	data = data[len(passEncMagic):]
	if len(data) < 16 {
		return nil, fmt.Errorf("encrypted export is truncated")
	}
	salt, rest := data[:16], data[16:]
	key, err := scrypt.Key([]byte(passphrase), salt, scryptN, scryptR, scryptP, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(rest) < ns {
		return nil, fmt.Errorf("encrypted export is truncated")
	}
	plaintext, err := gcm.Open(nil, rest[:ns], rest[ns:], nil)
	if err != nil {
		return nil, Invalid("could not decrypt export (wrong passphrase?)")
	}
	return plaintext, nil
}

// backupKey returns the instance's backup encryption key, generating and
// persisting a fresh 32-byte key (AES-256) on first use. The key lives in the
// data directory at 0600 — back it up out-of-band; without it, encrypted
// backups cannot be restored.
func (e *Engine) backupKey() ([]byte, error) {
	path := filepath.Join(e.dataDir, "backup.key")
	if data, err := os.ReadFile(path); err == nil && len(data) == 32 {
		return data, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate backup key: %w", err)
	}
	if err := os.MkdirAll(e.dataDir, 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0600); err != nil {
		return nil, fmt.Errorf("persist backup key: %w", err)
	}
	return key, nil
}

// decryptBackupBytes decrypts a downloaded backup using the instance key. A
// plaintext (legacy) archive is returned unchanged.
func (e *Engine) decryptBackupBytes(data []byte) ([]byte, error) {
	if !bytes.HasPrefix(data, []byte(backupEncMagic)) {
		return data, nil // legacy plaintext backup
	}
	key, err := e.backupKey()
	if err != nil {
		return nil, fmt.Errorf("backup key: %w", err)
	}
	return decryptBackup(key, data)
}

// encryptBackup encrypts a backup archive with AES-256-GCM, prefixing the magic
// marker and the nonce. Backups contain databases and secrets, so they are
// encrypted at rest (local disk or remote object storage alike).
func encryptBackup(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := append([]byte(backupEncMagic), nonce...)
	return gcm.Seal(out, nonce, plaintext, nil), nil
}

// decryptBackup reverses encryptBackup. Data without the magic prefix is
// returned unchanged, so backups taken before encryption remain restorable.
func decryptBackup(key, data []byte) ([]byte, error) {
	if !bytes.HasPrefix(data, []byte(backupEncMagic)) {
		return data, nil
	}
	data = data[len(backupEncMagic):]
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return nil, fmt.Errorf("encrypted backup is truncated")
	}
	plaintext, err := gcm.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt backup (wrong key?): %w", err)
	}
	return plaintext, nil
}
