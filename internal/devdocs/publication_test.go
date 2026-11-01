package devdocs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCleanSyncRequiresReceiveService(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	for _, args := range [][]string{{"init"}, {"sync"}} {
		if code, out, errOut := runCLI(ctx, env.codeDir, args...); code != 0 {
			t.Fatalf("setup command failed: %s %s", out, errOut)
		}
	}
	docs := filepath.Join(env.codeDir, "devdocs")
	before, err := RunGitOutput(ctx, docs, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	// The fetch service remains available; publication is not available.
	if err := RunGit(ctx, docs, "config", "remote.origin.receivepack", "false"); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := runCLI(ctx, env.codeDir, "sync"); code == 0 {
		t.Fatal("clean sync claimed success without checking the receive service")
	}
	after, err := RunGitOutput(ctx, docs, "rev-parse", "HEAD")
	if err != nil || after != before {
		t.Fatalf("clean failed sync changed local history: %q, %v", after, err)
	}
}

func TestSyncRejectsAdditionalPushRemoteBeforeSnapshot(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	if code, out, errOut := runCLI(ctx, env.codeDir, "init"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}
	docs := filepath.Join(env.codeDir, "devdocs")
	state, err := ReadLocalState(docs)
	if err != nil {
		t.Fatal(err)
	}
	before, err := RunGitOutput(ctx, docs, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(t.TempDir(), "foreign.git")
	if err := RunGit(ctx, "", "init", "--bare", foreign); err != nil {
		t.Fatal(err)
	}
	for _, remote := range []string{foreign, state.Remote} {
		if err := RunGit(ctx, docs, "config", "--add", "remote.origin.pushurl", remote); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(docs, "private-note.md"), []byte("must stay local\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := runCLI(ctx, env.codeDir, "sync"); code == 0 {
		t.Fatal("sync accepted an additional unpinned push remote")
	}
	after, err := RunGitOutput(ctx, docs, "rev-parse", "HEAD")
	if err != nil || after != before {
		t.Fatalf("invalid remote configuration mutated local history: %q, %v", after, err)
	}
	staged, err := RunGitOutput(ctx, docs, "diff", "--cached", "--name-only")
	if err != nil || staged != "" {
		t.Fatalf("invalid remote configuration staged private files: %q, %v", staged, err)
	}
	refs, err := RunGitOutput(ctx, foreign, "for-each-ref", "--format=%(refname)")
	if err != nil || refs != "" {
		t.Fatalf("private history reached the foreign remote: %q, %v", refs, err)
	}
}
