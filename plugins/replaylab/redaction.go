package replaylab

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
)

var (
	// Email regex
	emailRegex = regexp.MustCompile(`(?i)\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)

	// Phone numbers (US, international formats)
	phoneRegex = regexp.MustCompile(`(?:\+?\d{1,3}[-.\s]?)?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}\b`)

	// API Keys & Tokens: sk-, tvly-, ghp-, bearer, authorization tokens
	apiKeyRegex = regexp.MustCompile(`(?i)\b(sk-[a-zA-Z0-9_\-]{20,}|tvly-[a-zA-Z0-9_\-]{20,}|ghp_[a-zA-Z0-9]{20,}|Bearer\s+[a-zA-Z0-9_\-\.]{20,})\b`)

	// Credit Card-like 13 to 19 digit blocks
	creditCardRegex = regexp.MustCompile(`\b(?:\d{4}[-\s]?){3}\d{4}\b|\b\d{15,16}\b`)
)

// Redactor sanitizes sensitive personal and credential information from strings.
type Redactor struct {
	enabled bool
}

// NewRedactor creates a configured Redactor.
func NewRedactor(enabled bool) *Redactor {
	return &Redactor{enabled: enabled}
}

// Redact cleans sensitive PII from input text.
func (r *Redactor) Redact(input string) string {
	if !r.enabled || len(input) == 0 {
		return input
	}

	cleaned := apiKeyRegex.ReplaceAllString(input, "[REDACTED_API_KEY]")
	cleaned = creditCardRegex.ReplaceAllString(cleaned, "[REDACTED_CREDIT_CARD]")
	cleaned = emailRegex.ReplaceAllString(cleaned, "[REDACTED_EMAIL]")
	cleaned = phoneRegex.ReplaceAllString(cleaned, "[REDACTED_PHONE]")

	return cleaned
}

// HashString computes a deterministic SHA256 hex digest for hash-only storage mode.
func HashString(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}
