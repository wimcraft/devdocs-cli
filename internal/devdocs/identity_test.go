package devdocs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUUIDv4(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		id, err := NewUUID()
		if err != nil {
			t.Fatalf("NewUUID error: %v", err)
		}
		if seen[id] {
			t.Fatalf("duplicate UUID generated: %s", id)
		}
		seen[id] = true

		if err := ValidateUUID(id); err != nil {
			t.Fatalf("generated UUID %q failed validation: %v", id, err)
		}

		// Check version 4 (char 14) and variant 1 (char 19 in [8,9,a,b])
		if id[14] != '4' {
			t.Errorf("UUID %q version char is %c, expected '4'", id, id[14])
		}
		variant := id[19]
		if variant != '8' && variant != '9' && variant != 'a' && variant != 'b' {
			t.Errorf("UUID %q variant char is %c, expected RFC4122 variant", id, variant)
		}
	}

	invalidUUIDs := []string{
		"",
		"not-a-uuid",
		"12345678-1234-1234-1234-123456789abc", // Version 1, not 4
		"c1f7b9f8-7440-4966-2679-05a9ee8fe835", // Variant 2
		"c1f7b9f8-7440-4966-8679-05a9ee8fe835-extra", // Too long
	}

	for _, id := range invalidUUIDs {
		if err := ValidateUUID(id); err == nil {
			t.Errorf("expected UUID %q to fail validation, got nil", id)
		}
	}
}

func TestSlugSanitizationAndValidation(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"my-project", "my-project"},
		{"My Project!", "my-project"},
		{"Payments & Billing v2", "payments-billing-v2"},
		{"devdocs.lock", "devdocs-lock"},
		{"   spaced   ", "spaced"},
		{"", "project"},
		{"...", "project"},
	}

	for _, tc := range cases {
		got := SanitizeSlug(tc.input)
		if got != tc.expected {
			t.Errorf("SanitizeSlug(%q) = %q, expected %q", tc.input, got, tc.expected)
		}
		if err := ValidateSlug(got); err != nil {
			t.Errorf("SanitizeSlug(%q) produced invalid slug %q: %v", tc.input, got, err)
		}
	}

	invalidSlugs := []string{
		"",
		".hidden",
		"bad.lock",
		"consecutive..dots",
		"has space",
		"has:colon",
		"has~tilde",
		"has^caret",
		"has?question",
		"has*star",
		"has[bracket",
		"has@{reflog",
	}

	for _, s := range invalidSlugs {
		if err := ValidateSlug(s); err == nil {
			t.Errorf("expected ValidateSlug(%q) to fail, got nil", s)
		}
	}
}

func TestBranchNameFormattingAndParsing(t *testing.T) {
	id, err := NewUUID()
	if err != nil {
		t.Fatalf("NewUUID error: %v", err)
	}

	branch := ProjectBranchName("payments", id)
	expectedPrefix := "devdocs/payments-"
	if !strings.HasPrefix(branch, expectedPrefix) {
		t.Errorf("branch %q does not start with %q", branch, expectedPrefix)
	}

	slug, parsedUUID, err := ParseBranchName(branch)
	if err != nil {
		t.Fatalf("ParseBranchName(%q) failed: %v", branch, err)
	}
	if slug != "payments" {
		t.Errorf("parsed slug %q, expected 'payments'", slug)
	}
	if parsedUUID != strings.ToLower(id) {
		t.Errorf("parsed UUID %q, expected %q", parsedUUID, id)
	}

	invalidBranches := []string{
		"master",
		"main",
		"devdocs/",
		"devdocs/no-uuid",
		"devdocs/slug-12345",
		"refs/heads/devdocs/payments-" + id,
	}

	for _, b := range invalidBranches {
		if _, _, err := ParseBranchName(b); err == nil {
			t.Errorf("expected ParseBranchName(%q) to fail, got nil", b)
		}
	}
}

func TestMetadataValidation(t *testing.T) {
	id, _ := NewUUID()
	branch := ProjectBranchName("test-proj", id)

	validMeta := &Metadata{
		SchemaVersion: 1,
		ProjectID:     id,
		Branch:        branch,
		Name:          "Test Project",
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		Links: []LinkDeclaration{
			{Source: "AGENTS.md", Destination: "AGENTS.md"},
			{Source: "skills", Destination: ".claude/skills"},
		},
	}

	if err := ValidateMetadata(validMeta); err != nil {
		t.Fatalf("expected valid metadata, got: %v", err)
	}

	// Unsupported schema version
	metaBadSchema := *validMeta
	metaBadSchema.SchemaVersion = 2
	if err := ValidateMetadata(&metaBadSchema); err == nil {
		t.Errorf("expected error for schema_version 2, got nil")
	}

	// Mismatched UUID in branch
	otherID, _ := NewUUID()
	metaMismatchedBranch := *validMeta
	metaMismatchedBranch.Branch = ProjectBranchName("test-proj", otherID)
	if err := ValidateMetadata(&metaMismatchedBranch); err == nil {
		t.Errorf("expected error for mismatched branch UUID, got nil")
	}

	// Overlapping destination links
	metaOverlapping := *validMeta
	metaOverlapping.Links = []LinkDeclaration{
		{Source: "a", Destination: "dir"},
		{Source: "b", Destination: "dir/sub"},
	}
	if err := ValidateMetadata(&metaOverlapping); err == nil {
		t.Errorf("expected error for overlapping link destinations, got nil")
	}
}

func TestRejectSymlinkedMetadataAndState(t *testing.T) {
	temp := t.TempDir()
	docsDir := filepath.Join(temp, "devdocs")
	_ = os.MkdirAll(filepath.Join(docsDir, ".git"), 0755)

	targetFile := filepath.Join(temp, "real-metadata.json")
	_ = os.WriteFile(targetFile, []byte(`{"schema_version":1}`), 0644)

	// Symlink .devdocs.json
	metaSymlink := filepath.Join(docsDir, ".devdocs.json")
	_ = os.Symlink(targetFile, metaSymlink)

	if _, err := ReadMetadata(docsDir); err == nil {
		t.Errorf("expected ReadMetadata to reject symlinked .devdocs.json, got nil")
	}

	// Symlink devdocs-state.json
	stateSymlink := filepath.Join(docsDir, ".git", "devdocs-state.json")
	_ = os.Symlink(targetFile, stateSymlink)

	if _, err := ReadLocalState(docsDir); err == nil {
		t.Errorf("expected ReadLocalState to reject symlinked devdocs-state.json, got nil")
	}
}
