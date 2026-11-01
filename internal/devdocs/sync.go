package devdocs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Sync synchronizes the devdocs workspace with its remote branch.
func Sync(ctx context.Context, message string, out, errOut io.Writer) error {
	if err := CheckGitVersion(ctx); err != nil {
		return err
	}

	ws, err := DiscoverWorkspace(ctx, "")
	if err != nil {
		return err
	}

	// Acquire workspace lock before reading mutable state
	lock, err := AcquireWorkspaceLock(ws.CodeRoot, ws.CommonGitDir)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Unlock() }()

	docsInfo, err := os.Lstat(ws.DocsDir)
	if err != nil {
		return fmt.Errorf("devdocs workspace not found in %s; run 'devdocs init' or 'devdocs attach' first", ws.DocsDir)
	}
	if docsInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("devdocs workspace %s is a symlink; symlinked workspaces are rejected", ws.DocsDir)
	}

	// Conflict markers can make metadata unparseable, so report the merge first
	if err := CheckGitOperationInProgress(ctx, ws.DocsDir); err != nil {
		return err
	}

	// Validate metadata and local state under lock
	meta, err := ReadMetadata(ws.DocsDir)
	if err != nil {
		return fmt.Errorf("validating workspace metadata: %w", err)
	}
	state, err := ReadLocalState(ws.DocsDir)
	if err != nil {
		return fmt.Errorf("validating workspace local state: %w", err)
	}

	// Validate workspace branch, remote, pushurl, refspec, clean operation state, and isolation
	if err := ValidateWorkspacePins(ctx, ws, meta, state); err != nil {
		return err
	}

	// Verify Git author identity before staging
	if _, _, _, err := CheckAuthorIdentity(ctx, ws.DocsDir); err != nil {
		return err
	}

	// git add -A would record a nested repository as an empty gitlink, so a
	// successful sync would upload none of its files. Git lists only those
	// untracked entries with a trailing slash.
	untracked, err := RunGitLines(ctx, ws.DocsDir, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return fmt.Errorf("inspecting untracked files: %w", err)
	}
	for _, path := range untracked {
		if strings.HasSuffix(path, "/") {
			return fmt.Errorf("%s is a nested Git repository and its files would not be synchronized; remove its .git directory or ignore it", path)
		}
	}

	// Snapshot all non-ignored changes into a local commit
	if err := RunGit(ctx, ws.DocsDir, "add", "-A"); err != nil {
		return fmt.Errorf("snapshotting workspace changes: %w", err)
	}

	// Check if there are changes staged for commit
	hasLocalChanges := false
	diffCmd := GitCommand(ctx, ws.DocsDir, "diff-index", "--quiet", "HEAD", "--")
	if err := diffCmd.Run(); err != nil {
		// Exit code 1 indicates differences exist
		hasLocalChanges = true
	}

	if hasLocalChanges {
		commitMsg := strings.TrimSpace(message)
		if commitMsg == "" {
			commitMsg = fmt.Sprintf("devdocs: sync %s", time.Now().UTC().Format(time.RFC3339))
		}
		if err := RunGit(ctx, ws.DocsDir, "commit", "-m", commitMsg); err != nil {
			return fmt.Errorf("committing workspace snapshot: %w", err)
		}
		fmt.Fprintf(out, "Created snapshot commit: %s\n", commitMsg)
	}

	// Query remote branch existence
	lsOut, err := RunGitOutput(ctx, ws.DocsDir, "ls-remote", "--heads", "origin", "refs/heads/"+state.Branch)
	if err != nil {
		return fmt.Errorf("connecting to remote %s: %w", RedactRemoteURL(state.Remote), err)
	}

	remoteBranchExists := strings.TrimSpace(lsOut) != ""

	// Check publication integrity: previously published branch disappearing is an error
	if state.Published && !remoteBranchExists {
		return fmt.Errorf("previously published branch %q no longer exists on remote; refusing to silently recreate", state.Branch)
	}

	refspec := fmt.Sprintf("refs/heads/%s:refs/remotes/origin/%s", state.Branch, state.Branch)

	if remoteBranchExists {
		// Observing valid upstream on remote transitions state to published
		if !state.Published {
			state.Published = true
			if err := WriteLocalState(ws.DocsDir, state); err != nil {
				return fmt.Errorf("recording publication state: %w", err)
			}
		}

		// Fetch only the exact project branch without tags
		if err := RunGit(ctx, ws.DocsDir, "fetch", "--no-tags", "origin", refspec); err != nil {
			return fmt.Errorf("fetching branch %s from remote: %w", state.Branch, err)
		}

		// Pre-merge inspection of remote .devdocs.json
		remoteMetaJSON, err := RunGitOutput(ctx, ws.DocsDir, "show", "refs/remotes/origin/"+state.Branch+":.devdocs.json")
		if err != nil {
			return fmt.Errorf("reading remote metadata from branch %s: %w", state.Branch, err)
		}

		var remoteMeta Metadata
		if err := json.Unmarshal([]byte(remoteMetaJSON), &remoteMeta); err != nil {
			return fmt.Errorf("parsing remote metadata from branch %s: %w", state.Branch, err)
		}
		if err := ValidateMetadata(&remoteMeta); err != nil {
			return fmt.Errorf("validating remote metadata from branch %s: %w", state.Branch, err)
		}
		if remoteMeta.ProjectID != state.ProjectID {
			return fmt.Errorf("remote branch project_id %q does not match local project_id %q; refusing to merge unrelated project", remoteMeta.ProjectID, state.ProjectID)
		}
		if remoteMeta.Branch != state.Branch {
			return fmt.Errorf("remote branch name %q does not match local branch %q; refusing to merge", remoteMeta.Branch, state.Branch)
		}

		// Verify merge base
		baseCommit, err := RunGitOutput(ctx, ws.DocsDir, "merge-base", "HEAD", "refs/remotes/origin/"+state.Branch)
		if err != nil || baseCommit == "" {
			return fmt.Errorf("local branch and remote branch %s have unrelated histories; refusing to merge", state.Branch)
		}

		localHead, err := RunGitOutput(ctx, ws.DocsDir, "rev-parse", "HEAD")
		if err != nil {
			return fmt.Errorf("reading local HEAD: %w", err)
		}
		remoteHead, err := RunGitOutput(ctx, ws.DocsDir, "rev-parse", "refs/remotes/origin/"+state.Branch)
		if err != nil {
			return fmt.Errorf("reading remote tracking ref: %w", err)
		}

		if remoteHead == baseCommit {
			// Already integrated, including identical heads. Still push below to
			// confirm publication through the configured receive service.
		} else if localHead == baseCommit {
			// Local is strictly behind remote: fast-forward merge
			if err := RunGit(ctx, ws.DocsDir, "merge", "--ff-only", "refs/remotes/origin/"+state.Branch); err != nil {
				return fmt.Errorf("fast-forwarding to remote: %w", err)
			}
			fmt.Fprintln(out, "Fast-forwarded workspace to latest remote changes.")
		} else {
			// Diverged: attempt normal 3-way merge
			mergeErr := RunGit(ctx, ws.DocsDir, "merge", "--no-edit", "refs/remotes/origin/"+state.Branch)
			if mergeErr != nil {
				// Check for merge conflicts
				conflicts, _ := RunGitLines(ctx, ws.DocsDir, "diff", "--name-only", "--diff-filter=U")
				if len(conflicts) > 0 {
					return fmt.Errorf("merge conflict occurred during sync\n\nConflicting paths:\n  %s\n\nTo resolve the conflict:\n  1. Edit and resolve the reported files in %s\n  2. Stage resolved files: devdocs git add -- <resolved-paths>\n  3. Commit the resolution: devdocs git commit\n  4. Retry sync: devdocs sync\n\nTo abandon the merge attempt and keep local snapshot:\n  devdocs git merge --abort",
						strings.Join(conflicts, "\n  "), ws.DocsDir)
				}
				return fmt.Errorf("merging remote changes: %w", mergeErr)
			}
			fmt.Fprintln(out, "Integrated remote changes via 3-way merge.")
		}

		// Post-merge validation of metadata: verify both schema and pinned project identity
		postMeta, err := ReadMetadata(ws.DocsDir)
		if err != nil {
			return fmt.Errorf("post-merge metadata validation failed: %w", err)
		}
		if postMeta.ProjectID != state.ProjectID || postMeta.Branch != state.Branch {
			return fmt.Errorf("post-merge metadata project identity mismatch (project: %s, branch: %s)", postMeta.ProjectID, postMeta.Branch)
		}
	}

	// Push only the expected project branch without force
	pushTarget := fmt.Sprintf("HEAD:refs/heads/%s", state.Branch)
	if err := RunGit(ctx, ws.DocsDir, "push", "origin", pushTarget); err != nil {
		return fmt.Errorf("push rejected by remote (remote may have advanced concurrently): %w\nrun 'devdocs sync' again to integrate remote changes", err)
	}

	// Mark published if not already
	if !state.Published {
		state.Published = true
		if err := WriteLocalState(ws.DocsDir, state); err != nil {
			return fmt.Errorf("updating publication state: %w", err)
		}
	}

	// Check if any local non-ignored changes appeared during sync
	concurrentCheck, err := RunGitOutput(ctx, ws.DocsDir, "status", "--porcelain")
	if err == nil && strings.TrimSpace(concurrentCheck) != "" {
		fmt.Fprintln(out, "\nNotice: concurrent unpublished changes remain in workspace (not included in this sync).")
	}

	fmt.Fprintf(out, "Synchronized project branch %s with remote.\n", state.Branch)
	return nil
}
