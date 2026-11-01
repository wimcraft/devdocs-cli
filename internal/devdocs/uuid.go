package devdocs

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var uuidV4Regex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-4[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

// NewUUID generates a cryptographically secure RFC 4122 UUID v4.
func NewUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generating uuid: %w", err)
	}
	// Version 4: bits 12-15 of time_hi_and_version set to 0100
	b[6] = (b[6] & 0x0f) | 0x40
	// Variant 1: bits 6-7 of clock_seq_hi_and_reserved set to 10
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// ValidateUUID checks if a string is a valid UUID v4.
func ValidateUUID(id string) error {
	if !uuidV4Regex.MatchString(id) {
		return fmt.Errorf("invalid UUID v4: %q", id)
	}
	return nil
}

// SanitizeSlug converts a raw name or directory name into a safe Git ref component.
func SanitizeSlug(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "project"
	}

	var b strings.Builder
	lastDash := false

	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.' {
			b.WriteRune(unicode.ToLower(r))
			lastDash = false
		} else {
			// Replace any non-alphanumeric character with a dash
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}

	s := strings.Trim(b.String(), "-.")
	// Disallow ".lock" suffix
	if strings.HasSuffix(s, ".lock") {
		s = strings.TrimSuffix(s, ".lock") + "-lock"
	}
	if s == "" {
		return "project"
	}
	return s
}

// ValidateSlug verifies that a slug is a valid single Git ref component.
func ValidateSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("slug cannot be empty")
	}
	if strings.HasPrefix(slug, ".") || strings.HasSuffix(slug, ".") {
		return fmt.Errorf("slug cannot start or end with a dot: %q", slug)
	}
	if strings.HasSuffix(slug, ".lock") {
		return fmt.Errorf("slug cannot end with .lock: %q", slug)
	}
	if strings.Contains(slug, "..") {
		return fmt.Errorf("slug cannot contain consecutive dots '..': %q", slug)
	}
	if strings.ContainsAny(slug, " /:~^?*[\t\n\r\\@{") {
		return fmt.Errorf("slug contains invalid git ref characters: %q", slug)
	}
	return nil
}

// ProjectBranchName formats the standard branch name devdocs/<slug>-<uuid>.
func ProjectBranchName(slug, uuid string) string {
	return fmt.Sprintf("devdocs/%s-%s", slug, strings.ToLower(uuid))
}

// ParseBranchName extracts the slug and UUID from a branch name devdocs/<slug>-<uuid>.
func ParseBranchName(branch string) (slug, uuid string, err error) {
	if !strings.HasPrefix(branch, "devdocs/") {
		return "", "", fmt.Errorf("branch %q must start with 'devdocs/'", branch)
	}
	rest := strings.TrimPrefix(branch, "devdocs/")
	lastDash := strings.LastIndexByte(rest, '-')
	if lastDash < 0 {
		return "", "", fmt.Errorf("branch %q does not match pattern 'devdocs/<slug>-<uuid>'", branch)
	}
	// A UUID has 4 dashes: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	// So we need to look for the start of the UUID (36 characters at the end)
	if len(rest) < 37 || rest[len(rest)-37] != '-' {
		return "", "", fmt.Errorf("branch %q does not have a valid UUID suffix", branch)
	}
	splitIdx := len(rest) - 37
	slugPart := rest[:splitIdx]
	uuidPart := rest[splitIdx+1:]
	if err := ValidateUUID(uuidPart); err != nil {
		return "", "", fmt.Errorf("branch %q has invalid UUID: %w", branch, err)
	}
	if err := ValidateSlug(slugPart); err != nil {
		return "", "", fmt.Errorf("branch %q has invalid slug: %w", branch, err)
	}
	return slugPart, uuidPart, nil
}
