package devdocs

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// testEnv sets up an isolated testing environment with dedicated HOME, config, and Git identity.
type testEnv struct {
	rootDir   string
	homeDir   string
	configDir string
	remoteDir string
	codeDir   string
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	root := t.TempDir()
	home := filepath.Join(root, "home")
	config := filepath.Join(home, ".config")
	remote := filepath.Join(root, "remote.git")
	code := filepath.Join(root, "code-repo")

	for _, d := range []string{home, config, remote, code} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("creating test dir %s: %v", d, err)
		}
	}

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test Author")
	t.Setenv("GIT_AUTHOR_EMAIL", "author@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test Committer")
	t.Setenv("GIT_COMMITTER_EMAIL", "committer@example.com")
	// Configure git author identity in global gitconfig
	_ = RunGit(context.Background(), "", "config", "--global", "user.name", "Test Author")
	_ = RunGit(context.Background(), "", "config", "--global", "user.email", "author@example.com")

	ctx := context.Background()

	// Initialize bare remote
	if err := RunGit(ctx, remote, "init", "--bare"); err != nil {
		t.Fatalf("initializing bare remote %s: %v", remote, err)
	}

	// Initialize code repository with an initial commit
	if err := RunGit(ctx, code, "init"); err != nil {
		t.Fatalf("initializing code repo: %v", err)
	}
	initialFile := filepath.Join(code, "main.go")
	if err := os.WriteFile(initialFile, []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("writing main.go: %v", err)
	}
	if err := RunGit(ctx, code, "add", "main.go"); err != nil {
		t.Fatalf("staging main.go: %v", err)
	}
	if err := RunGit(ctx, code, "commit", "-m", "initial commit in code repo"); err != nil {
		t.Fatalf("committing in code repo: %v", err)
	}

	// Set global devdocs config with default remote
	cfg := &Config{DefaultRemote: remote}
	if _, err := SaveConfig(cfg); err != nil {
		t.Fatalf("saving test config: %v", err)
	}

	return &testEnv{
		rootDir:   root,
		homeDir:   home,
		configDir: config,
		remoteDir: remote,
		codeDir:   code,
	}
}

// runCLI runs devdocs CLI within codeDir.
func runCLI(ctx context.Context, dir string, args ...string) (code int, stdout, stderr string) {
	var outBuf, errBuf bytes.Buffer
	origWd, _ := os.Getwd()
	if dir != "" {
		_ = os.Chdir(dir)
		defer func() { _ = os.Chdir(origWd) }()
	}

	code = Run(ctx, args, &bytes.Buffer{}, &outBuf, &errBuf)
	return code, outBuf.String(), errBuf.String()
}
