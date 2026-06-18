package engine

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/aystro/apod/internal/totp"
)

const recoveryCodeCount = 8

// totpCode / verifyTOTP are thin wrappers used by tests and the engine.
func totpCode(secret string, t time.Time) (string, error) { return totp.Code(secret, t) }

func verifyTOTP(secret, code string, t time.Time) (uint64, bool) {
	return totp.Verify(secret, code, t)
}

// Setup2FA generates a fresh secret and otpauth URI but does NOT enable 2FA —
// the user must confirm with a valid code via Enable2FA. Re-running it before
// confirmation rotates the pending secret.
func (e *Engine) Setup2FA(name string) (secret, uri string, err error) {
	if _, err := e.db.GetUserByName(name); err != nil {
		return "", "", err
	}
	secret, err = totp.NewSecret()
	if err != nil {
		return "", "", err
	}
	if err := e.db.SetUserTOTPSecret(name, secret); err != nil {
		return "", "", err
	}
	return secret, totp.URI(secret, name, "apod"), nil
}

// Enable2FA confirms setup with a current code, turns on enforcement, and
// returns one-time recovery codes (shown to the user once).
func (e *Engine) Enable2FA(name, code string) ([]string, error) {
	secret, _, err := e.db.GetUserTOTP(name)
	if err != nil {
		return nil, err
	}
	if secret == "" {
		return nil, fmt.Errorf("run 2FA setup first")
	}
	step, ok := totp.Verify(secret, code, time.Now())
	if !ok {
		return nil, fmt.Errorf("invalid code")
	}

	plain, hashed, err := generateRecoveryCodes()
	if err != nil {
		return nil, err
	}
	codesJSON, _ := json.Marshal(hashed)
	if err := e.db.SetUserRecoveryCodes(name, string(codesJSON)); err != nil {
		return nil, err
	}
	_ = step // enrollment doesn't set the replay floor; the first login does.
	if err := e.db.SetUserTOTPEnabled(name, true); err != nil {
		return nil, err
	}
	e.LogActivity("server", "user_2fa_enabled", fmt.Sprintf("2FA enabled for %s", name), "success")
	return plain, nil
}

// Disable2FA turns off 2FA after verifying a current code, wiping the secret
// and recovery codes.
func (e *Engine) Disable2FA(name, code string) error {
	secret, enabled, err := e.db.GetUserTOTP(name)
	if err != nil {
		return err
	}
	if !enabled {
		return fmt.Errorf("2FA is not enabled")
	}
	if _, ok := totp.Verify(secret, code, time.Now()); !ok {
		return fmt.Errorf("invalid code")
	}
	if err := e.db.ClearUserTOTP(name); err != nil {
		return err
	}
	e.LogActivity("server", "user_2fa_disabled", fmt.Sprintf("2FA disabled for %s", name), "success")
	return nil
}

// consumeSecondFactor accepts either a TOTP code (rejecting replays of an
// already-used step) or an unused recovery code, which it then burns.
func (e *Engine) consumeSecondFactor(name, secret, code string) bool {
	if step, ok := totp.Verify(secret, code, time.Now()); ok {
		last, _ := e.db.GetUserTOTPLastStep(name)
		if step <= last {
			return false // replay of this or an earlier step
		}
		e.db.SetUserTOTPLastStep(name, step)
		return true
	}
	return e.consumeRecoveryCode(name, code)
}

func (e *Engine) consumeRecoveryCode(name, code string) bool {
	raw, err := e.db.GetUserRecoveryCodes(name)
	if err != nil {
		return false
	}
	var hashes []string
	if json.Unmarshal([]byte(raw), &hashes) != nil {
		return false
	}
	for i, h := range hashes {
		if bcrypt.CompareHashAndPassword([]byte(h), []byte(code)) == nil {
			// Remove the used code.
			remaining := append(append([]string{}, hashes[:i]...), hashes[i+1:]...)
			out, _ := json.Marshal(remaining)
			e.db.SetUserRecoveryCodes(name, string(out))
			return true
		}
	}
	return false
}

// generateRecoveryCodes returns plaintext codes (for the user) and their
// bcrypt hashes (for storage).
func generateRecoveryCodes() (plain, hashed []string, err error) {
	for i := 0; i < recoveryCodeCount; i++ {
		// 16 bytes = 128 bits of entropy, well above online-brute-force range.
		buf := make([]byte, 16)
		if _, err := rand.Read(buf); err != nil {
			return nil, nil, fmt.Errorf("generate recovery code: %w", err)
		}
		code := hex.EncodeToString(buf) // 32 hex chars
		h, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
		if err != nil {
			return nil, nil, err
		}
		plain = append(plain, code)
		hashed = append(hashed, string(h))
	}
	return plain, hashed, nil
}

// GetUserTOTPStatus exposes whether 2FA is enabled (for /auth/me).
func (e *Engine) GetUserTOTPStatus(name string) (secret string, enabled bool, err error) {
	return e.db.GetUserTOTP(name)
}
