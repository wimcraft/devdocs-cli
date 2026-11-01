package devdocs

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitAdoptPreservesExistingDocuments(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	docs := filepath.Join(env.codeDir, "devdocs")
	originals := map[string][]byte{
		"fyi.md":           []byte("existing decisions\r\n"),
		"readme.md":        []byte("my custom readme\n"),
		"drafts/old ü.txt": {0, 1, 2, 255, '\n'},
		"scripts/run.sh":   []byte("#!/bin/sh\nprintf existing\\n\n"),
		".gitignore":       []byte("ignored.log\n"),
		"ignored.log":      []byte("local-only log\n"),
	}
	info := make(map[string]os.FileInfo)
	for name, contents := range originals {
		path := filepath.Join(docs, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0644)
		if name == "scripts/run.sh" {
			mode = 0755
		}
		if err := os.WriteFile(path, contents, mode); err != nil {
			t.Fatal(err)
		}
		var err error
		info[name], err = os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(env.rootDir, "outside.txt")
	if err := os.WriteFile(outside, []byte("do not dereference this content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(docs, "reference")); err != nil {
		t.Fatal(err)
	}
	beforeIndex, err := GitCommand(ctx, env.codeDir, "ls-files", "--stage", "-z").Output()
	if err != nil {
		t.Fatal(err)
	}
	beforeHead, err := RunGitOutput(ctx, env.codeDir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	// Prove adoption does not need access to the configured remote.
	offlineRemote := env.remoteDir + "-offline"
	if err := os.Rename(env.remoteDir, offlineRemote); err != nil {
		t.Fatal(err)
	}
	defer os.Rename(offlineRemote, env.remoteDir)
	if code, out, errOut := runCLI(ctx, filepath.Join(docs, "drafts"), "init", "--adopt", "--name", "Existing notes"); code != 0 {
		t.Fatalf("adoption failed: %s %s", out, errOut)
	} else if !strings.Contains(out, "ignored.log") {
		t.Fatalf("adoption hid an ignored document: %s", out)
	}
	for name, want := range originals {
		path := filepath.Join(docs, name)
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("adoption changed %q: %q, %v", name, got, err)
		}
		after, err := os.Stat(path)
		if err != nil || !os.SameFile(info[name], after) || after.Mode() != info[name].Mode() || !after.ModTime().Equal(info[name].ModTime()) {
			t.Fatalf("adoption replaced or changed original file metadata for %q: %v", name, err)
		}
		if name != "ignored.log" {
			blob, err := GitCommand(ctx, docs, "show", "HEAD:"+name).Output()
			if err != nil || !bytes.Equal(blob, want) {
				t.Fatalf("initial snapshot did not retain %q: %q, %v", name, blob, err)
			}
		}
	}
	entries, err := os.ReadDir(docs)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() == "FYI.md" || entry.Name() == "README.md" {
			t.Fatalf("created an unwanted template alias %q", entry.Name())
		}
	}
	if target, err := os.Readlink(filepath.Join(docs, "reference")); err != nil || target != outside {
		t.Fatalf("adoption changed a document symlink: %q, %v", target, err)
	}
	blob, err := GitCommand(ctx, docs, "show", "HEAD:reference").Output()
	if err != nil || string(blob) != outside {
		t.Fatalf("snapshot followed a symlink instead of recording its target: %q, %v", blob, err)
	}
	if _, err := GitCommand(ctx, docs, "show", "HEAD:ignored.log").Output(); err == nil {
		t.Fatal("force-added ignored existing content")
	}
	afterIndex, err := GitCommand(ctx, env.codeDir, "ls-files", "--stage", "-z").Output()
	if err != nil || !bytes.Equal(beforeIndex, afterIndex) {
		t.Fatalf("adoption changed the source index: %v", err)
	}
	afterHead, err := RunGitOutput(ctx, env.codeDir, "rev-parse", "HEAD")
	if err != nil || afterHead != beforeHead {
		t.Fatalf("adoption changed source history: %v", err)
	}
	state, err := ReadLocalState(docs)
	if err != nil || state.Published {
		t.Fatalf("adoption was not a valid unpublished workspace: %v", err)
	}
	docsHead, err := RunGitOutput(ctx, docs, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if code, out, errOut := runCLI(ctx, docs, "init", "--adopt"); code != 0 {
		t.Fatalf("repeat adoption failed: %s %s", out, errOut)
	}
	repeatedHead, err := RunGitOutput(ctx, docs, "rev-parse", "HEAD")
	if err != nil || repeatedHead != docsHead {
		t.Fatalf("repeat adoption changed docs history: %v", err)
	}
	repeatedState, err := ReadLocalState(docs)
	if err != nil || repeatedState.ProjectID != state.ProjectID {
		t.Fatalf("repeat adoption changed project identity: %v", err)
	}
}

func TestInitAdoptRefusesUnsafeDirectories(t *testing.T) {
	for _, kind := range []string{"tracked", "repository", "nested-repository", "reserved-metadata", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			env := setupTestEnv(t)
			ctx := context.Background()
			docs := filepath.Join(env.codeDir, "devdocs")
			actual := docs
			if kind == "symlink" {
				actual = filepath.Join(env.rootDir, "external-docs")
			}
			if err := os.MkdirAll(actual, 0755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(actual, "notes.md")
			if err := os.WriteFile(path, []byte("original notes"), 0644); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "tracked":
				if err := RunGit(ctx, env.codeDir, "add", "-f", "devdocs/notes.md"); err != nil {
					t.Fatal(err)
				}
			case "repository":
				if err := RunGit(ctx, actual, "init"); err != nil {
					t.Fatal(err)
				}
			case "nested-repository":
				if err := os.MkdirAll(filepath.Join(actual, "nested", ".git"), 0755); err != nil {
					t.Fatal(err)
				}
			case "reserved-metadata":
				if err := os.WriteFile(filepath.Join(actual, ".devdocs.json"), []byte("belongs to the user"), 0644); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(actual, docs); err != nil {
					t.Fatal(err)
				}
			}
			before, err := GitCommand(ctx, env.codeDir, "ls-files", "--stage", "-z").Output()
			if err != nil {
				t.Fatal(err)
			}
			if code, _, _ := runCLI(ctx, env.codeDir, "init", "--adopt"); code == 0 {
				t.Fatal("adopted an unsafe existing directory")
			}
			if contents, err := os.ReadFile(path); err != nil || string(contents) != "original notes" {
				t.Fatalf("failed adoption changed original notes: %q, %v", contents, err)
			}
			if kind == "reserved-metadata" {
				if contents, err := os.ReadFile(filepath.Join(actual, ".devdocs.json")); err != nil || string(contents) != "belongs to the user" {
					t.Fatalf("overwrote existing reserved metadata: %q, %v", contents, err)
				}
			} else if _, err := os.Lstat(filepath.Join(actual, ".devdocs.json")); !os.IsNotExist(err) {
				t.Fatalf("failed adoption created metadata: %v", err)
			}
			after, err := GitCommand(ctx, env.codeDir, "ls-files", "--stage", "-z").Output()
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("failed adoption changed source index: %v", err)
			}
		})
	}
}

func TestInitAdoptRollsBackFailedCommit(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	docs := filepath.Join(env.codeDir, "devdocs")
	if err := os.MkdirAll(docs, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "fyi.md"), []byte("existing decisions"), 0644); err != nil {
		t.Fatal(err)
	}
	hooks := filepath.Join(env.rootDir, "hooks")
	if err := os.Mkdir(hooks, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := RunGit(ctx, "", "config", "--global", "core.hooksPath", hooks); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := runCLI(ctx, env.codeDir, "init", "--adopt"); code == 0 {
		t.Fatal("expected commit failure")
	}
	entries, err := os.ReadDir(docs)
	if err != nil || len(entries) != 1 || entries[0].Name() != "fyi.md" {
		t.Fatalf("failed adoption left generated data in the original tree: %v, %v", entries, err)
	}
	if contents, err := os.ReadFile(filepath.Join(docs, "fyi.md")); err != nil || string(contents) != "existing decisions" {
		t.Fatalf("rollback changed original data: %q, %v", contents, err)
	}
	if err := RunGit(ctx, "", "config", "--global", "--unset", "core.hooksPath"); err != nil {
		t.Fatal(err)
	}
	if code, out, errOut := runCLI(ctx, env.codeDir, "init", "--adopt"); code != 0 {
		t.Fatalf("retry after correcting the commit failure failed: %s %s", out, errOut)
	}
}

func TestInitAdoptHonorsExistingIgnoreRules(t *testing.T) {
	for _, hideMetadata := range []bool{false, true} {
		t.Run(map[bool]string{false: "existing-templates-ignored", true: "new-metadata-ignored"}[hideMetadata], func(t *testing.T) {
			env := setupTestEnv(t)
			ctx := context.Background()
			docs := filepath.Join(env.codeDir, "devdocs")
			if err := os.MkdirAll(docs, 0755); err != nil {
				t.Fatal(err)
			}
			rules := "README.md\nFYI.md\n*.log\n"
			if hideMetadata {
				rules += ".devdocs.json\n"
			}
			for name, contents := range map[string]string{"README.md": "custom readme", "fyi.md": "custom decisions", ".gitignore": rules, "notes.md": "retained"} {
				if err := os.WriteFile(filepath.Join(docs, name), []byte(contents), 0644); err != nil {
					t.Fatal(err)
				}
			}
			code, out, errOut := runCLI(ctx, env.codeDir, "init", "--adopt")
			if hideMetadata {
				if code == 0 || !strings.Contains(errOut, ".devdocs.json") {
					t.Fatalf("did not diagnose ignored required metadata: %s %s", out, errOut)
				}
				if _, err := os.Lstat(filepath.Join(docs, ".git")); !os.IsNotExist(err) {
					t.Fatalf("installed Git metadata after failed adoption: %v", err)
				}
			} else {
				if code != 0 {
					t.Fatalf("existing ignored templates prevented adoption: %s %s", out, errOut)
				}
				if blob, err := GitCommand(ctx, docs, "show", "HEAD:notes.md").Output(); err != nil || string(blob) != "retained" {
					t.Fatalf("did not snapshot non-ignored notes: %q, %v", blob, err)
				}
				if _, err := GitCommand(ctx, docs, "show", "HEAD:README.md").Output(); err == nil {
					t.Fatal("force-added an ignored existing template")
				}
			}
			if data, err := os.ReadFile(filepath.Join(docs, "README.md")); err != nil || string(data) != "custom readme" {
				t.Fatalf("overwrote existing ignored template: %q, %v", data, err)
			}
		})
	}
}
