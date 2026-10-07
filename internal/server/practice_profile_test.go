package server

import (
	"bytes"
	"mime/multipart"
	"net/textproto"
	"testing"
)

func TestValidatePracticeCode(t *testing.T) {
	for _, tc := range []struct {
		code, want string
	}{
		{"valid-code1", ""},
		{"", "Practice code is required"},
		{"abc", "Practice code must be at least 4 characters"},
		{"UPPER", "Practice code can only contain lowercase letters, numbers, and hyphens"},
		{"bad_code", "Practice code can only contain lowercase letters, numbers, and hyphens"},
	} {
		if got := validatePracticeCode(tc.code); got != tc.want {
			t.Errorf("validatePracticeCode(%q) = %q, want %q", tc.code, got, tc.want)
		}
	}
}

func TestValidWebsite(t *testing.T) {
	if !validWebsite("https://example.test/path") || !validWebsite("http://example.test") {
		t.Error("expected HTTP websites to be valid")
	}
	if validWebsite("example.test") || validWebsite("javascript:alert(1)") {
		t.Error("expected invalid website to be rejected")
	}
}

func TestReadValidatedLogo(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 504)...)
	header := &multipart.FileHeader{Header: textproto.MIMEHeader{"Content-Type": {"image/png"}}}
	file := &memoryMultipartFile{Reader: bytes.NewReader(png)}
	data, contentType, err := readValidatedLogo(file, header)
	if err != nil || contentType != "image/png" || !bytes.Equal(data, png) {
		t.Fatalf("valid PNG result: type=%q err=%v", contentType, err)
	}

	header.Header.Set("Content-Type", "image/jpeg")
	if _, _, err := readValidatedLogo(&memoryMultipartFile{Reader: bytes.NewReader(png)}, header); err == nil {
		t.Error("mismatched submitted and detected MIME was accepted")
	}
}

func TestR2OwnedKey(t *testing.T) {
	storage := &r2ObjectStorage{publicHost: "media.example.test"}
	if key, ok := storage.OwnedKey("https://media.example.test/practice-id/logo_1", "practice-id"); !ok || key != "practice-id/logo_1" {
		t.Errorf("owned key = %q, %t", key, ok)
	}
	for _, value := range []string{
		"https://other.example.test/practice-id/logo_1",
		"https://media.example.test/other-practice/logo_1",
		"https://media.example.test/practice-id/../other/logo_1",
	} {
		if key, ok := storage.OwnedKey(value, "practice-id"); ok {
			t.Errorf("unsafe URL accepted as %q: %s", key, value)
		}
	}
}

type memoryMultipartFile struct{ *bytes.Reader }

func (*memoryMultipartFile) Close() error { return nil }
