package devdocs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatusOfflineUnpublished(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	if code, out, errOut := runCLI(ctx, env.codeDir, "init", "--name", "Status Offline"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}

	docsDir := filepath.Join(env.codeDir, "devdocs")
	_ = os.WriteFile(filepath.Join(docsDir, "draft.md"), []byte("draft note\n"), 0644)

	code, out, errOut := runCLI(ctx, env.codeDir, "status")
	if code != 0 {
		t.Fatalf("status failed (%d): %s %s", code, out, errOut)
	}

	if !strings.Contains(out, "unpublished (local only") {
		t.Errorf("expected status to report unpublished, got: %s", out)
	}
	if !strings.Contains(out, "draft.md") {
		t.Errorf("expected status to show untracked draft.md, got: %s", out)
	}
}

func TestStatusPreservesTrackedPathnames(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	if code, out, errOut := runCLI(ctx, env.codeDir, "init"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}
	docsDir := filepath.Join(env.codeDir, "devdocs")
	if err := os.WriteFile(filepath.Join(docsDir, "FYI.md"), []byte("edited decision\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if code, out, errOut := runCLI(ctx, env.codeDir, "status"); code != 0 || !strings.Contains(out, "FYI.md") {
		t.Fatalf("status lost the unstaged tracked pathname: %s %s", out, errOut)
	}
	const renamed = "decision ü space.md"
	if err := RunGit(ctx, docsDir, "mv", "FYI.md", renamed); err != nil {
		t.Fatal(err)
	}
	if code, out, errOut := runCLI(ctx, env.codeDir, "status"); code != 0 || !strings.Contains(out, renamed) || strings.Contains(out, "FYI.md") {
		t.Fatalf("status misread the renamed pathname: %s %s", out, errOut)
	}
}

func TestStatusIgnoredFilesVisibility(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	if code, out, errOut := runCLI(ctx, env.codeDir, "init", "--name", "Ignored Vis"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}

	docsDir := filepath.Join(env.codeDir, "devdocs")
	_ = os.WriteFile(filepath.Join(docsDir, ".gitignore"), []byte("*.private\n"), 0644)
	_ = os.WriteFile(filepath.Join(docsDir, "secret.private"), []byte("confidential\n"), 0644)

	code, out, errOut := runCLI(ctx, env.codeDir, "status")
	if code != 0 {
		t.Fatalf("status failed: %s %s", out, errOut)
	}

	if !strings.Contains(out, "secret.private") {
		t.Errorf("expected status to display ignored secret.private, got: %s", out)
	}
}

func TestStatusSafetyFailureTracked(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	if code, out, errOut := runCLI(ctx, env.codeDir, "init", "--name", "Tracked Safety"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}

	docsDir := filepath.Join(env.codeDir, "devdocs")
	_ = os.WriteFile(filepath.Join(docsDir, "tracked.txt"), []byte("tracked content"), 0644)
	if code, out, errOut := runCLI(ctx, env.codeDir, "link", "tracked.txt", "tracked.txt"); code != 0 {
		t.Fatalf("link failed: %s %s", out, errOut)
	}

	// Force track the link destination in the code repository index
	if err := RunGit(ctx, env.codeDir, "add", "-f", "tracked.txt"); err != nil {
		t.Fatalf("force-adding to code index: %v", err)
	}

	code, out, errOut := runCLI(ctx, env.codeDir, "status")
	if code == 0 {
		t.Fatalf("expected status to fail with safety failure, but succeeded: %s", out)
	}

	if !strings.Contains(out+errOut, "tracked.txt") {
		t.Fatalf("status did not identify the unsafe indexed path: %s %s", out, errOut)
	}
}

func TestStatusSafetyFailureIneffectiveExclusion(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	if code, out, errOut := runCLI(ctx, env.codeDir, "init", "--name", "Exclude Safety"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}

	// Override exclusion in code repo .gitignore
	gitignorePath := filepath.Join(env.codeDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte("!devdocs\n"), 0644); err != nil {
		t.Fatalf("writing .gitignore: %v", err)
	}

	code, out, errOut := runCLI(ctx, env.codeDir, "status")
	if code == 0 {
		t.Fatalf("expected status to fail when exclusion is ineffective, but succeeded: %s", out)
	}

	if !strings.Contains(out+errOut, "devdocs") {
		t.Fatalf("status did not identify the unprotected docs path: %s %s", out, errOut)
	}
}

func TestStatusGitOperationInProgress(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	if code, out, errOut := runCLI(ctx, env.codeDir, "init", "--name", "Git Op Safety"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}

	docsDir := filepath.Join(env.codeDir, "devdocs")
	// Simulate merge in progress by creating MERGE_HEAD
	mergeHeadFile := filepath.Join(docsDir, ".git", "MERGE_HEAD")
	_ = os.WriteFile(mergeHeadFile, []byte("0123456789abcdef0123456789abcdef01234567\n"), 0644)

	code, out, _ := runCLI(ctx, env.codeDir, "status")
	if code == 0 {
		t.Fatalf("expected status to fail when git merge is in progress, but succeeded: %s", out)
	}

	if _, err := os.Stat(mergeHeadFile); err != nil {
		t.Fatalf("status changed the in-progress merge: %v", err)
	}
}

func TestStatusLinkHealth(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	if code, out, errOut := runCLI(ctx, env.codeDir, "init", "--name", "Link Health"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}

	docsDir := filepath.Join(env.codeDir, "devdocs")
	_ = os.WriteFile(filepath.Join(docsDir, "healthy.txt"), []byte("healthy"), 0644)
	_ = os.WriteFile(filepath.Join(docsDir, "missing.txt"), []byte("missing"), 0644)
	_ = os.WriteFile(filepath.Join(docsDir, "changed.txt"), []byte("changed"), 0644)
	_ = os.WriteFile(filepath.Join(docsDir, "stale.txt"), []byte("stale"), 0644)

	// Legitmately create links so exclusions and ownership are established
	if code, out, errOut := runCLI(ctx, env.codeDir, "link", "healthy.txt", "healthy.txt"); code != 0 {
		t.Fatalf("link healthy failed: %s %s", out, errOut)
	}
	if code, out, errOut := runCLI(ctx, env.codeDir, "link", "missing.txt", "missing.txt"); code != 0 {
		t.Fatalf("link missing failed: %s %s", out, errOut)
	}
	if code, out, errOut := runCLI(ctx, env.codeDir, "link", "changed.txt", "changed.txt"); code != 0 {
		t.Fatalf("link changed failed: %s %s", out, errOut)
	}
	if code, out, errOut := runCLI(ctx, env.codeDir, "link", "stale.txt", "stale.txt"); code != 0 {
		t.Fatalf("link stale failed: %s %s", out, errOut)
	}

	// 1. missing: destination symlink was deleted from filesystem, declaration remains
	_ = os.Remove(filepath.Join(env.codeDir, "missing.txt"))

	// 2. changed: destination symlink points to wrong target
	changedDest := filepath.Join(env.codeDir, "changed.txt")
	_ = os.Remove(changedDest)
	_ = os.Symlink("devdocs/healthy.txt", changedDest)

	// 3. stale: declaration removed from .devdocs.json remotely, local ownership remains
	meta, _ := ReadMetadata(docsDir)
	var remaining []LinkDeclaration
	for _, l := range meta.Links {
		if l.Destination != "stale.txt" {
			remaining = append(remaining, l)
		}
	}
	meta.Links = remaining
	_ = WriteMetadata(docsDir, meta)

	code, out, errOut := runCLI(ctx, env.codeDir, "status")
	if code != 0 {
		t.Fatalf("status failed unexpectedly (%d): %s %s", code, out, errOut)
	}

	if !strings.Contains(out, "healthy.txt -> healthy.txt [healthy]") {
		t.Errorf("expected healthy link report, got: %s", out)
	}
	if !strings.Contains(out, "missing.txt -> missing.txt [missing; not installed]") {
		t.Errorf("expected missing link report, got: %s", out)
	}
	if !strings.Contains(out, "changed.txt -> changed.txt [changed") {
		t.Errorf("expected changed link report, got: %s", out)
	}
	if !strings.Contains(out, "stale.txt -> devdocs/stale.txt [stale") {
		t.Errorf("expected stale link report, got: %s", out)
	}
}
