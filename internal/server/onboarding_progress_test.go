package server

import (
	"strings"
	"testing"
)

func TestDecodePatchOnboardingProgress(t *testing.T) {
	input, err := decodePatchOnboardingProgress(strings.NewReader(`{"seen":true,"dismissed":false,"ignored":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if !input.Seen || input.Dismissed || input.Completed {
		t.Errorf("unexpected input: %+v", input)
	}
	for _, body := range []string{`{}`, `null`, `[]`, `{"seen":false}`, `{"completed":"true"}`, `{`} {
		if _, err := decodePatchOnboardingProgress(strings.NewReader(body)); err == nil {
			t.Errorf("expected error for %s", body)
		}
	}
}

func TestPracticeCodeNormalization(t *testing.T) {
	if got := generateSlug("  My -- Practice! Name  ", true); got != "my-practice-name" {
		t.Errorf("sanitized code = %q", got)
	}
	if got := truncateToSegments(generateSlug("One Two Three Four", false), 3); got != "one-two-three" {
		t.Errorf("suggestion slug = %q", got)
	}
}
