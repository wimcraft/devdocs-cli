package devdocs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Status inspects and displays the offline status of the devdocs workspace.
// Returns an error if any safety failure or unfinished Git operation is detected.
func Status(ctx context.Context, out, errOut io.Writer) error {
	if err := CheckGitVersion(ctx); err != nil {
		return err
	}

	ws, err := DiscoverWorkspace(ctx, "")
	if err != nil {
		return err
	}

	if _, err := os.Stat(ws.DocsDir); err != nil {
		return fmt.Errorf("devdocs workspace not found in %s; run 'devdocs init' or 'devdocs attach' first", ws.DocsDir)
	}

	meta, err := ReadMetadata(ws.DocsDir)
	if err != nil {
		// Conflict markers make metadata unparseable; name the merge, not the JSON error.
		if opErr := CheckGitOperationInProgress(ctx, ws.DocsDir); opErr != nil {
			return opErr
		}
		return fmt.Errorf("reading metadata: %w", err)
	}

	state, err := ReadLocalState(ws.DocsDir)
	if err != nil {
		return fmt.Errorf("reading local state: %w", err)
	}

	var safetyFailures []string

	// Check code repository exclusions and tracking for devdocs
	if err := CheckTrackedInCodeRepo(ctx, ws.CodeRoot, "devdocs"); err != nil {
		safetyFailures = append(safetyFailures, fmt.Sprintf("devdocs path is tracked in code repository index: %v", err))
	}
	if err := CheckEffectiveIgnore(ctx, ws.CodeRoot, "devdocs"); err != nil {
		safetyFailures = append(safetyFailures, fmt.Sprintf("devdocs exclusion is ineffective: %v", err))
	}

	// Check installed link destinations for tracking and effective exclusion
	installedMap := make(map[string]bool)
	for _, inst := range state.InstalledLinks {
		installedMap[inst.Destination] = true
		if err := CheckTrackedInCodeRepo(ctx, ws.CodeRoot, inst.Destination); err != nil {
			safetyFailures = append(safetyFailures, fmt.Sprintf("installed link destination %q is tracked in code repository index: %v", inst.Destination, err))
		}
		if err := CheckEffectiveIgnore(ctx, ws.CodeRoot, inst.Destination); err != nil {
			safetyFailures = append(safetyFailures, fmt.Sprintf("installed link destination %q exclusion is ineffective: %v", inst.Destination, err))
		}
	}

	// For declared links not locally installed, check if tracked in code index or colliding with an unexcluded path
	for _, l := range meta.Links {
		if !installedMap[l.Destination] {
			if err := CheckTrackedInCodeRepo(ctx, ws.CodeRoot, l.Destination); err != nil {
				safetyFailures = append(safetyFailures, fmt.Sprintf("declared link destination %q is tracked in code repository index: %v", l.Destination, err))
			}
			destFull := filepath.Join(ws.CodeRoot, l.Destination)
			if _, err := os.Lstat(destFull); err == nil {
				if err := CheckEffectiveIgnore(ctx, ws.CodeRoot, l.Destination); err != nil {
					safetyFailures = append(safetyFailures, fmt.Sprintf("declared link destination %q exists on disk but exclusion is ineffective: %v", l.Destination, err))
				}
			}
		}
	}
	// Check active Git operations in docs
	gitOpErr := CheckGitOperationInProgress(ctx, ws.DocsDir)

	fmt.Fprintf(out, "Project:     %s (id: %s)\n", meta.Name, meta.ProjectID)
	fmt.Fprintf(out, "Docs Path:   %s\n", ws.DocsDir)
	fmt.Fprintf(out, "Branch:      %s\n", meta.Branch)
	fmt.Fprintf(out, "Remote:      %s\n", RedactRemoteURL(state.Remote))

	if state.Published {
		fmt.Fprintf(out, "Publication: published\n")
		// Check ahead/behind against cached remote tracking branch
		trackingRef := "refs/remotes/origin/" + state.Branch
		if _, err := RunGitOutput(ctx, ws.DocsDir, "rev-parse", "--verify", trackingRef); err == nil {
			revCount, err := RunGitOutput(ctx, ws.DocsDir, "rev-list", "--left-right", "--count", "HEAD..."+trackingRef)
			if err == nil {
				parts := strings.Fields(revCount)
				if len(parts) >= 2 {
					ahead, _ := strconv.Atoi(parts[0])
					behind, _ := strconv.Atoi(parts[1])
					fmt.Fprintf(out, "Sync:        %d ahead, %d behind (against last fetched remote state; may be stale)\n", ahead, behind)
				}
			}
		} else {
			fmt.Fprintf(out, "Sync:        no remote tracking branch fetched yet (may be stale)\n")
		}
	} else {
		fmt.Fprintf(out, "Publication: unpublished (local only; publish with 'devdocs sync')\n")
		fmt.Fprintf(out, "Sync:        local commits not yet pushed\n")
	}

	// Git operation state
	if gitOpErr != nil {
		fmt.Fprintf(out, "\nGit Operation In Progress:\n  %v\n", gitOpErr)
	}

	// Working tree changes & ignored files
	statusBytes, err := GitCommand(ctx, ws.DocsDir, "status", "--porcelain=v1", "-z", "--ignored", "--untracked-files=all").Output()
	if err != nil {
		return fmt.Errorf("reading docs working tree status: %w", err)
	}
	var staged []string
	var unstaged []string
	var untracked []string
	var ignored []string

	records := strings.Split(string(statusBytes), "\x00")
	for i := 0; i < len(records); i++ {
		line := records[i]
		if len(line) < 3 {
			continue
		}
		prefix := line[:2]
		path := line[3:]
		if strings.ContainsAny(prefix, "RC") {
			i++ // Rename/copy records include a second, original pathname.
		}

		switch {
		case prefix == "!!":
			ignored = append(ignored, path)
		case prefix == "??":
			untracked = append(untracked, path)
		default:
			if prefix[0] != ' ' && prefix[0] != '?' {
				staged = append(staged, fmt.Sprintf("[%c] %s", prefix[0], path))
			}
			if prefix[1] != ' ' && prefix[1] != '?' {
				unstaged = append(unstaged, fmt.Sprintf("[%c] %s", prefix[1], path))
			}
		}
	}

	fmt.Fprintf(out, "\nWorking Tree:\n")
	if len(staged) == 0 && len(unstaged) == 0 && len(untracked) == 0 {
		fmt.Fprintf(out, "  Clean (no changes to commit)\n")
	} else {
		if len(staged) > 0 {
			fmt.Fprintf(out, "  Staged changes (%d):\n", len(staged))
			for _, s := range staged {
				fmt.Fprintf(out, "    %s\n", s)
			}
		}
		if len(unstaged) > 0 {
			fmt.Fprintf(out, "  Unstaged changes (%d):\n", len(unstaged))
			for _, u := range unstaged {
				fmt.Fprintf(out, "    %s\n", u)
			}
		}
		if len(untracked) > 0 {
			fmt.Fprintf(out, "  Untracked files (%d):\n", len(untracked))
			for _, ut := range untracked {
				fmt.Fprintf(out, "    %s\n", ut)
			}
		}
	}

	if len(ignored) > 0 {
		fmt.Fprintf(out, "  Ignored paths (%d; omitted from sync):\n", len(ignored))
		for _, ign := range ignored {
			fmt.Fprintf(out, "    %s\n", ign)
		}
	}

	// Link health
	fmt.Fprintf(out, "\nLinks:\n")
	declaredDests := make(map[string]bool)
	if len(meta.Links) == 0 && len(state.InstalledLinks) == 0 {
		fmt.Fprintf(out, "  None declared or installed\n")
	} else {
		for _, l := range meta.Links {
			declaredDests[l.Destination] = true
			destFull := filepath.Join(ws.CodeRoot, l.Destination)
			sourceFull := filepath.Join(ws.DocsDir, l.Source)
			expectedRel, _ := ComputeRelativeLinkTarget(destFull, sourceFull)

			info, err := os.Lstat(destFull)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Fprintf(out, "  %s -> %s [missing; not installed]\n", l.Destination, l.Source)
				} else {
					fmt.Fprintf(out, "  %s -> %s [error: %v]\n", l.Destination, l.Source, err)
				}
			} else if info.Mode()&os.ModeSymlink != 0 {
				target, _ := os.Readlink(destFull)
				if target == expectedRel {
					fmt.Fprintf(out, "  %s -> %s [healthy]\n", l.Destination, l.Source)
				} else {
					fmt.Fprintf(out, "  %s -> %s [changed: points to %q, expected %q]\n", l.Destination, l.Source, target, expectedRel)
				}
			} else {
				fmt.Fprintf(out, "  %s -> %s [changed: replaced by regular file/directory]\n", l.Destination, l.Source)
			}
		}

		// Check for stale installed links whose declaration was removed remotely
		for _, inst := range state.InstalledLinks {
			if !declaredDests[inst.Destination] {
				fmt.Fprintf(out, "  %s -> %s [stale: declaration was removed remotely; run 'devdocs unlink %s' to remove]\n",
					inst.Destination, inst.Target, inst.Destination)
			}
		}
	}

	// Safety diagnostics
	if len(safetyFailures) > 0 {
		fmt.Fprintf(out, "\nRepository Isolation Failures:\n")
		for _, sf := range safetyFailures {
			fmt.Fprintf(out, "  - %s\n", sf)
		}
	}

	// Return non-zero status if safety failure or git operation in progress
	if len(safetyFailures) > 0 {
		return fmt.Errorf("repository isolation safety check failed: %d issue(s) detected", len(safetyFailures))
	}
	if gitOpErr != nil {
		return gitOpErr
	}

	return nil
}
