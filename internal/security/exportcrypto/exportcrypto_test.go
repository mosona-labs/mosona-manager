package exportcrypto

import (
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	payload := map[string]any{
		"version": 1,
		"secret":  "ssh-key-material",
	}
	enc, err := EncryptJSON("export-pass-123", payload)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err = DecryptJSON("export-pass-123", enc, &out); err != nil {
		t.Fatal(err)
	}
	if out["secret"] != "ssh-key-material" {
		t.Fatalf("got %#v", out)
	}
}

func TestDecryptWrongPassword(t *testing.T) {
	enc, err := EncryptJSON("correct-password", map[string]string{"a": "b"})
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]string
	if err = DecryptJSON("wrong-password", enc, &out); err == nil {
		t.Fatal("expected decrypt failure")
	}
}