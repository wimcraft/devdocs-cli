package devdocs

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDocsGitNeverFallsBackToSourceIndex(t *testing.T) {
	for _, kind := range []string{"absent", "plain-directory", "empty-git", "symlinked-root", "symlinked-git", "git-file"} {
		t.Run(kind, func(t *testing.T) {
			env := setupTestEnv(t)
			ctx := context.Background()
			docs := filepath.Join(env.codeDir, "devdocs")
			if err := os.WriteFile(filepath.Join(env.codeDir, "untracked-source.txt"), []byte("must not be staged"), 0644); err != nil {
				t.Fatal(err)
			}
			if kind == "symlinked-root" {
				external := filepath.Join(env.rootDir, "external")
				if err := os.Mkdir(external, 0755); err != nil {
					t.Fatal(err)
				}
				if err := RunGit(ctx, external, "init"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(external, docs); err != nil {
					t.Fatal(err)
				}
			} else if kind != "absent" {
				if err := os.Mkdir(docs, 0755); err != nil {
					t.Fatal(err)
				}
				if kind == "empty-git" {
					if err := os.Mkdir(filepath.Join(docs, ".git"), 0755); err != nil {
						t.Fatal(err)
					}
				}
				if kind == "symlinked-git" {
					if err := os.Symlink(filepath.Join(env.codeDir, ".git"), filepath.Join(docs, ".git")); err != nil {
						t.Fatal(err)
					}
				} else if kind == "git-file" {
					if err := os.WriteFile(filepath.Join(docs, ".git"), []byte("gitdir: ../.git\n"), 0644); err != nil {
						t.Fatal(err)
					}
				}
			}
			before, err := GitCommand(ctx, env.codeDir, "ls-files", "--stage", "-z").Output()
			if err != nil {
				t.Fatal(err)
			}
			if code, _, _ := runCLI(ctx, env.codeDir, "git", "rev-parse", "--show-toplevel"); code == 0 {
				t.Fatal("Git pass-through fell back to the source repository")
			}
			if code, _, _ := runCLI(ctx, env.codeDir, "git", "add", "-A"); code == 0 {
				t.Fatal("ran a mutating Git command without a real docs checkout")
			}
			after, err := GitCommand(ctx, env.codeDir, "ls-files", "--stage", "-z").Output()
			if err != nil || !bytes.Equal(after, before) {
				t.Fatalf("Git pass-through modified the source index: %v", err)
			}
		})
	}
}

func TestDocsGitCommitsOnlyPrivateDocuments(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	if code, out, errOut := runCLI(ctx, env.codeDir, "init"); code != 0 {
		t.Fatalf("init failed: %s %s", out, errOut)
	}
	docs := filepath.Join(env.codeDir, "devdocs")
	for _, dir := range []string{filepath.Join(env.codeDir, "src"), filepath.Join(docs, "drafts")} {
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(docs, ".gitignore"), []byte("private.md\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "private.md"), []byte("private decisions\n"), 0644); err != nil {
		t.Fatal(err)
	}
	beforeHead, err := RunGitOutput(ctx, env.codeDir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	beforeIndex, err := GitCommand(ctx, env.codeDir, "ls-files", "--stage", "-z").Output()
	if err != nil {
		t.Fatal(err)
	}
	if code, out, errOut := runCLI(ctx, filepath.Join(env.codeDir, "src"), "git", "add", "-f", "--", "private.md"); code != 0 {
		t.Fatalf("staging private document failed: %s %s", out, errOut)
	}
	if code, out, errOut := runCLI(ctx, filepath.Join(docs, "drafts"), "git", "commit", "-m", "Private decisions", "--", "private.md"); code != 0 {
		t.Fatalf("private commit failed: %s %s", out, errOut)
	}
	blob, err := GitCommand(ctx, docs, "show", "HEAD:private.md").Output()
	if err != nil || string(blob) != "private decisions\n" {
		t.Fatalf("private history did not contain the requested document: %q, %v", blob, err)
	}
	afterHead, err := RunGitOutput(ctx, env.codeDir, "rev-parse", "HEAD")
	if err != nil || afterHead != beforeHead {
		t.Fatalf("private Git commit modified source history: %v", err)
	}
	afterIndex, err := GitCommand(ctx, env.codeDir, "ls-files", "--stage", "-z").Output()
	if err != nil || !bytes.Equal(afterIndex, beforeIndex) {
		t.Fatalf("private Git commit modified source index: %v", err)
	}
	state, err := ReadLocalState(docs)
	if err != nil || state.Published {
		t.Fatalf("native commit unexpectedly published the workspace: %v", err)
	}
}

func TestDocsGitRepairsInvalidOrAbsentMetadata(t *testing.T) {
	for _, missing := range []bool{false, true} {
		name := "invalid"
		if missing {
			name = "both-markers-missing"
		}
		t.Run(name, func(t *testing.T) {
			env := setupTestEnv(t)
			ctx := context.Background()
			if code, out, errOut := runCLI(ctx, env.codeDir, "init"); code != 0 {
				t.Fatalf("init failed: %s %s", out, errOut)
			}
			docs := filepath.Join(env.codeDir, "devdocs")
			metadata := filepath.Join(docs, ".devdocs.json")
			original, err := os.ReadFile(metadata)
			if err != nil {
				t.Fatal(err)
			}
			if missing {
				for _, path := range []string{metadata, filepath.Join(docs, ".git", "devdocs-state.json")} {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				}
			} else if err := os.WriteFile(metadata, []byte("not valid JSON"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(docs, "drafts"), 0755); err != nil {
				t.Fatal(err)
			}
			if code, out, errOut := runCLI(ctx, filepath.Join(docs, "drafts"), "git", "restore", "--", ".devdocs.json"); code != 0 {
				t.Fatalf("native metadata recovery was blocked: %s %s", out, errOut)
			}
			actual, err := os.ReadFile(metadata)
			if err != nil || !bytes.Equal(actual, original) {
				t.Fatalf("native Git failed to restore original metadata: %q, %v", actual, err)
			}
		})
	}
}
