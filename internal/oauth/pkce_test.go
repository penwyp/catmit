package oauth

import "testing"

func TestGeneratePKCE(t *testing.T) {
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE() error = %v", err)
	}
	if len(verifier) < 43 {
		t.Fatalf("verifier too short: %d", len(verifier))
	}
	if len(challenge) < 43 {
		t.Fatalf("challenge too short: %d", len(challenge))
	}
}

func TestGenerateStateAndNonce(t *testing.T) {
	state, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() error = %v", err)
	}
	nonce, err := GenerateNonce()
	if err != nil {
		t.Fatalf("GenerateNonce() error = %v", err)
	}
	if state == "" || nonce == "" {
		t.Fatalf("state/nonce must not be empty")
	}
	if state == nonce {
		t.Fatalf("state and nonce should differ")
	}
}
