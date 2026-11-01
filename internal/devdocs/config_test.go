package devdocs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRemoteURL(t *testing.T) {
	valid := []string{
		"git@github.com:owner/devdocs.git",
		"ssh://git@github.com/owner/devdocs.git",
		"https://github.com/owner/devdocs.git",
		"file:///var/git/devdocs.git",
		"/tmp/devdocs.git",
		"./local-repo.git",
	}

	for _, u := range valid {
		if err := ValidateRemoteURL(u); err != nil {
			t.Errorf("expected valid URL %q, got error: %v", u, err)
		}
	}

	invalid := []string{
		"",
		"--upload-pack=evil",
		"https://user:password@github.com/repo.git",
		"https://user@github.com/repo.git",
		"http://user:password@github.com/repo.git",
		"ssh://user:secret@github.com/repo.git",
		"user:secret@github.com:repo.git",
		"ext::sh -c unwanted-command",
	}

	for _, remote := range invalid {
		if err := ValidateRemoteURL(remote); err == nil {
			t.Errorf("accepted unsafe remote %q", remote)
		}
	}
}

func TestRedactRemoteURL(t *testing.T) {
	cases := []string{
		"https://user:pass@github.com/owner/repo.git",
		"http://user:secret123@example.com/repo.git",
		"ssh://git:mypassword@example.com/repo.git",
		"fatal: unable to access 'https://user:pass@example.com/repo.git': denied",
		"first https://user:pass@one.example/repo and ssh://git:mypassword@two.example/repo",
	}

	for _, u := range cases {
		got := RedactRemoteURL(u)
		if strings.Contains(got, "pass") || strings.Contains(got, "secret123") || strings.Contains(got, "mypassword") {
			t.Errorf("RedactRemoteURL(%q) leaked credentials: %q", u, got)
		}
	}
}

func TestTokenizeCommandLine(t *testing.T) {
	cases := []struct {
		input    string
		expected []string
	}{
		{"code --wait", []string{"code", "--wait"}},
		{`"my editor" --foo 'bar baz'`, []string{"my editor", "--foo", "bar baz"}},
		{`editor\ with\ spaces --flag`, []string{"editor with spaces", "--flag"}},
		{`subl -n -w`, []string{"subl", "-n", "-w"}},
	}

	for _, tc := range cases {
		tokens, err := TokenizeCommandLine(tc.input)
		if err != nil {
			t.Fatalf("unexpected error tokenizing %q: %v", tc.input, err)
		}
		if len(tokens) != len(tc.expected) {
			t.Fatalf("token count mismatch for %q: got %v, expected %v", tc.input, tokens, tc.expected)
		}
		for i, tok := range tokens {
			if tok != tc.expected[i] {
				t.Errorf("token %d mismatch for %q: got %q, expected %q", i, tc.input, tok, tc.expected[i])
			}
		}
	}

	// Malformed input
	if _, err := TokenizeCommandLine(`"unmatched quote`); err == nil {
		t.Errorf("expected error for unmatched double quote, got nil")
	}
	if _, err := TokenizeCommandLine(`'unmatched single quote`); err == nil {
		t.Errorf("expected error for unmatched single quote, got nil")
	}
}

func TestLoadAndSaveConfig(t *testing.T) {
	temp := t.TempDir()
	configDir := filepath.Join(temp, "custom-config")
	t.Setenv("XDG_CONFIG_HOME", configDir)

	// Initial load should return empty config
	cfg, path, err := LoadConfig()
	if err != nil {
		t.Fatalf("loading initial config: %v", err)
	}
	if cfg.DefaultRemote != "" {
		t.Errorf("expected empty initial default remote, got %q", cfg.DefaultRemote)
	}
	expectedPath := filepath.Join(configDir, "devdocs", "config.json")
	if path != expectedPath {
		t.Errorf("config path mismatch: got %q, expected %q", path, expectedPath)
	}

	// Save config
	cfg.DefaultRemote = "git@github.com:test/repo.git"
	savedPath, err := SaveConfig(cfg)
	if err != nil {
		t.Fatalf("saving config: %v", err)
	}
	if savedPath != expectedPath {
		t.Errorf("saved path mismatch: got %q, expected %q", savedPath, expectedPath)
	}

	// Reload config
	reloaded, _, err := LoadConfig()
	if err != nil {
		t.Fatalf("reloading config: %v", err)
	}
	if reloaded.DefaultRemote != cfg.DefaultRemote {
		t.Errorf("reloaded remote mismatch: got %q, expected %q", reloaded.DefaultRemote, cfg.DefaultRemote)
	}

	// Corrupt config file
	if err := os.WriteFile(expectedPath, []byte("{invalid json"), 0644); err != nil {
		t.Fatalf("corrupting config file: %v", err)
	}
	if _, _, err := LoadConfig(); err == nil {
		t.Errorf("expected error loading corrupted config file, got nil")
	}
}
