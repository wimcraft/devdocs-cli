package devdocs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Init initializes a new devdocs project in the current code repository.
func Init(ctx context.Context, name string, adopt bool, out, errOut io.Writer) error {
	if err := CheckGitVersion(ctx); err != nil {
		return err
	}

	ws, err := DiscoverWorkspace(ctx, "")
	if err != nil {
		return err
	}

	authorName, authorEmail, isLocalAuthor, err := CheckAuthorIdentity(ctx, ws.CodeRoot)
	if err != nil {
		return err
	}

	cfg, _, err := LoadConfig()
	if err != nil {
		return err
	}
	if cfg.DefaultRemote == "" {
		return errors.New("no default remote configured; run 'devdocs config --remote <git-url>' first")
	}

	// Lock workspace for mutation
	lock, err := AcquireWorkspaceLock(ws.CodeRoot, ws.CommonGitDir)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Unlock() }()

	adopting := false
	var existingTemplates map[string]bool

	// Check if already initialized and valid
	if fi, err := os.Lstat(ws.DocsDir); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("existing path %s is a symlink; refusing to overwrite or adopt", ws.DocsDir)
		}
		if !fi.IsDir() {
			return fmt.Errorf("existing path %s is not a directory; refusing to overwrite or adopt", ws.DocsDir)
		}
		meta, metaErr := ReadMetadata(ws.DocsDir)
		state, stateErr := ReadLocalState(ws.DocsDir)
		if metaErr == nil && stateErr == nil {
			if err := ValidateWorkspacePins(ctx, ws, meta, state); err != nil {
				return err
			}
			fmt.Fprintf(out, "devdocs already initialized in %s\nProject: %s (id: %s)\nBranch: %s\nRemote: %s\n",
				ws.DocsDir, meta.Name, meta.ProjectID, meta.Branch, state.Remote)
			return nil
		}
		if !adopt {
			return fmt.Errorf("existing path %s is not initialized by Devdocs; use 'devdocs init --adopt' to initialize unmanaged documents without overwriting them", ws.DocsDir)
		}
		existingTemplates, err = adoptionEntries(ctx, ws.DocsDir)
		if err != nil {
			return err
		}
		adopting = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspecting %s: %w", ws.DocsDir, err)
	}

	// Update and verify code repository exclusions
	if err := UpdateExclusions(ws.CommonGitDir, nil); err != nil {
		return fmt.Errorf("updating code repository exclusions: %w", err)
	}
	if err := CheckEffectiveIgnore(ctx, ws.CodeRoot, "devdocs"); err != nil {
		return err
	}
	if err := CheckTrackedInCodeRepo(ctx, ws.CodeRoot, "devdocs"); err != nil {
		return err
	}

	projectUUID, err := NewUUID()
	if err != nil {
		return err
	}

	displayName := strings.TrimSpace(name)
	if displayName == "" {
		displayName = filepath.Base(ws.CodeRoot)
	}
	slug := SanitizeSlug(displayName)
	branch := ProjectBranchName(slug, projectUUID)

	stagingDir, err := os.MkdirTemp(ws.CommonGitDir, "devdocs-staging-*")
	if err != nil {
		return fmt.Errorf("creating staging directory: %w", err)
	}
	success := false
	var created initialFiles
	defer func() {
		if !success && adopting {
			created.rollback()
		}
		_ = os.RemoveAll(stagingDir)
	}()

	// Initialize git repository in staging
	if err := RunGit(ctx, stagingDir, "init"); err != nil {
		return fmt.Errorf("initializing git in staging: %w", err)
	}
	if err := RunGit(ctx, stagingDir, "checkout", "-b", branch); err != nil {
		return fmt.Errorf("creating initial branch %s: %w", branch, err)
	}

	// Configure remote and fetch refspec
	if err := RunGit(ctx, stagingDir, "remote", "add", "origin", cfg.DefaultRemote); err != nil {
		return fmt.Errorf("adding origin remote: %w", err)
	}
	refspec := fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", branch, branch)
	if err := RunGit(ctx, stagingDir, "config", "remote.origin.fetch", refspec); err != nil {
		return fmt.Errorf("configuring fetch refspec: %w", err)
	}
	if err := RunGit(ctx, stagingDir, "config", "remote.origin.tagOpt", "--no-tags"); err != nil {
		return fmt.Errorf("configuring tag option: %w", err)
	}

	// If author identity was code-local, copy to docs config
	if isLocalAuthor {
		if err := RunGit(ctx, stagingDir, "config", "user.name", authorName); err != nil {
			return err
		}
		if err := RunGit(ctx, stagingDir, "config", "user.email", authorEmail); err != nil {
			return err
		}
	}

	worktree := stagingDir
	if adopting {
		worktree = ws.DocsDir
	}
	preparedGitDir := filepath.Join(stagingDir, ".git")
	requiredFiles := []string{".devdocs.json"}
	for _, template := range []string{"README.md", "FYI.md"} {
		if !existingTemplates[strings.ToLower(template)] {
			requiredFiles = append(requiredFiles, template)
		}
	}
	if err := checkInitialIgnores(ctx, worktree, preparedGitDir, requiredFiles); err != nil {
		return err
	}

	createdAt := time.Now().UTC().Format(time.RFC3339)
	meta := &Metadata{
		SchemaVersion: SchemaVersionCurrent,
		ProjectID:     projectUUID,
		Branch:        branch,
		Name:          displayName,
		CreatedAt:     createdAt,
		Links:         []LinkDeclaration{},
	}
	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding initial metadata: %w", err)
	}
	if err := created.create(worktree, ".devdocs.json", append(metaJSON, '\n')); err != nil {
		return err
	}

	if !existingTemplates["readme.md"] {
		readmeContent := fmt.Sprintf(`# Devdocs: %s

This directory contains private development context versioned independently from the code repository.

## Usage
- Edit notes, decision logs, drafts, or scripts here.
- Synchronize changes across machines using 'devdocs sync'.
- Run 'devdocs status' to view uncommitted changes, link health, and sync state.

## Privacy and Storage Notice
- This repository is synchronized to the configured private Git remote.
- Private Git history is not a secret vault: do not store passwords, API keys, credentials, or production data.
- Deleting a file from the working tree does not remove it from Git history.
- Ensure your configured remote repository is private and authorized for project documentation.
`, displayName)

		if err := created.create(worktree, "README.md", []byte(readmeContent)); err != nil {
			return err
		}
	}

	if !existingTemplates["fyi.md"] {
		fyiContent := fmt.Sprintf(`# FYI: Decision Log and Private Notes

Project: %s
Created: %s

## Decisions & Context
- Initialized devdocs workspace.
`, displayName, createdAt)

		if err := created.create(worktree, "FYI.md", []byte(fyiContent)); err != nil {
			return err
		}
	}

	// Use the existing worktree directly during adoption; Git does not follow
	// document symlinks or copy the tree into the metadata staging directory.
	if err := RunGit(ctx, worktree, "--git-dir="+preparedGitDir, "--work-tree="+worktree, "add", "-A"); err != nil {
		return fmt.Errorf("staging initial files: %w", err)
	}
	commitMsg := fmt.Sprintf("devdocs: initialize %s", displayName)
	if err := RunGit(ctx, worktree, "--git-dir="+preparedGitDir, "--work-tree="+worktree, "commit", "-m", commitMsg); err != nil {
		return fmt.Errorf("committing initial files: %w", err)
	}

	var ignored []byte
	if adopting {
		ignored, err = GitCommand(ctx, worktree, "--git-dir="+preparedGitDir, "--work-tree="+worktree,
			"ls-files", "--others", "--ignored", "--exclude-standard", "-z").Output()
		if err != nil {
			return fmt.Errorf("inspecting ignored documents before adoption: %w", err)
		}
	}

	// Write machine-local state in docs .git
	state := &LocalState{
		ProjectID:      projectUUID,
		Branch:         branch,
		Remote:         cfg.DefaultRemote,
		Published:      false,
		InstalledLinks: []InstalledLink{},
	}
	if err := WriteLocalState(stagingDir, state); err != nil {
		return fmt.Errorf("writing initial local state: %w", err)
	}

	if adopting {
		destination := filepath.Join(ws.DocsDir, ".git")
		if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("Git metadata appeared at %s during adoption; refusing to replace it", destination)
		}
		if err := os.Rename(preparedGitDir, destination); err != nil {
			return fmt.Errorf("installing adopted workspace Git metadata: %w", err)
		}
	} else if err := os.Rename(stagingDir, ws.DocsDir); err != nil {
		return fmt.Errorf("moving staging directory into place %s: %w", ws.DocsDir, err)
	}
	success = true

	fmt.Fprintf(out, "Initialized devdocs project %q (%s)\nBranch: %s\nRemote: %s (unpublished)\nDocs path: %s\n",
		displayName, projectUUID, branch, RedactRemoteURL(cfg.DefaultRemote), ws.DocsDir)
	if adopting {
		fmt.Fprintln(out, "Existing documents preserved in place and committed locally; nothing was uploaded.")
		if len(ignored) != 0 {
			fmt.Fprintln(out, "Ignored paths were preserved but not committed:")
			for _, path := range strings.Split(string(ignored), "\x00") {
				if path != "" {
					fmt.Fprintf(out, "  %q\n", path)
				}
			}
		}
	}

	return nil
}

// List discovers and lists available devdocs project branches on the configured remote.
func List(ctx context.Context, out, errOut io.Writer) error {
	if err := CheckGitVersion(ctx); err != nil {
		return err
	}

	cfg, _, err := LoadConfig()
	if err != nil {
		return err
	}
	if cfg.DefaultRemote == "" {
		return errors.New("no default remote configured; run 'devdocs config --remote <git-url>' first")
	}

	rawOut, err := RunGitOutput(ctx, "", "ls-remote", "--heads", cfg.DefaultRemote, "refs/heads/devdocs/*")
	if err != nil {
		return fmt.Errorf("listing remote project branches from %s: %w", RedactRemoteURL(cfg.DefaultRemote), err)
	}

	lines := strings.Split(strings.TrimSpace(rawOut), "\n")
	var branches []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		parts := strings.Split(trimmed, "\t")
		if len(parts) >= 2 {
			ref := parts[1]
			branchName := strings.TrimPrefix(ref, "refs/heads/")
			branches = append(branches, branchName)
		}
	}

	if len(branches) == 0 {
		fmt.Fprintf(out, "No devdocs projects found on remote %s.\n", RedactRemoteURL(cfg.DefaultRemote))
		return nil
	}

	fmt.Fprintf(out, "Available devdocs projects on %s:\n", RedactRemoteURL(cfg.DefaultRemote))
	for _, b := range branches {
		slug, uuid, err := ParseBranchName(b)
		if err == nil {
			fmt.Fprintf(out, "  %s  (slug: %s, id: %s)\n", b, slug, uuid)
		} else {
			fmt.Fprintf(out, "  %s\n", b)
		}
	}

	return nil
}

// Attach clones an existing devdocs project branch into the current code repository.
func Attach(ctx context.Context, branch string, out, errOut io.Writer) error {
	if err := CheckGitVersion(ctx); err != nil {
		return err
	}

	branch = strings.TrimSpace(branch)
	if branch == "" {
		return errors.New("branch name cannot be empty")
	}
	if _, _, err := ParseBranchName(branch); err != nil {
		return fmt.Errorf("invalid project branch %q: %w", branch, err)
	}

	cfg, _, err := LoadConfig()
	if err != nil {
		return err
	}
	if cfg.DefaultRemote == "" {
		return errors.New("no default remote configured; run 'devdocs config --remote <git-url>' first")
	}

	ws, err := DiscoverWorkspace(ctx, "")
	if err != nil {
		return err
	}

	authorName, authorEmail, isLocalAuthor, err := CheckAuthorIdentity(ctx, ws.CodeRoot)
	if err != nil {
		return err
	}

	// Lock workspace for mutation
	lock, err := AcquireWorkspaceLock(ws.CodeRoot, ws.CommonGitDir)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Unlock() }()

	// Check if already attached and valid
	if fi, err := os.Lstat(ws.DocsDir); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("existing path %s is a symlink; refusing to overwrite", ws.DocsDir)
		}
		if !fi.IsDir() {
			return fmt.Errorf("existing path %s is not a directory; refusing to overwrite", ws.DocsDir)
		}
		meta, metaErr := ReadMetadata(ws.DocsDir)
		state, stateErr := ReadLocalState(ws.DocsDir)
		if metaErr == nil && stateErr == nil && meta.Branch == branch && state.Branch == branch {
			if err := ValidateWorkspacePins(ctx, ws, meta, state); err == nil {
				fmt.Fprintf(out, "Already attached to project branch %s (id: %s) in %s\n", branch, meta.ProjectID, ws.DocsDir)
				return nil
			}
		}
		return fmt.Errorf("existing path %s already exists and does not match requested branch %s; refusing to overwrite", ws.DocsDir, branch)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspecting %s: %w", ws.DocsDir, err)
	}

	// Update and verify exclusions before exposing files
	if err := UpdateExclusions(ws.CommonGitDir, nil); err != nil {
		return fmt.Errorf("updating exclusions: %w", err)
	}
	if err := CheckEffectiveIgnore(ctx, ws.CodeRoot, "devdocs"); err != nil {
		return err
	}
	if err := CheckTrackedInCodeRepo(ctx, ws.CodeRoot, "devdocs"); err != nil {
		return err
	}

	stagingDir, err := os.MkdirTemp(ws.CommonGitDir, "devdocs-staging-attach-*")
	if err != nil {
		return fmt.Errorf("creating staging directory: %w", err)
	}
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(stagingDir)
		}
	}()

	// Clone exact branch with no tags and single-branch
	cloneArgs := []string{
		"clone",
		"--single-branch",
		"--no-tags",
		"--branch", branch,
		cfg.DefaultRemote,
		stagingDir,
	}
	if err := RunGit(ctx, "", cloneArgs...); err != nil {
		return fmt.Errorf("cloning branch %s from %s: %w", branch, RedactRemoteURL(cfg.DefaultRemote), err)
	}

	// Validate cloned metadata
	meta, err := ReadMetadata(stagingDir)
	if err != nil {
		return fmt.Errorf("validating cloned project metadata: %w", err)
	}
	if meta.Branch != branch {
		return fmt.Errorf("cloned metadata branch %q does not match requested branch %q", meta.Branch, branch)
	}

	// Configure fetch refspec and disable tags
	refspec := fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", branch, branch)
	_ = RunGit(ctx, stagingDir, "config", "remote.origin.fetch", refspec)
	_ = RunGit(ctx, stagingDir, "config", "remote.origin.tagOpt", "--no-tags")

	if isLocalAuthor {
		_ = RunGit(ctx, stagingDir, "config", "user.name", authorName)
		_ = RunGit(ctx, stagingDir, "config", "user.email", authorEmail)
	}

	// Write machine-local state
	state := &LocalState{
		ProjectID:      meta.ProjectID,
		Branch:         branch,
		Remote:         cfg.DefaultRemote,
		Published:      true, // Observed on remote
		InstalledLinks: []InstalledLink{},
	}
	if err := WriteLocalState(stagingDir, state); err != nil {
		return fmt.Errorf("writing local state: %w", err)
	}

	// Move staging into place
	if err := os.Rename(stagingDir, ws.DocsDir); err != nil {
		return fmt.Errorf("moving cloned repository into %s: %w", ws.DocsDir, err)
	}
	success = true

	fmt.Fprintf(out, "Attached devdocs project %q (%s)\nBranch: %s\nDocs path: %s\n",
		meta.Name, meta.ProjectID, branch, ws.DocsDir)

	if len(meta.Links) > 0 {
		fmt.Fprintf(out, "\nDeclared links in project metadata (not applied automatically):\n")
		for _, l := range meta.Links {
			fmt.Fprintf(out, "  %s -> %s\n", l.Source, l.Destination)
		}
		fmt.Fprintf(out, "Run 'devdocs link --apply' to install these links on this machine.\n")
	}

	return nil
}
