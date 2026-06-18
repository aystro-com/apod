package engine

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/aystro/apod/internal/models"
)

const (
	sessionTokenPrefix = "apod_sess_"
	sessionTTL         = 24 * time.Hour
	minPasswordLength  = 8
)

// errInvalidCredentials is intentionally identical for unknown users, users
// without a password, and wrong passwords — no username enumeration.
var errInvalidCredentials = fmt.Errorf("invalid username or password")

// ErrTwoFactorRequired signals that the password was correct but a valid 2FA
// code (or recovery code) must also be supplied. The "2fa_required" marker is
// matched by the HTTP layer and the UI.
var ErrTwoFactorRequired = fmt.Errorf("2fa_required: two-factor code required")

// ErrAccountLocked signals that the account is temporarily locked after too
// many failed login attempts. The HTTP layer maps it to 429.
var ErrAccountLocked = fmt.Errorf("account_locked: too many failed attempts")

// IsSessionToken reports whether a bearer token is a login session token
// (as opposed to a long-lived API key).
func IsSessionToken(token string) bool {
	return strings.HasPrefix(token, sessionTokenPrefix)
}

// SetUserPassword hashes and stores a login password for a user and revokes
// any existing sessions.
func (e *Engine) SetUserPassword(name, password string) error {
	if len(password) < minPasswordLength {
		return fmt.Errorf("password must be at least %d characters", minPasswordLength)
	}
	if _, err := e.db.GetUserByName(name); err != nil {
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	if err := e.db.SetUserPasswordHash(name, string(hash)); err != nil {
		return err
	}
	// A password change invalidates every existing session for the user.
	if err := e.db.DeleteSessionsForUser(name); err != nil {
		return err
	}

	e.LogActivity("server", "user_set_password", fmt.Sprintf("password updated for %s", name), "success")
	return nil
}

// LoginWithPassword verifies a username/password pair (and a 2FA code, if the
// user has 2FA enabled) and creates a session. It returns the raw session
// token (shown to the client once) and the user.
func (e *Engine) LoginWithPassword(name, password, code string) (string, *models.User, error) {
	// Refuse early when the account is locked from repeated failures. Keyed on
	// the submitted name (existing or not), so the response can't enumerate users.
	if e.loginThrottle.Locked(name) {
		return "", nil, ErrAccountLocked
	}

	hash, err := e.db.GetUserPasswordHash(name)
	if err != nil || hash == "" {
		// Burn comparable time for unknown users / users without a password
		// so responses don't reveal which usernames exist.
		bcrypt.CompareHashAndPassword(
			[]byte("$2a$10$7EqJtq98hPqEX7fNZaFWoOhi5B0xT1uYJ3mZqK5wW1nyG7uRr0FhO"),
			[]byte(password),
		)
		e.loginThrottle.RecordFailure(name)
		return "", nil, errInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		e.loginThrottle.RecordFailure(name)
		return "", nil, errInvalidCredentials
	}

	user, err := e.db.GetUserByName(name)
	if err != nil {
		e.loginThrottle.RecordFailure(name)
		return "", nil, errInvalidCredentials
	}

	// Second factor, when enabled. A wrong code counts as a failed attempt so
	// the second factor can't be brute-forced past a correct password.
	if secret, enabled, _ := e.db.GetUserTOTP(name); enabled {
		if code == "" {
			return "", nil, ErrTwoFactorRequired
		}
		if !e.consumeSecondFactor(name, secret, code) {
			e.loginThrottle.RecordFailure(name)
			return "", nil, errInvalidCredentials
		}
	}

	// Successful auth — clear the failure record.
	e.loginThrottle.Reset(name)

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", nil, fmt.Errorf("generate session token: %w", err)
	}
	rawToken := sessionTokenPrefix + hex.EncodeToString(tokenBytes)

	// Opportunistic cleanup keeps the table small without a background job.
	e.db.DeleteExpiredSessions()

	if err := e.db.CreateSession(HashAPIKey(rawToken), name, time.Now().Add(sessionTTL)); err != nil {
		return "", nil, fmt.Errorf("create session: %w", err)
	}

	e.LogActivity("server", "user_login", fmt.Sprintf("session created for %s", name), "success")
	return rawToken, user, nil
}

// ValidateSessionToken resolves a session token to its user, or (nil, nil)
// when the token is unknown or expired.
func (e *Engine) ValidateSessionToken(rawToken string) (*models.User, error) {
	if !IsSessionToken(rawToken) {
		return nil, nil
	}
	name, err := e.db.GetSessionUser(HashAPIKey(rawToken))
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, nil
	}
	return e.db.GetUserByName(name)
}

// Logout revokes a single session token.
func (e *Engine) Logout(rawToken string) error {
	return e.db.DeleteSession(HashAPIKey(rawToken))
}
