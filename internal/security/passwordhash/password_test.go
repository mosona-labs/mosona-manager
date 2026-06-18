package passwordhash

import (
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func TestHashAndVerifyArgon2id(t *testing.T) {
	stored, err := Hash("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !IsArgon2idHash(stored) {
		t.Fatalf("expected argon2id hash, got %q", stored)
	}
	ok, needsRehash, err := Verify("correct horse battery staple", stored, "", "")
	if err != nil || !ok || needsRehash {
		t.Fatalf("verify ok=%v needsRehash=%v err=%v", ok, needsRehash, err)
	}
	ok, _, _ = Verify("wrong", stored, "", "")
	if ok {
		t.Fatal("wrong password should fail")
	}
}

func TestLegacySHA256Verify(t *testing.T) {
	salt := "abcdefghijklmnop"
	token := "site-token-secret"
	password := "legacy-pass"
	legacy := legacySHA256Hex(password + salt + token)
	if !IsLegacySHA256Hash(legacy) {
		t.Fatal("legacy hash not recognized")
	}
	ok, needsRehash, err := Verify(password, legacy, salt, token)
	if err != nil || !ok || !needsRehash {
		t.Fatalf("legacy ok=%v needsRehash=%v err=%v", ok, needsRehash, err)
	}
	ok, _, _ = Verify("bad", legacy, salt, token)
	if ok {
		t.Fatal("bad legacy password should fail")
	}
}

func TestIsLegacySHA256Hash(t *testing.T) {
	if !IsLegacySHA256Hash(hex.EncodeToString(make([]byte, 32))) {
		t.Fatal("64 hex should be legacy")
	}
	if IsLegacySHA256Hash("not-hex") || IsLegacySHA256Hash("$argon2id$v=19$m=1,t=1,p=1$abc$def") {
		t.Fatal("non-legacy should not match")
	}
}

func TestArgon2ParamsCapped(t *testing.T) {
	saltB64 := base64.RawStdEncoding.EncodeToString(make([]byte, 16))
	hashB64 := base64.RawStdEncoding.EncodeToString(make([]byte, 32))
	cases := []string{
		"$argon2id$v=19$m=4294967295,t=3,p=2$" + saltB64 + "$" + hashB64,
		"$argon2id$v=19$m=65536,t=999,p=2$" + saltB64 + "$" + hashB64,
		"$argon2id$v=19$m=65536,t=3,p=99$" + saltB64 + "$" + hashB64,
	}
	for _, h := range cases {
		ok, _, err := Verify("x", h, "", "")
		if ok || err == nil {
			t.Fatalf("expected reject for oversized params ok=%v err=%v", ok, err)
		}
	}
}

func TestInvalidPHCNoPanic(t *testing.T) {
	cases := []string{
		"$argon2id$v=19$m=65536,t=3,p=2$$",
		"$argon2id$v=99$m=1,t=1,p=1$YWJj$ZGVm",
		"garbage",
	}
	for _, h := range cases {
		ok, needsRehash, err := Verify("x", h, "", "")
		if ok || needsRehash {
			t.Fatalf("hash %q should not verify", h)
		}
		_ = err
	}
}

func TestParsePHCParams(t *testing.T) {
	stored, err := Hash("p")
	if err != nil {
		t.Fatal(err)
	}
	_, params, _, err := parsePHC(stored)
	if err != nil {
		t.Fatal(err)
	}
	if params.memory != argon2MemoryKiB || params.iterations != argon2Iterations || params.parallelism != argon2Parallel {
		t.Fatalf("params mismatch: %+v", params)
	}
}
