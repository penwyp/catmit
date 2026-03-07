package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// GeneratePKCE returns a verifier and S256 challenge pair.
func GeneratePKCE() (verifier string, challenge string, err error) {
	verifier, err = randomURLSafeString(64)
	if err != nil {
		return "", "", err
	}
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return verifier, challenge, nil
}

// GenerateState returns a random state value.
func GenerateState() (string, error) {
	return randomURLSafeString(32)
}

// GenerateNonce returns a random nonce value.
func GenerateNonce() (string, error) {
	return randomURLSafeString(32)
}

func randomURLSafeString(n int) (string, error) {
	if n <= 0 {
		return "", fmt.Errorf("invalid random string size: %d", n)
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
