package cxd

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestRequestKeyUsesStableDigestAndRejectsInvalidID(t *testing.T) {
	if _, err := requestKey(""); err == nil {
		t.Fatal("empty request ID accepted")
	}
	if _, err := requestKey(string(make([]byte, 257))); err == nil {
		t.Fatal("overlong request ID accepted")
	}
	sum := sha256.Sum256([]byte("user-a/case-1"))
	want := hex.EncodeToString(sum[:])
	got, err := requestKey("user-a/case-1")
	if err != nil || got != want {
		t.Fatalf("request key = %q, %v; want %q", got, err, want)
	}
}
