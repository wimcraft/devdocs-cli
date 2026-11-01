package devdocs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncTransferAndDeletions(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// Initialize project in replica 1
	if code, out, errOut := runCLI(ctx, env.codeDir, "init", "--name", "Sync Transfer"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}

	docsDir1 := filepath.Join(env.codeDir, "devdocs")
	meta, err := ReadMetadata(docsDir1)
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}

	// Add file1, file2
	_ = os.WriteFile(filepath.Join(docsDir1, "file1.txt"), []byte("file1 content"), 0644)
	_ = os.WriteFile(filepath.Join(docsDir1, "file2.txt"), []byte("file2 to be deleted"), 0644)

	// Sync
	if code, out, errOut := runCLI(ctx, env.codeDir, "sync", "-m", "add files 1 and 2"); code != 0 {
		t.Fatalf("sync failed: %s %s", out, errOut)
	}

	// Now modify file1, delete file2, add file3
	_ = os.WriteFile(filepath.Join(docsDir1, "file1.txt"), []byte("file1 updated"), 0644)
	_ = os.Remove(filepath.Join(docsDir1, "file2.txt"))
	_ = os.WriteFile(filepath.Join(docsDir1, "file3.txt"), []byte("file3 new"), 0644)

	if code, out, errOut := runCLI(ctx, env.codeDir, "sync", "-m", "update and delete"); code != 0 {
		t.Fatalf("sync update failed: %s %s", out, errOut)
	}

	// Attach in replica 2
	replica2Code := filepath.Join(env.rootDir, "replica2")
	_ = os.MkdirAll(replica2Code, 0755)
	_ = RunGit(ctx, replica2Code, "init")
	_ = os.WriteFile(filepath.Join(replica2Code, "main.go"), []byte("package main\n"), 0644)
	_ = RunGit(ctx, replica2Code, "add", "main.go")
	_ = RunGit(ctx, replica2Code, "commit", "-m", "init rep2")

	if code, out, errOut := runCLI(ctx, replica2Code, "attach", meta.Branch); code != 0 {
		t.Fatalf("attach replica 2 failed: %s %s", out, errOut)
	}

	docsDir2 := filepath.Join(replica2Code, "devdocs")

	// Verify file1 has updated content
	content1, err := os.ReadFile(filepath.Join(docsDir2, "file1.txt"))
	if err != nil || string(content1) != "file1 updated" {
		t.Errorf("file1 content mismatch in replica 2: %s, %v", content1, err)
	}

	// Verify file2 is deleted
	if _, err := os.Stat(filepath.Join(docsDir2, "file2.txt")); !os.IsNotExist(err) {
		t.Errorf("file2.txt should have been deleted in replica 2")
	}

	// Verify file3 exists
	content3, err := os.ReadFile(filepath.Join(docsDir2, "file3.txt"))
	if err != nil || string(content3) != "file3 new" {
		t.Errorf("file3 content mismatch in replica 2: %s, %v", content3, err)
	}

	// Sync on clean replica 2: must succeed with no new commit
	before, err := RunGitOutput(ctx, docsDir2, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runCLI(ctx, replica2Code, "sync")
	if code != 0 {
		t.Fatalf("clean sync failed: %s %s", out, errOut)
	}
	after, err := RunGitOutput(ctx, docsDir2, "rev-parse", "HEAD")
	if err != nil || after != before {
		t.Fatalf("clean sync changed local history: %q, %v", after, err)
	}
}

func TestSyncNonConflictingDivergence(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	if code, out, errOut := runCLI(ctx, env.codeDir, "init", "--name", "Diverge Test"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}
	docs1 := filepath.Join(env.codeDir, "devdocs")
	meta, _ := ReadMetadata(docs1)

	// First sync to publish branch
	if code, out, errOut := runCLI(ctx, env.codeDir, "sync"); code != 0 {
		t.Fatalf("initial sync failed: %s %s", out, errOut)
	}

	// Attach in replica 2
	replica2Code := filepath.Join(env.rootDir, "rep2-div")
	_ = os.MkdirAll(replica2Code, 0755)
	_ = RunGit(ctx, replica2Code, "init")
	_ = os.WriteFile(filepath.Join(replica2Code, "main.go"), []byte("package main\n"), 0644)
	_ = RunGit(ctx, replica2Code, "add", "main.go")
	_ = RunGit(ctx, replica2Code, "commit", "-m", "init rep2")
	if code, out, errOut := runCLI(ctx, replica2Code, "attach", meta.Branch); code != 0 {
		t.Fatalf("attach replica 2 failed: %s %s", out, errOut)
	}
	docs2 := filepath.Join(replica2Code, "devdocs")

	// Replica 1 adds file-r1.txt and syncs
	_ = os.WriteFile(filepath.Join(docs1, "file-r1.txt"), []byte("replica 1 edit"), 0644)
	if code, out, errOut := runCLI(ctx, env.codeDir, "sync"); code != 0 {
		t.Fatalf("replica 1 sync failed: %s %s", out, errOut)
	}

	// Replica 2 adds file-r2.txt and syncs
	_ = os.WriteFile(filepath.Join(docs2, "file-r2.txt"), []byte("replica 2 edit"), 0644)
	if code, out, errOut := runCLI(ctx, replica2Code, "sync"); code != 0 {
		t.Fatalf("replica 2 sync (3-way merge) failed: %s %s", out, errOut)
	}

	// Replica 2 must have BOTH files
	if _, err := os.Stat(filepath.Join(docs2, "file-r1.txt")); err != nil {
		t.Errorf("file-r1.txt missing in replica 2 after merge")
	}
	if _, err := os.Stat(filepath.Join(docs2, "file-r2.txt")); err != nil {
		t.Errorf("file-r2.txt missing in replica 2 after merge")
	}

	// Replica 1 syncs to get replica 2's changes
	if code, out, errOut := runCLI(ctx, env.codeDir, "sync"); code != 0 {
		t.Fatalf("replica 1 fast-forward sync failed: %s %s", out, errOut)
	}
	if _, err := os.Stat(filepath.Join(docs1, "file-r2.txt")); err != nil {
		t.Errorf("file-r2.txt missing in replica 1 after fast-forward")
	}
}

func TestSyncConflictRecovery(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	if code, out, errOut := runCLI(ctx, env.codeDir, "init", "--name", "Conflict Test"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}
	docs1 := filepath.Join(env.codeDir, "devdocs")
	meta, _ := ReadMetadata(docs1)

	// Create shared file and sync
	sharedFile1 := filepath.Join(docs1, "shared.txt")
	_ = os.WriteFile(sharedFile1, []byte("Base content\n"), 0644)
	if code, out, errOut := runCLI(ctx, env.codeDir, "sync"); code != 0 {
		t.Fatalf("base sync failed: %s %s", out, errOut)
	}

	// Attach in replica 2
	replica2Code := filepath.Join(env.rootDir, "rep2-conflict")
	_ = os.MkdirAll(replica2Code, 0755)
	_ = RunGit(ctx, replica2Code, "init")
	_ = os.WriteFile(filepath.Join(replica2Code, "main.go"), []byte("package main\n"), 0644)
	_ = RunGit(ctx, replica2Code, "add", "main.go")
	_ = RunGit(ctx, replica2Code, "commit", "-m", "init rep2")
	if code, out, errOut := runCLI(ctx, replica2Code, "attach", meta.Branch); code != 0 {
		t.Fatalf("attach replica 2 failed: %s %s", out, errOut)
	}
	docs2 := filepath.Join(replica2Code, "devdocs")

	// Replica 1 edits shared.txt and syncs
	_ = os.WriteFile(sharedFile1, []byte("Replica 1 conflicting line\n"), 0644)
	if code, out, errOut := runCLI(ctx, env.codeDir, "sync"); code != 0 {
		t.Fatalf("replica 1 sync failed: %s %s", out, errOut)
	}

	// Replica 2 edits shared.txt with conflicting change and syncs
	sharedFile2 := filepath.Join(docs2, "shared.txt")
	_ = os.WriteFile(sharedFile2, []byte("Replica 2 conflicting line\n"), 0644)

	code, out, errOut := runCLI(ctx, replica2Code, "sync")
	if code == 0 {
		t.Fatalf("expected sync to fail with conflict, but succeeded: %s", out)
	}

	if !strings.Contains(errOut, "conflict") || !strings.Contains(errOut, "shared.txt") {
		t.Errorf("expected conflict guidance naming shared.txt, got: %s", errOut)
	}
	if !strings.Contains(errOut, "merge --abort") {
		t.Errorf("expected merge --abort guidance, got: %s", errOut)
	}

	// Verify merge is in progress
	if _, err := os.Stat(filepath.Join(docs2, ".git", "MERGE_HEAD")); err != nil {
		t.Fatalf("MERGE_HEAD expected to exist in conflicting workspace: %v", err)
	}

	// Aborting merge must succeed and preserve local snapshot commit
	if err := RunGit(ctx, docs2, "merge", "--abort"); err != nil {
		t.Fatalf("git merge --abort failed: %v", err)
	}

	// Local snapshot commit should still have Replica 2's edit
	abortedContent, err := os.ReadFile(sharedFile2)
	if err != nil || string(abortedContent) != "Replica 2 conflicting line\n" {
		t.Errorf("local snapshot edit lost after merge --abort: %s, %v", abortedContent, err)
	}
}

func TestSyncVanishedRemoteBranchRefusal(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	if code, out, errOut := runCLI(ctx, env.codeDir, "init", "--name", "Vanish Test"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}
	docs1 := filepath.Join(env.codeDir, "devdocs")
	meta, _ := ReadMetadata(docs1)

	// Sync to publish
	if code, out, errOut := runCLI(ctx, env.codeDir, "sync"); code != 0 {
		t.Fatalf("sync failed: %s %s", out, errOut)
	}

	// Verify state is published
	state, _ := ReadLocalState(docs1)
	if !state.Published {
		t.Fatalf("expected state.Published to be true")
	}

	// Delete branch on bare remote directly
	if err := RunGit(ctx, env.remoteDir, "branch", "-D", meta.Branch); err != nil {
		t.Fatalf("deleting branch on remote: %v", err)
	}

	// Make an edit and sync
	_ = os.WriteFile(filepath.Join(docs1, "new.txt"), []byte("new"), 0644)
	code, out, _ := runCLI(ctx, env.codeDir, "sync")
	if code == 0 {
		t.Fatalf("expected sync to refuse when published branch vanished, but succeeded: %s", out)
	}

	if _, err := RunGitOutput(ctx, env.remoteDir, "show-ref", "--verify", "refs/heads/"+meta.Branch); err == nil {
		t.Fatal("sync recreated the deleted project branch")
	}
}

func TestSyncRefusesWhenInstalledLinkTracked(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	if code, out, errOut := runCLI(ctx, env.codeDir, "init", "--name", "Tracked Link Sync"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}

	docsDir := filepath.Join(env.codeDir, "devdocs")
	_ = os.WriteFile(filepath.Join(docsDir, "AGENTS.md"), []byte("agent instructions\n"), 0644)

	// Link AGENTS.md
	if code, out, errOut := runCLI(ctx, env.codeDir, "link", "AGENTS.md", "AGENTS.md"); code != 0 {
		t.Fatalf("link failed: %s %s", out, errOut)
	}

	// Initial sync succeeds
	if code, out, errOut := runCLI(ctx, env.codeDir, "sync"); code != 0 {
		t.Fatalf("initial sync failed: %s %s", out, errOut)
	}

	// Now force-add AGENTS.md to code repository index (violating A12)
	if err := RunGit(ctx, env.codeDir, "add", "-f", "AGENTS.md"); err != nil {
		t.Fatalf("force-adding AGENTS.md: %v", err)
	}

	// devdocs sync MUST refuse!
	code, out, _ := runCLI(ctx, env.codeDir, "sync")
	if code == 0 {
		t.Fatalf("expected sync to refuse when installed link is tracked, but succeeded: %s", out)
	}
	if files, err := RunGitOutput(ctx, env.codeDir, "ls-files", "--", "AGENTS.md"); err != nil || files != "AGENTS.md" {
		t.Fatalf("sync changed the source index: %q, %v", files, err)
	}
}

func TestSyncRefusesWhenBranchOrRemoteTampered(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	if code, out, errOut := runCLI(ctx, env.codeDir, "init", "--name", "Pin Tamper"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}
	docsDir := filepath.Join(env.codeDir, "devdocs")

	// Switch branch in docs
	if err := RunGit(ctx, docsDir, "checkout", "-b", "tampered-branch"); err != nil {
		t.Fatalf("checkout tampered branch: %v", err)
	}

	// Sync must refuse
	code, out, _ := runCLI(ctx, env.codeDir, "sync")
	if code == 0 {
		t.Fatalf("expected sync to refuse when branch is tampered, but succeeded: %s", out)
	}
}

// A nested repository would be recorded as an empty gitlink, so sync would
// report success while uploading none of its files.
func TestSyncRefusesNestedRepository(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	if code, out, errOut := runCLI(ctx, env.codeDir, "init"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}
	docsDir := filepath.Join(env.codeDir, "devdocs")
	nested := filepath.Join(docsDir, "vendored")
	_ = os.MkdirAll(nested, 0755)
	_ = RunGit(ctx, nested, "init")
	_ = os.WriteFile(filepath.Join(nested, "note.md"), []byte("note"), 0644)
	_ = RunGit(ctx, nested, "add", ".")
	_ = RunGit(ctx, nested, "commit", "-m", "nested")

	code, _, errOut := runCLI(ctx, env.codeDir, "sync")
	if code == 0 || !strings.Contains(errOut, "vendored") {
		t.Fatalf("sync should refuse the nested repository, got code %d: %s", code, errOut)
	}
	if staged, _ := RunGitOutput(ctx, docsDir, "ls-files", "--stage", "vendored"); staged != "" {
		t.Errorf("nested repository was staged: %s", staged)
	}

	// An ignored nested repository is deliberately omitted, like any ignored path.
	_ = os.WriteFile(filepath.Join(docsDir, ".gitignore"), []byte("vendored/\n"), 0644)
	if code, out, errOut := runCLI(ctx, env.codeDir, "sync"); code != 0 {
		t.Fatalf("sync with ignored nested repository failed: %s %s", out, errOut)
	}
}

// Two replicas adding links is the ordinary way to conflict on metadata, whose
// markers make it unparseable: diagnostics must name the merge, not a JSON error.
func TestMetadataConflictReportsMerge(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	if code, out, errOut := runCLI(ctx, env.codeDir, "init"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}
	if code, out, errOut := runCLI(ctx, env.codeDir, "sync"); code != 0 {
		t.Fatalf("sync failed: %s %s", out, errOut)
	}
	docsDir := filepath.Join(env.codeDir, "devdocs")
	meta, err := ReadMetadata(docsDir)
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}

	other := filepath.Join(env.rootDir, "other")
	_ = os.MkdirAll(other, 0755)
	_ = RunGit(ctx, other, "init")
	if code, out, errOut := runCLI(ctx, other, "attach", meta.Branch); code != 0 {
		t.Fatalf("attach failed: %s %s", out, errOut)
	}

	_ = os.WriteFile(filepath.Join(docsDir, "one.md"), []byte("one"), 0644)
	if code, out, errOut := runCLI(ctx, env.codeDir, "link", "one.md", "ONE.md"); code != 0 {
		t.Fatalf("link failed: %s %s", out, errOut)
	}
	if code, out, errOut := runCLI(ctx, env.codeDir, "sync"); code != 0 {
		t.Fatalf("sync failed: %s %s", out, errOut)
	}
	_ = os.WriteFile(filepath.Join(other, "devdocs", "two.md"), []byte("two"), 0644)
	if code, out, errOut := runCLI(ctx, other, "link", "two.md", "TWO.md"); code != 0 {
		t.Fatalf("link failed: %s %s", out, errOut)
	}
	if code, _, errOut := runCLI(ctx, other, "sync"); code == 0 || !strings.Contains(errOut, ".devdocs.json") {
		t.Fatalf("expected a metadata conflict, got code %d: %s", code, errOut)
	}

	for _, command := range []string{"status", "sync"} {
		code, _, errOut := runCLI(ctx, other, command)
		if code == 0 || !strings.Contains(errOut, "merge is in progress") {
			t.Errorf("%s should report the unfinished merge, got code %d: %s", command, code, errOut)
		}
	}
}
