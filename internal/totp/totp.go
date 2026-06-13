// Package totp implements RFC 6238 time-based one-time passwords
// (SHA-1, 30-second step, 6 digits) with no external dependencies.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"time"
)

const (
	stepSeconds = 30
	digits      = 1000000 // 6 digits
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewSecret returns a 160-bit random secret, base32-encoded.
func NewSecret() (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate totp secret: %w", err)
	}
	return b32.EncodeToString(buf), nil
}

// URI builds the otpauth:// enrollment URI consumed by authenticator apps.
func URI(secret, account, issuer string) string {
	return fmt.Sprintf(
		"otpauth://totp/%s:%s?secret=%s&issuer=%s&algorithm=SHA1&digits=6&period=30",
		url.PathEscape(issuer), url.PathEscape(account), secret, url.QueryEscape(issuer),
	)
}

// Code computes the 6-digit code for the given time.
func Code(secret string, t time.Time) (string, error) {
	key, err := b32.DecodeString(secret)
	if err != nil {
		return "", fmt.Errorf("invalid totp secret")
	}
	return hotp(key, uint64(t.Unix())/stepSeconds), nil
}

// Verify checks a code against the current step ±1 (clock drift tolerance).
// It returns the matched step so callers can persist it and reject replays.
func Verify(secret, code string, t time.Time) (step uint64, ok bool) {
	key, err := b32.DecodeString(secret)
	if err != nil || len(code) != 6 {
		return 0, false
	}
	current := uint64(t.Unix()) / stepSeconds
	for _, s := range []uint64{current, current - 1, current + 1} {
		expected := hotp(key, s)
		if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return s, true
		}
	}
	return 0, false
}

func hotp(key []byte, step uint64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], step)
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0xf
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%digits)
}
