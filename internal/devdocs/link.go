package devdocs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Link creates a relative symlink from a code-root destination to a docs-relative source
// and records the declaration in .devdocs.json and local ownership in devdocs-state.json.
func Link(ctx context.Context, source, destination string, out, errOut io.Writer) error {
	if err := CheckGitVersion(ctx); err != nil {
		return err
	}

	trimmedSource := strings.TrimSpace(source)
	trimmedDest := strings.TrimSpace(destination)

	// Validate raw paths before normalization to reject hidden traversals
	if err := ValidateSourcePath(trimmedSource); err != nil {
		return err
	}
	if err := ValidateDestinationPath(trimmedDest); err != nil {
		return err
	}

	sourceClean := filepath.ToSlash(filepath.Clean(trimmedSource))
	destClean := filepath.ToSlash(filepath.Clean(trimmedDest))

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

	meta, err := ReadMetadata(ws.DocsDir)
	if err != nil {
		return fmt.Errorf("reading workspace metadata: %w", err)
	}
	state, err := ReadLocalState(ws.DocsDir)
	if err != nil {
		return fmt.Errorf("reading workspace local state: %w", err)
	}

	// Validate workspace branch, remote, refspec, operations, and isolation
	if err := ValidateWorkspacePins(ctx, ws, meta, state); err != nil {
		return err
	}

	sourceFullPath := filepath.Join(ws.DocsDir, sourceClean)
	destFullPath := filepath.Join(ws.CodeRoot, destClean)

	if err := CheckParentSymlinkEscapes(ws.DocsDir, sourceClean); err != nil {
		return err
	}
	if err := CheckParentSymlinkEscapes(ws.CodeRoot, destClean); err != nil {
		return err
	}

	// Verify source content exists and does not escape docs or enter reserved metadata
	if err := CheckSourceSymlinkSafety(ws.DocsDir, sourceFullPath); err != nil {
		return err
	}

	relTarget, err := ComputeRelativeLinkTarget(destFullPath, sourceFullPath)
	if err != nil {
		return err
	}

	// Check existing declarations
	var existingDests []string
	alreadyDeclaredSame := false
	for _, l := range meta.Links {
		if l.Destination == destClean {
			if l.Source == sourceClean {
				alreadyDeclaredSame = true
			} else {
				return fmt.Errorf("destination %q is already declared for source %q", destClean, l.Source)
			}
		}
		existingDests = append(existingDests, l.Destination)
	}

	if !alreadyDeclaredSame {
		if err := CheckDestinationOverlaps(destClean, existingDests); err != nil {
			return err
		}
	}

	// Inspect destination
	destInfo, err := os.Lstat(destFullPath)
	if err == nil {
		if destInfo.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(destFullPath)
			if target == relTarget && alreadyDeclaredSame && isInstalled(state, destClean, relTarget) {
				// Idempotent return must still check code repository index and exclusion safety
				if err := CheckTrackedInCodeRepo(ctx, ws.CodeRoot, destClean); err != nil {
					return err
				}
				if err := CheckEffectiveIgnore(ctx, ws.CodeRoot, destClean); err != nil {
					return err
				}
				fmt.Fprintf(out, "Link %s -> %s is already installed and up to date.\n", destClean, relTarget)
				return nil
			}
			return fmt.Errorf("destination path %q already exists (symlink pointing to %q); refusing to overwrite or adopt", destClean, target)
		}
		return fmt.Errorf("destination path %q already exists as a regular file or directory; refusing to overwrite or adopt", destClean)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspecting destination %s: %w", destFullPath, err)
	}

	// Check if tracked in code repository
	if err := CheckTrackedInCodeRepo(ctx, ws.CodeRoot, destClean); err != nil {
		return err
	}

	// Update code repository exclusions
	if err := UpdateExclusions(ws.CommonGitDir, []string{destClean}); err != nil {
		return fmt.Errorf("updating code repository exclusions: %w", err)
	}
	if err := CheckEffectiveIgnore(ctx, ws.CodeRoot, destClean); err != nil {
		return err
	}

	// Track newly created parent directories for rollback
	destDir := filepath.Dir(destFullPath)
	var createdDirs []string
	curr := destDir
	for curr != ws.CodeRoot && curr != "." && curr != "/" {
		if _, err := os.Lstat(curr); errors.Is(err, os.ErrNotExist) {
			createdDirs = append([]string{curr}, createdDirs...)
			curr = filepath.Dir(curr)
		} else {
			break
		}
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating parent directory %s: %w", destDir, err)
	}

	// Rollback helper for Link
	linkCreated := false
	rollback := func() {
		if linkCreated {
			_ = os.Remove(destFullPath)
		}
		for i := len(createdDirs) - 1; i >= 0; i-- {
			_ = os.Remove(createdDirs[i])
		}
	}

	// Create relative symlink
	if err := os.Symlink(relTarget, destFullPath); err != nil {
		rollback()
		return fmt.Errorf("creating symlink %s -> %s: %w", destFullPath, relTarget, err)
	}
	linkCreated = true

	// Update metadata if not already declared
	if !alreadyDeclaredSame {
		meta.Links = append(meta.Links, LinkDeclaration{
			Source:      sourceClean,
			Destination: destClean,
		})
		if err := WriteMetadata(ws.DocsDir, meta); err != nil {
			rollback()
			return fmt.Errorf("updating .devdocs.json: %w", err)
		}
	}

	// Update local state ownership
	recordInstalledLink(state, destClean, relTarget)
	if err := WriteLocalState(ws.DocsDir, state); err != nil {
		rollback()
		return fmt.Errorf("updating local state: %w", err)
	}

	fmt.Fprintf(out, "Linked %s -> %s\n", destClean, relTarget)
	return nil
}

// ApplyLinks explicitly preflights and applies all declared links in .devdocs.json.
// If any check fails, no changes are made. On failure during creation, newly created links are rolled back.
func ApplyLinks(ctx context.Context, out, errOut io.Writer) error {
	if err := CheckGitVersion(ctx); err != nil {
		return err
	}

	ws, err := DiscoverWorkspace(ctx, "")
	if err != nil {
		return err
	}

	// Lock workspace for mutation before reading mutable state
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

	meta, err := ReadMetadata(ws.DocsDir)
	if err != nil {
		return fmt.Errorf("reading workspace metadata: %w", err)
	}
	state, err := ReadLocalState(ws.DocsDir)
	if err != nil {
		return fmt.Errorf("reading workspace local state: %w", err)
	}

	// Validate workspace pins and isolation
	if err := ValidateWorkspacePins(ctx, ws, meta, state); err != nil {
		return err
	}

	if len(meta.Links) == 0 {
		fmt.Fprintln(out, "No declared links in project metadata.")
		return nil
	}

	fmt.Fprintln(out, "Declared links to apply:")
	for _, l := range meta.Links {
		fmt.Fprintf(out, "  %s -> %s\n", l.Source, l.Destination)
	}

	// Preflight all links
	type plannedLink struct {
		sourceFullPath string
		destFullPath   string
		relTarget      string
		dest           string
		source         string
	}

	var planned []plannedLink
	var allDests []string

	for i, l := range meta.Links {
		if err := ValidateSourcePath(l.Source); err != nil {
			return fmt.Errorf("link[%d] invalid source: %w", i, err)
		}
		if err := ValidateDestinationPath(l.Destination); err != nil {
			return fmt.Errorf("link[%d] invalid destination: %w", i, err)
		}

		sourceClean := filepath.ToSlash(filepath.Clean(l.Source))
		destClean := filepath.ToSlash(filepath.Clean(l.Destination))

		sourceFull := filepath.Join(ws.DocsDir, sourceClean)
		destFull := filepath.Join(ws.CodeRoot, destClean)

		if err := CheckParentSymlinkEscapes(ws.DocsDir, sourceClean); err != nil {
			return err
		}
		if err := CheckParentSymlinkEscapes(ws.CodeRoot, destClean); err != nil {
			return err
		}

		// Verify source content exists and is safe
		if err := CheckSourceSymlinkSafety(ws.DocsDir, sourceFull); err != nil {
			return fmt.Errorf("link[%d] source invalid: %w", i, err)
		}

		relTarget, err := ComputeRelativeLinkTarget(destFull, sourceFull)
		if err != nil {
			return err
		}

		destInfo, err := os.Lstat(destFull)
		if err == nil {
			if destInfo.Mode()&os.ModeSymlink != 0 {
				target, _ := os.Readlink(destFull)
				if target != relTarget {
					return fmt.Errorf("destination %q already exists as a symlink pointing to %q (expected %q); refusing to overwrite", l.Destination, target, relTarget)
				}
			} else {
				return fmt.Errorf("destination %q already exists as a regular file or directory; refusing to overwrite or adopt", l.Destination)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspecting destination %s: %w", destFull, err)
		}

		if err := CheckTrackedInCodeRepo(ctx, ws.CodeRoot, destClean); err != nil {
			return err
		}

		allDests = append(allDests, destClean)
		planned = append(planned, plannedLink{
			sourceFullPath: sourceFull,
			destFullPath:   destFull,
			relTarget:      relTarget,
			dest:           destClean,
			source:         sourceClean,
		})
	}

	// Update and verify exclusions for all destinations
	if err := UpdateExclusions(ws.CommonGitDir, allDests); err != nil {
		return fmt.Errorf("updating code exclusions: %w", err)
	}
	for _, p := range planned {
		if err := CheckEffectiveIgnore(ctx, ws.CodeRoot, p.dest); err != nil {
			return err
		}
	}

	// Apply links tracking newly created filesystem objects for rollback
	var createdSymlinks []string
	var createdDirs []string

	rollback := func() {
		for _, sym := range createdSymlinks {
			_ = os.Remove(sym)
		}
		for i := len(createdDirs) - 1; i >= 0; i-- {
			_ = os.Remove(createdDirs[i])
		}
	}

	appliedCount := 0
	for _, p := range planned {
		destInfo, err := os.Lstat(p.destFullPath)
		if err == nil && destInfo.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(p.destFullPath)
			if target == p.relTarget {
				recordInstalledLink(state, p.dest, p.relTarget)
				continue
			}
		}

		// Track directories to create
		dirToCreate := filepath.Dir(p.destFullPath)
		var missingDirs []string
		curr := dirToCreate
		for curr != ws.CodeRoot && curr != "." && curr != "/" {
			if _, err := os.Lstat(curr); errors.Is(err, os.ErrNotExist) {
				missingDirs = append([]string{curr}, missingDirs...)
				curr = filepath.Dir(curr)
			} else {
				break
			}
		}

		if err := os.MkdirAll(dirToCreate, 0755); err != nil {
			rollback()
			return fmt.Errorf("creating parent directory %s: %w", dirToCreate, err)
		}
		createdDirs = append(createdDirs, missingDirs...)

		if err := os.Symlink(p.relTarget, p.destFullPath); err != nil {
			rollback()
			return fmt.Errorf("creating symlink %s -> %s: %w", p.destFullPath, p.relTarget, err)
		}
		createdSymlinks = append(createdSymlinks, p.destFullPath)

		recordInstalledLink(state, p.dest, p.relTarget)
		appliedCount++
	}

	if err := WriteLocalState(ws.DocsDir, state); err != nil {
		rollback()
		return fmt.Errorf("saving installed link state: %w", err)
	}

	fmt.Fprintf(out, "Successfully applied %d link(s).\n", appliedCount)
	return nil
}

// Unlink removes a locally owned symlink and removes its declaration from .devdocs.json.
// Preserves the source document and retains the code exclusion to prevent accidental exposure.
func Unlink(ctx context.Context, destination string, out, errOut io.Writer) error {
	if err := CheckGitVersion(ctx); err != nil {
		return err
	}

	trimmedDest := strings.TrimSpace(destination)
	if err := ValidateDestinationPath(trimmedDest); err != nil {
		return err
	}
	destClean := filepath.ToSlash(filepath.Clean(trimmedDest))

	ws, err := DiscoverWorkspace(ctx, "")
	if err != nil {
		return err
	}

	// Lock workspace for mutation before reading mutable state
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

	meta, err := ReadMetadata(ws.DocsDir)
	if err != nil {
		return fmt.Errorf("reading workspace metadata: %w", err)
	}
	state, err := ReadLocalState(ws.DocsDir)
	if err != nil {
		return fmt.Errorf("reading workspace local state: %w", err)
	}

	// Validate workspace pins and isolation
	if err := ValidateWorkspacePins(ctx, ws, meta, state); err != nil {
		return err
	}

	// Check if locally owned
	installedIdx := -1
	for i, inst := range state.InstalledLinks {
		if inst.Destination == destClean {
			installedIdx = i
			break
		}
	}
	if installedIdx < 0 {
		return fmt.Errorf("path %q is not a locally owned devdocs link", destClean)
	}
	expectedTarget := state.InstalledLinks[installedIdx].Target

	destFullPath := filepath.Join(ws.CodeRoot, destClean)
	destInfo, err := os.Lstat(destFullPath)
	if err == nil {
		if destInfo.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("destination %q has been replaced by a regular file or directory; refusing to remove", destClean)
		}
		target, err := os.Readlink(destFullPath)
		if err != nil {
			return fmt.Errorf("reading symlink %s: %w", destFullPath, err)
		}
		if target != expectedTarget {
			return fmt.Errorf("destination %q points to %q (expected %q); refusing to remove modified link", destClean, target, expectedTarget)
		}
		if err := os.Remove(destFullPath); err != nil {
			return fmt.Errorf("removing symlink %s: %w", destFullPath, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspecting destination %s: %w", destFullPath, err)
	}

	// Remove declaration from .devdocs.json if present
	var remaining []LinkDeclaration
	for _, l := range meta.Links {
		if l.Destination != destClean {
			remaining = append(remaining, l)
		}
	}
	if len(remaining) != len(meta.Links) {
		meta.Links = remaining
		if err := WriteMetadata(ws.DocsDir, meta); err != nil {
			return fmt.Errorf("updating .devdocs.json: %w", err)
		}
	}

	// Remove from local state ownership
	state.InstalledLinks = append(state.InstalledLinks[:installedIdx], state.InstalledLinks[installedIdx+1:]...)
	if err := WriteLocalState(ws.DocsDir, state); err != nil {
		return fmt.Errorf("updating local state: %w", err)
	}

	fmt.Fprintf(out, "Unlinked %s.\nNotice: local Git exclusion for %q was retained to prevent accidental exposure.\n", destClean, destClean)
	return nil
}

func isInstalled(state *LocalState, dest, target string) bool {
	for _, inst := range state.InstalledLinks {
		if inst.Destination == dest && inst.Target == target {
			return true
		}
	}
	return false
}

func recordInstalledLink(state *LocalState, dest, target string) {
	for i, inst := range state.InstalledLinks {
		if inst.Destination == dest {
			state.InstalledLinks[i].Target = target
			return
		}
	}
	state.InstalledLinks = append(state.InstalledLinks, InstalledLink{
		Destination: dest,
		Target:      target,
	})
}
