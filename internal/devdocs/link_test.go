package devdocs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinkBasicAndIdempotent(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// Initialize devdocs
	if code, out, errOut := runCLI(ctx, env.codeDir, "init", "--name", "link-test"); code != 0 {
		t.Fatalf("init failed (%d): %s %s", code, out, errOut)
	}

	docsDir := filepath.Join(env.codeDir, "devdocs")
	agentsFile := filepath.Join(docsDir, "AGENTS.md")
	if err := os.WriteFile(agentsFile, []byte("agent instructions\n"), 0644); err != nil {
		t.Fatalf("writing AGENTS.md: %v", err)
	}

	// Create link
	code, out, errOut := runCLI(ctx, env.codeDir, "link", "AGENTS.md", "AGENTS.md")
	if code != 0 {
		t.Fatalf("link failed (%d): %s %s", code, out, errOut)
	}

	destLink := filepath.Join(env.codeDir, "AGENTS.md")
	fi, err := os.Lstat(destLink)
	if err != nil {
		t.Fatalf("destination link not found: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected destination to be a symlink")
	}

	target, err := os.Readlink(destLink)
	if err != nil {
		t.Fatalf("reading symlink: %v", err)
	}
	expectedTarget := "devdocs/AGENTS.md"
	if target != expectedTarget {
		t.Errorf("symlink target mismatch: got %q, expected %q", target, expectedTarget)
	}

	// Verify metadata
	meta, err := ReadMetadata(docsDir)
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}
	if len(meta.Links) != 1 || meta.Links[0].Destination != "AGENTS.md" {
		t.Fatalf("metadata links mismatch: %+v", meta.Links)
	}

	// Verify local state
	state, err := ReadLocalState(docsDir)
	if err != nil {
		t.Fatalf("reading local state: %v", err)
	}
	if len(state.InstalledLinks) != 1 || state.InstalledLinks[0].Destination != "AGENTS.md" {
		t.Fatalf("local state installed links mismatch: %+v", state.InstalledLinks)
	}

	// Repeating link must be idempotent
	code, _, errOut = runCLI(ctx, env.codeDir, "link", "AGENTS.md", "AGENTS.md")
	if code != 0 {
		t.Fatalf("idempotent link failed (%d): %s", code, errOut)
	}
}

func TestLinkBoundaryAndTraversalRejection(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	if code, out, errOut := runCLI(ctx, env.codeDir, "init"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}

	docsDir := filepath.Join(env.codeDir, "devdocs")
	testFile := filepath.Join(docsDir, "test.txt")
	_ = os.WriteFile(testFile, []byte("test"), 0644)

	invalidPairs := []struct {
		src  string
		dest string
	}{
		{"../outside.txt", "test.txt"},
		{"test.txt", "../outside.txt"},
		{"/abs/src", "test.txt"},
		{"test.txt", "/abs/dest"},
		{".git/config", "leak"},
		{"test.txt", ".git/config"},
		{"test.txt", "devdocs/nested"},
		{".devdocs.json", "meta"},
	}

	for _, tc := range invalidPairs {
		code, _, _ := runCLI(ctx, env.codeDir, "link", tc.src, tc.dest)
		if code == 0 {
			t.Errorf("expected link %q -> %q to fail, but succeeded", tc.src, tc.dest)
		}
	}
}

func TestLinkCollisionRefusal(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	if code, out, errOut := runCLI(ctx, env.codeDir, "init"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}

	docsDir := filepath.Join(env.codeDir, "devdocs")
	src := filepath.Join(docsDir, "notes.md")
	_ = os.WriteFile(src, []byte("notes"), 0644)

	// Collision with tracked file (main.go)
	code, _, _ := runCLI(ctx, env.codeDir, "link", "notes.md", "main.go")
	if code == 0 {
		t.Errorf("expected collision with tracked file to fail, but succeeded")
	}
	mainContent, err := os.ReadFile(filepath.Join(env.codeDir, "main.go"))
	if err != nil || !strings.Contains(string(mainContent), "package main") {
		t.Errorf("main.go data was changed or corrupted: %v", err)
	}

	// Collision with unmanaged regular file
	unmanaged := filepath.Join(env.codeDir, "existing.txt")
	_ = os.WriteFile(unmanaged, []byte("existing"), 0644)
	code, _, _ = runCLI(ctx, env.codeDir, "link", "notes.md", "existing.txt")
	if code == 0 {
		t.Fatal("expected collision with existing unmanaged file to fail")
	}
	if content, err := os.ReadFile(unmanaged); err != nil || string(content) != "existing" {
		t.Fatalf("collision changed existing content: %q, %v", content, err)
	}
}

func TestSourceSymlinkEscapeRefusal(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	if code, out, errOut := runCLI(ctx, env.codeDir, "init"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}

	docsDir := filepath.Join(env.codeDir, "devdocs")
	outsideSentinel := filepath.Join(env.rootDir, "outside-sentinel.txt")
	_ = os.WriteFile(outsideSentinel, []byte("outside content"), 0644)

	// Create symlink inside docs pointing outside
	externalSymlink := filepath.Join(docsDir, "external")
	if err := os.Symlink(outsideSentinel, externalSymlink); err != nil {
		t.Fatalf("creating external symlink: %v", err)
	}

	// Linking external symlink must be refused
	code, _, _ := runCLI(ctx, env.codeDir, "link", "external", "external-link")
	if code == 0 {
		t.Errorf("expected link of external symlink to fail, but succeeded")
	}

	// Ensure destination link was NOT created
	if _, err := os.Lstat(filepath.Join(env.codeDir, "external-link")); !os.IsNotExist(err) {
		t.Errorf("external-link was created despite pointing outside docs")
	}

	// Raw traversal before Clean check
	_ = os.WriteFile(filepath.Join(docsDir, "notes.md"), []byte("notes"), 0644)
	code, _, _ = runCLI(ctx, env.codeDir, "link", "unused/../notes.md", "raw-link")
	if code == 0 {
		t.Errorf("expected raw traversal 'unused/../notes.md' to fail, but succeeded")
	}
}

func TestLinkApplyPreflight(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	if code, out, errOut := runCLI(ctx, env.codeDir, "init"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}

	docsDir := filepath.Join(env.codeDir, "devdocs")
	_ = os.WriteFile(filepath.Join(docsDir, "good.txt"), []byte("good"), 0644)
	_ = os.WriteFile(filepath.Join(docsDir, "colliding.txt"), []byte("colliding"), 0644)

	// Create an unmanaged file in code repo that will collide with link 2
	_ = os.WriteFile(filepath.Join(env.codeDir, "collide-dest.txt"), []byte("unmanaged"), 0644)

	// Update metadata with two declarations: good and colliding
	meta, err := ReadMetadata(docsDir)
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}
	meta.Links = []LinkDeclaration{
		{Source: "good.txt", Destination: "good-dest.txt"},
		{Source: "colliding.txt", Destination: "collide-dest.txt"},
	}
	if err := WriteMetadata(docsDir, meta); err != nil {
		t.Fatalf("writing metadata: %v", err)
	}

	// Run devdocs link --apply: must fail preflight without creating good-dest.txt!
	code, _, _ := runCLI(ctx, env.codeDir, "link", "--apply")
	if code == 0 {
		t.Fatalf("expected link --apply to fail preflight, but succeeded")
	}

	// Ensure good-dest.txt was NOT created
	if _, err := os.Lstat(filepath.Join(env.codeDir, "good-dest.txt")); !os.IsNotExist(err) {
		t.Errorf("expected good-dest.txt to NOT be created on preflight failure")
	}
}

func TestUnlink(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	if code, out, errOut := runCLI(ctx, env.codeDir, "init"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}

	docsDir := filepath.Join(env.codeDir, "devdocs")
	src := filepath.Join(docsDir, "doc.md")
	_ = os.WriteFile(src, []byte("content"), 0644)

	// Link it
	if code, out, errOut := runCLI(ctx, env.codeDir, "link", "doc.md", "doc.md"); code != 0 {
		t.Fatalf("link failed: %s %s", out, errOut)
	}

	// Unlink it
	code, out, errOut := runCLI(ctx, env.codeDir, "unlink", "doc.md")
	if code != 0 {
		t.Fatalf("unlink failed (%d): %s %s", code, out, errOut)
	}

	// Destination symlink must be removed
	if _, err := os.Lstat(filepath.Join(env.codeDir, "doc.md")); !os.IsNotExist(err) {
		t.Errorf("destination link was not removed")
	}

	// Source file MUST NOT be deleted!
	if _, err := os.Stat(src); err != nil {
		t.Errorf("source document was deleted during unlink: %v", err)
	}

	// Notice about retained exclusion must be printed
	if !strings.Contains(out, "exclusion") {
		t.Errorf("expected unlink output to report retained exclusion, got: %s", out)
	}

	// Declaration must be removed from .devdocs.json
	meta, err := ReadMetadata(docsDir)
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}
	for _, l := range meta.Links {
		if l.Destination == "doc.md" {
			t.Errorf("declaration was not removed from .devdocs.json")
		}
	}
}

func TestUnlinkRefusesModifiedDestination(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	if code, out, errOut := runCLI(ctx, env.codeDir, "init"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}

	docsDir := filepath.Join(env.codeDir, "devdocs")
	src := filepath.Join(docsDir, "original.md")
	_ = os.WriteFile(src, []byte("original"), 0644)

	if code, out, errOut := runCLI(ctx, env.codeDir, "link", "original.md", "doc.md"); code != 0 {
		t.Fatalf("link failed: %s %s", out, errOut)
	}

	// Replace symlink with regular file
	destPath := filepath.Join(env.codeDir, "doc.md")
	_ = os.Remove(destPath)
	_ = os.WriteFile(destPath, []byte("user modified regular file"), 0644)

	// Unlink must refuse!
	code, _, _ := runCLI(ctx, env.codeDir, "unlink", "doc.md")
	if code == 0 {
		t.Fatalf("expected unlink to refuse modified regular file, but succeeded")
	}
	if content, err := os.ReadFile(destPath); err != nil || string(content) != "user modified regular file" {
		t.Fatalf("unlink changed the replacement file: %q, %v", content, err)
	}
}
func TestUnlinkStaleOwnership(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	if code, out, errOut := runCLI(ctx, env.codeDir, "init"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}

	docsDir := filepath.Join(env.codeDir, "devdocs")
	src := filepath.Join(docsDir, "stale-src.md")
	_ = os.WriteFile(src, []byte("content"), 0644)

	if code, out, errOut := runCLI(ctx, env.codeDir, "link", "stale-src.md", "stale.md"); code != 0 {
		t.Fatalf("link failed: %s %s", out, errOut)
	}

	// Simulate remote removing declaration from .devdocs.json while local state retains ownership
	meta, err := ReadMetadata(docsDir)
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}
	meta.Links = []LinkDeclaration{} // Remote wiped declarations
	if err := WriteMetadata(docsDir, meta); err != nil {
		t.Fatalf("writing metadata: %v", err)
	}

	// Unlink stale link
	code, out, errOut := runCLI(ctx, env.codeDir, "unlink", "stale.md")
	if code != 0 {
		t.Fatalf("unlink stale link failed (%d): %s %s", code, out, errOut)
	}

	// Destination link must be removed
	if _, err := os.Lstat(filepath.Join(env.codeDir, "stale.md")); !os.IsNotExist(err) {
		t.Errorf("expected stale symlink to be removed")
	}

	// State ownership must be removed
	state, err := ReadLocalState(docsDir)
	if err != nil {
		t.Fatalf("reading state: %v", err)
	}
	for _, inst := range state.InstalledLinks {
		if inst.Destination == "stale.md" {
			t.Errorf("stale link ownership was not removed from local state")
		}
	}
}
