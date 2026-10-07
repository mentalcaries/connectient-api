package server

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestTokenCipherRoundTrip(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	cipher, err := newTokenCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := cipher.Encrypt("secret-token")
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.Split(encrypted, ":")) != 3 || strings.Contains(encrypted, "secret-token") {
		t.Fatalf("invalid encrypted format: %q", encrypted)
	}
	decrypted, err := cipher.Decrypt(encrypted)
	if err != nil || decrypted != "secret-token" {
		t.Fatalf("decrypted=%q err=%v", decrypted, err)
	}
}

func TestTokenCipherRejectsInvalidKeyAndCiphertext(t *testing.T) {
	if _, err := newTokenCipher(base64.StdEncoding.EncodeToString([]byte("short"))); err == nil {
		t.Fatal("short key accepted")
	}
	cipher, _ := newTokenCipher(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	if _, err := cipher.Decrypt("invalid"); err == nil {
		t.Fatal("invalid ciphertext accepted")
	}
}
