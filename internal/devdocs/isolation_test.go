package devdocs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestExclusionsPreserveExistingAndPriorPaths(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	common := filepath.Join(env.codeDir, ".git")
	path := filepath.Join(common, "info", "exclude")
	if err := os.WriteFile(path, []byte("*.log\ntemp/\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateExclusions(common, []string{"AGENTS.md", "private [docs]/notes.md"}); err != nil {
		t.Fatal(err)
	}
	if err := UpdateExclusions(common, []string{"CLAUDE.md"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"build.log", "devdocs", "AGENTS.md", "CLAUDE.md", "private [docs]/notes.md"} {
		if err := CheckEffectiveIgnore(ctx, env.codeDir, path); err != nil {
			t.Errorf("existing or newly managed path %q is no longer excluded: %v", path, err)
		}
	}
}

func TestExclusionsRefuseSymlinkedFile(t *testing.T) {
	env := setupTestEnv(t)
	common := filepath.Join(env.codeDir, ".git")
	path := filepath.Join(common, "info", "exclude")
	external := filepath.Join(t.TempDir(), "outside-ignore")
	const original = "outside-content\n"
	if err := os.WriteFile(external, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, path); err != nil {
		t.Fatal(err)
	}
	if err := UpdateExclusions(common, []string{"AGENTS.md"}); err == nil {
		t.Fatal("followed a symlinked exclusion file")
	}
	if content, err := os.ReadFile(external); err != nil || string(content) != original {
		t.Fatalf("changed external exclusion data: %q, %v", content, err)
	}
	if target, err := os.Readlink(path); err != nil || target != external {
		t.Fatalf("replaced the existing symlink: %q, %v", target, err)
	}
}

func TestCheckEffectiveIgnorePrecedence(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	commonGitDir := filepath.Join(env.codeDir, ".git")

	// Update exclusions to include /devdocs
	if err := UpdateExclusions(commonGitDir, nil); err != nil {
		t.Fatalf("UpdateExclusions error: %v", err)
	}

	// Verify devdocs is ignored
	if err := CheckEffectiveIgnore(ctx, env.codeDir, "devdocs"); err != nil {
		t.Errorf("expected devdocs to be ignored, got: %v", err)
	}

	// Now add an override in shared .gitignore (!devdocs)
	gitignorePath := filepath.Join(env.codeDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte("!devdocs\n"), 0644); err != nil {
		t.Fatalf("writing .gitignore: %v", err)
	}

	// CheckEffectiveIgnore must detect the override and return an error
	if err := CheckEffectiveIgnore(ctx, env.codeDir, "devdocs"); err == nil {
		t.Fatal("expected exclusion override to be refused")
	}
}

func TestCheckTrackedInCodeRepo(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// Initial file main.go is tracked
	if err := CheckTrackedInCodeRepo(ctx, env.codeDir, "main.go"); err == nil {
		t.Errorf("expected CheckTrackedInCodeRepo to detect tracked main.go, got nil")
	}

	// devdocs is not tracked
	if err := CheckTrackedInCodeRepo(ctx, env.codeDir, "devdocs"); err != nil {
		t.Errorf("expected devdocs to be untracked, got: %v", err)
	}

	// Create and track a file under devdocs/
	docsDir := filepath.Join(env.codeDir, "devdocs")
	_ = os.MkdirAll(docsDir, 0755)
	trackedFile := filepath.Join(docsDir, "tracked.txt")
	_ = os.WriteFile(trackedFile, []byte("tracked content"), 0644)
	if err := RunGit(ctx, env.codeDir, "add", "-f", "devdocs/tracked.txt"); err != nil {
		t.Fatalf("force-adding tracked.txt: %v", err)
	}

	// CheckTrackedInCodeRepo on devdocs must now fail!
	if err := CheckTrackedInCodeRepo(ctx, env.codeDir, "devdocs"); err == nil {
		t.Errorf("expected CheckTrackedInCodeRepo to detect tracked file under devdocs, got nil")
	}
}

func TestWorktreeDiscovery(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	worktreeDir := filepath.Join(env.rootDir, "code-worktree")
	if err := RunGit(ctx, env.codeDir, "worktree", "add", "-b", "feature", worktreeDir); err != nil {
		t.Fatalf("creating git worktree: %v", err)
	}

	// Verify that <worktreeDir>/.git is a file, not a directory
	gitFile := filepath.Join(worktreeDir, ".git")
	fi, err := os.Lstat(gitFile)
	if err != nil {
		t.Fatalf("stating worktree .git: %v", err)
	}
	if fi.IsDir() {
		t.Fatalf("expected worktree .git to be a file, but it is a directory")
	}

	ws, err := DiscoverWorkspace(ctx, worktreeDir)
	if err != nil {
		t.Fatalf("DiscoverWorkspace in worktree failed: %v", err)
	}

	// CodeRoot must be worktreeDir
	evalWorktree, _ := filepath.EvalSymlinks(worktreeDir)
	if ws.CodeRoot != evalWorktree {
		t.Errorf("CodeRoot mismatch: got %s, expected %s", ws.CodeRoot, evalWorktree)
	}

	// CommonGitDir must point back to primary repo .git
	evalCommon, _ := filepath.EvalSymlinks(filepath.Join(env.codeDir, ".git"))
	if ws.CommonGitDir != evalCommon {
		t.Errorf("CommonGitDir mismatch: got %s, expected %s", ws.CommonGitDir, evalCommon)
	}
}

func TestInvocationFromInsideDocs(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// Initialize devdocs
	code, out, errOut := runCLI(ctx, env.codeDir, "init", "--name", "inside-test")
	if code != 0 {
		t.Fatalf("init failed (%d): %s %s", code, out, errOut)
	}

	docsDir := filepath.Join(env.codeDir, "devdocs")
	ws, err := DiscoverWorkspace(ctx, docsDir)
	if err != nil {
		t.Fatalf("DiscoverWorkspace from inside docs failed: %v", err)
	}

	if !ws.InsideDocsRepo {
		t.Errorf("expected InsideDocsRepo to be true")
	}

	evalCode, _ := filepath.EvalSymlinks(env.codeDir)
	if ws.CodeRoot != evalCode {
		t.Errorf("CodeRoot mismatch when invoked inside docs: got %s, expected %s", ws.CodeRoot, evalCode)
	}
}
