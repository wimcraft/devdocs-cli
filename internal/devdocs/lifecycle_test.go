package devdocs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitOffline(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// Get initial code repo commit hash
	codeHeadBefore, err := RunGitOutput(ctx, env.codeDir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("getting code HEAD before init: %v", err)
	}

	code, out, errOut := runCLI(ctx, env.codeDir, "init", "--name", "Alpha Project")
	if code != 0 {
		t.Fatalf("init failed (%d): %s %s", code, out, errOut)
	}

	// Code repository HEAD, index, and tracked files must be completely unchanged
	codeHeadAfter, err := RunGitOutput(ctx, env.codeDir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("getting code HEAD after init: %v", err)
	}
	if codeHeadBefore != codeHeadAfter {
		t.Errorf("code HEAD changed after init: was %s, now %s", codeHeadBefore, codeHeadAfter)
	}

	codeStatus, err := RunGitOutput(ctx, env.codeDir, "status", "--porcelain")
	if err != nil {
		t.Fatalf("checking code repo status: %v", err)
	}
	if codeStatus != "" {
		t.Errorf("code repo is dirty after init: %s", codeStatus)
	}

	// Docs repo must exist and be an independent git repo
	docsDir := filepath.Join(env.codeDir, "devdocs")
	meta, err := ReadMetadata(docsDir)
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}
	if meta.Name != "Alpha Project" {
		t.Errorf("metadata name mismatch: %q", meta.Name)
	}

	state, err := ReadLocalState(docsDir)
	if err != nil {
		t.Fatalf("reading state: %v", err)
	}
	if state.Published {
		t.Errorf("expected state.Published to be false for offline init")
	}

	// Repeat init must be idempotent
	code, outRepeat, errOutRepeat := runCLI(ctx, env.codeDir, "init", "--name", "Alpha Project")
	if code != 0 {
		t.Fatalf("repeat init failed (%d): %s %s", code, outRepeat, errOutRepeat)
	}
}

func TestInitIgnoredTemplateFails(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// Set up global git ignore rule that ignores FYI.md
	globalIgnoreDir := filepath.Join(env.homeDir, ".config", "git")
	_ = os.MkdirAll(globalIgnoreDir, 0755)
	globalIgnoreFile := filepath.Join(globalIgnoreDir, "ignore")
	if err := os.WriteFile(globalIgnoreFile, []byte("FYI.md\n"), 0644); err != nil {
		t.Fatalf("writing global git ignore: %v", err)
	}

	code, out, errOut := runCLI(ctx, env.codeDir, "init", "--name", "Ignore Fail")
	if code == 0 {
		t.Fatalf("expected init to fail when FYI.md is globally ignored, but succeeded: %s", out)
	}

	if !strings.Contains(errOut, "FYI.md") || !strings.Contains(errOut, "ignored") {
		t.Errorf("expected error message to identify ignored FYI.md, got: %s", errOut)
	}

	// devdocs directory must NOT exist (cleaned up staging)
	docsDir := filepath.Join(env.codeDir, "devdocs")
	if _, err := os.Stat(docsDir); !os.IsNotExist(err) {
		t.Errorf("devdocs directory was not cleaned up after failed init")
	}
}
func TestInitMissingAuthorIdentity(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// Unset global and local git author identity
	_ = RunGit(ctx, "", "config", "--global", "--unset-all", "user.name")
	_ = RunGit(ctx, "", "config", "--global", "--unset-all", "user.email")
	_ = RunGit(ctx, env.codeDir, "config", "--local", "--unset-all", "user.name")
	_ = RunGit(ctx, env.codeDir, "config", "--local", "--unset-all", "user.email")

	code, out, _ := runCLI(ctx, env.codeDir, "init", "--name", "No Author")
	if code == 0 {
		t.Fatalf("expected init to fail without author identity, but succeeded: %s", out)
	}
	if _, err := os.Lstat(filepath.Join(env.codeDir, "devdocs")); !os.IsNotExist(err) {
		t.Fatalf("missing author identity left an initialized workspace: %v", err)
	}
}
func TestInitMissingRemote(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// Clear default remote in config
	_, _ = SaveConfig(&Config{})

	code, out, _ := runCLI(ctx, env.codeDir, "init", "--name", "No Remote")
	if code == 0 {
		t.Fatalf("expected init to fail without default remote, but succeeded: %s", out)
	}
	if _, err := os.Lstat(filepath.Join(env.codeDir, "devdocs")); !os.IsNotExist(err) {
		t.Fatalf("missing remote left an initialized workspace: %v", err)
	}
}

func TestAttachSelectedBranch(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// 1. Initialize project A in repo 1 and sync to remote
	code, _, errOut := runCLI(ctx, env.codeDir, "init", "--name", "Project A")
	if code != 0 {
		t.Fatalf("init project A failed: %s", errOut)
	}
	metaA, err := ReadMetadata(filepath.Join(env.codeDir, "devdocs"))
	if err != nil {
		t.Fatalf("reading metadata A: %v", err)
	}

	// Add a note in project A
	_ = os.WriteFile(filepath.Join(env.codeDir, "devdocs", "note-a.md"), []byte("note A"), 0644)
	if code, _, errOut := runCLI(ctx, env.codeDir, "sync"); code != 0 {
		t.Fatalf("sync project A failed: %s", errOut)
	}

	// 2. In a second temporary directory, initialize project B and sync to remote
	codeDirB := filepath.Join(env.rootDir, "code-b")
	_ = os.MkdirAll(codeDirB, 0755)
	_ = RunGit(ctx, codeDirB, "init")
	_ = os.WriteFile(filepath.Join(codeDirB, "main.go"), []byte("package main\n"), 0644)
	_ = RunGit(ctx, codeDirB, "add", "main.go")
	_ = RunGit(ctx, codeDirB, "commit", "-m", "init b")

	if code, _, errOut := runCLI(ctx, codeDirB, "init", "--name", "Project B"); code != 0 {
		t.Fatalf("init project B failed: %s", errOut)
	}
	_ = os.WriteFile(filepath.Join(codeDirB, "devdocs", "note-b.md"), []byte("note B"), 0644)
	if code, _, errOut := runCLI(ctx, codeDirB, "sync"); code != 0 {
		t.Fatalf("sync project B failed: %s", errOut)
	}

	// 3. List projects on remote: must show both branches
	code, listOut, listErr := runCLI(ctx, env.codeDir, "list")
	if code != 0 {
		t.Fatalf("list failed: %s", listErr)
	}
	if !strings.Contains(listOut, metaA.Branch) {
		t.Errorf("list output missing branch A %q: %s", metaA.Branch, listOut)
	}

	// 4. In a third repository (Replica 2 of A), attach only project A
	codeDirA2 := filepath.Join(env.rootDir, "code-a-replica2")
	_ = os.MkdirAll(codeDirA2, 0755)
	_ = RunGit(ctx, codeDirA2, "init")
	_ = os.WriteFile(filepath.Join(codeDirA2, "main.go"), []byte("package main\n"), 0644)
	_ = RunGit(ctx, codeDirA2, "add", "main.go")
	_ = RunGit(ctx, codeDirA2, "commit", "-m", "init a2")

	code, attachOut, attachErr := runCLI(ctx, codeDirA2, "attach", metaA.Branch)
	if code != 0 {
		t.Fatalf("attach failed: %s", attachErr)
	}
	if !strings.Contains(attachOut, "Attached") {
		t.Errorf("attach output mismatch: %s", attachOut)
	}

	// Verify project A notes exist
	docsDirA2 := filepath.Join(codeDirA2, "devdocs")
	noteAContent, err := os.ReadFile(filepath.Join(docsDirA2, "note-a.md"))
	if err != nil || string(noteAContent) != "note A" {
		t.Errorf("note-a.md content mismatch in replica: %s, %v", noteAContent, err)
	}

	// Verify project B does NOT exist in replica of A
	if _, err := os.Stat(filepath.Join(docsDirA2, "note-b.md")); !os.IsNotExist(err) {
		t.Errorf("project B files should NOT be present in project A docs")
	}

	// Verify local state has published: true
	stateA2, err := ReadLocalState(docsDirA2)
	if err != nil {
		t.Fatalf("reading state A2: %v", err)
	}
	if !stateA2.Published {
		t.Errorf("expected state.Published to be true after attach")
	}

	// Repeat attach is idempotent
	code, _, repeatErr := runCLI(ctx, codeDirA2, "attach", metaA.Branch)
	if code != 0 {
		t.Fatalf("repeat attach failed: %s", repeatErr)
	}
}

func TestWorkspaceFlockConcurrency(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// Acquire workspace lock manually
	commonGitDir := filepath.Join(env.codeDir, ".git")
	lock, err := AcquireWorkspaceLock(env.codeDir, commonGitDir)
	if err != nil {
		t.Fatalf("acquiring workspace lock: %v", err)
	}

	// Attempt to run init while lock is held
	code, _, _ := runCLI(ctx, env.codeDir, "init", "--name", "Lock Test")
	if code == 0 {
		t.Fatalf("expected command to fail while workspace lock is held, but succeeded")
	}

	// Release lock
	if err := lock.Unlock(); err != nil {
		t.Fatalf("releasing lock: %v", err)
	}

	// Now command must succeed
	code, _, errOut := runCLI(ctx, env.codeDir, "init", "--name", "Lock Test")
	if code != 0 {
		t.Fatalf("command failed after lock was released: %s", errOut)
	}
}
