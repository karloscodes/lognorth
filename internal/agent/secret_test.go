package agent

import "testing"

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := "an-instance-private-key"
	token := "github_pat_11ABCDEFG0_someSecretTokenValue123"

	enc, err := Encrypt(token, key)

	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc == "" || enc == token {
		t.Fatalf("ciphertext looks wrong: %q", enc)
	}

	dec, err := Decrypt(enc, key)

	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if dec != token {
		t.Fatalf("round-trip mismatch: got %q want %q", dec, token)
	}
}

func TestDecryptWithWrongKeyFails(t *testing.T) {
	enc, _ := Encrypt("a-secret", "key-a")

	if _, err := Decrypt(enc, "key-b"); err == nil {
		t.Fatal("expected decrypt with the wrong key to fail, got nil")
	}
}

func TestEmptyValuesRoundTrip(t *testing.T) {
	if enc, err := Encrypt("", "k"); err != nil || enc != "" {
		t.Fatalf("empty encrypt: %q, %v", enc, err)
	}
	if dec, err := Decrypt("", "k"); err != nil || dec != "" {
		t.Fatalf("empty decrypt: %q, %v", dec, err)
	}
}
