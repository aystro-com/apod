package engine

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestPassphraseEncryptRoundTrip(t *testing.T) {
	plain := []byte("PK\x03\x04 an export with .env and a db dump")

	enc, err := encryptWithPassphrase("correct horse battery staple", plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !isPassphraseEncrypted(enc) {
		t.Error("ciphertext should be detected as passphrase-encrypted")
	}
	if bytes.Contains(enc, plain) {
		t.Error("plaintext must not appear in ciphertext")
	}

	got, err := decryptWithPassphrase("correct horse battery staple", enc)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("round trip failed: %v", err)
	}

	// Wrong passphrase fails.
	if _, err := decryptWithPassphrase("wrong", enc); err == nil {
		t.Error("wrong passphrase must fail")
	}

	// Plaintext (no magic) passes through.
	if got, _ := decryptWithPassphrase("anything", plain); !bytes.Equal(got, plain) {
		t.Error("plaintext passthrough failed")
	}
}

func TestBackupEncryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	plain := []byte("PK\x03\x04 a backup archive with secrets")

	enc, err := encryptBackup(key, plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !bytes.HasPrefix(enc, []byte(backupEncMagic)) {
		t.Error("ciphertext should carry the magic prefix")
	}
	if bytes.Contains(enc, plain) {
		t.Error("plaintext must not appear in ciphertext")
	}

	got, err := decryptBackup(key, enc)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("round trip failed: got %q err %v", got, err)
	}

	// Wrong key fails (authenticated).
	bad := make([]byte, 32)
	rand.Read(bad)
	if _, err := decryptBackup(bad, enc); err == nil {
		t.Error("decrypt with wrong key must fail")
	}

	// Legacy plaintext (no magic) passes through unchanged.
	got, err = decryptBackup(key, plain)
	if err != nil || !bytes.Equal(got, plain) {
		t.Errorf("plaintext passthrough failed: %v", err)
	}
}
