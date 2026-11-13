package tests

import (
	"strings"
	"testing"

	"github.com/maximhq/bifrost/plugins/replaylab/plugins/replaylab"
)

func TestRedactorPIIScrubbing(t *testing.T) {
	redactor := replaylab.NewRedactor(true)

	sample := "Contact engineer alice.smith@acme-corp.com or call +1 (555) 234-5678. API key: sk-abc12345678901234567890. Card: 4111222233334444."
	cleaned := redactor.Redact(sample)

	if strings.Contains(cleaned, "alice.smith@acme-corp.com") {
		t.Errorf("Email was not redacted: %s", cleaned)
	}
	if strings.Contains(cleaned, "555") {
		t.Errorf("Phone number was not redacted: %s", cleaned)
	}
	if strings.Contains(cleaned, "sk-abc") {
		t.Errorf("API key was not redacted: %s", cleaned)
	}
	if strings.Contains(cleaned, "4111222233334444") {
		t.Errorf("Credit card was not redacted: %s", cleaned)
	}

	if !strings.Contains(cleaned, "[REDACTED_EMAIL]") {
		t.Errorf("Expected [REDACTED_EMAIL] token")
	}
	if !strings.Contains(cleaned, "[REDACTED_API_KEY]") {
		t.Errorf("Expected [REDACTED_API_KEY] token")
	}
}

func TestHashStringDeterministic(t *testing.T) {
	text := "Sensitive prompt content"
	hash1 := replaylab.HashString(text)
	hash2 := replaylab.HashString(text)

	if hash1 != hash2 {
		t.Errorf("Hashes are non-deterministic: %s != %s", hash1, hash2)
	}
	if len(hash1) != 64 {
		t.Errorf("Expected 64-char sha256 hex string, got len %d", len(hash1))
	}
}
