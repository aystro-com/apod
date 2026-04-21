package engine

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

// generateSupabaseJWT creates an HS256-signed JWT compatible with Supabase.
func generateSupabaseJWT(secret, role string) string {
	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	now := time.Now().Unix()
	payload := map[string]interface{}{
		"role": role,
		"iss":  "supabase",
		"iat":  now,
		"exp":  now + 5*365*24*3600, // 5 years
	}

	headerJSON, _ := json.Marshal(header)
	payloadJSON, _ := json.Marshal(payload)

	headerB64 := base64URLEncode(headerJSON)
	payloadB64 := base64URLEncode(payloadJSON)

	signingInput := headerB64 + "." + payloadB64
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	sig := base64URLEncode(mac.Sum(nil))

	return signingInput + "." + sig
}

func base64URLEncode(data []byte) string {
	s := base64.StdEncoding.EncodeToString(data)
	s = strings.TrimRight(s, "=")
	s = strings.ReplaceAll(s, "+", "-")
	s = strings.ReplaceAll(s, "/", "_")
	return s
}

func randomBase64(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

