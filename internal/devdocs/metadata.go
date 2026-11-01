package devdocs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ValidateMetadata performs strict validation on a Metadata object.
func ValidateMetadata(meta *Metadata) error {
	if meta == nil {
		return errors.New("metadata is nil")
	}

	if meta.SchemaVersion != SchemaVersionCurrent {
		return fmt.Errorf("unsupported metadata schema_version %d (expected %d)", meta.SchemaVersion, SchemaVersionCurrent)
	}

	if err := ValidateUUID(meta.ProjectID); err != nil {
		return fmt.Errorf("metadata project_id is invalid: %w", err)
	}

	slug, branchUUID, err := ParseBranchName(meta.Branch)
	if err != nil {
		return fmt.Errorf("metadata branch is invalid: %w", err)
	}

	if !strings.EqualFold(branchUUID, meta.ProjectID) {
		return fmt.Errorf("metadata branch UUID %q does not match project_id %q", branchUUID, meta.ProjectID)
	}

	if strings.TrimSpace(meta.Name) == "" {
		return errors.New("metadata name cannot be empty")
	}

	if _, err := time.Parse(time.RFC3339, meta.CreatedAt); err != nil {
		return fmt.Errorf("metadata created_at %q is not valid RFC3339: %w", meta.CreatedAt, err)
	}

	var destinations []string
	for i, l := range meta.Links {
		if err := ValidateSourcePath(l.Source); err != nil {
			return fmt.Errorf("metadata links[%d] source invalid: %w", i, err)
		}
		if err := ValidateDestinationPath(l.Destination); err != nil {
			return fmt.Errorf("metadata links[%d] destination invalid: %w", i, err)
		}
		if err := CheckDestinationOverlaps(l.Destination, destinations); err != nil {
			return fmt.Errorf("metadata links[%d]: %w", i, err)
		}
		destinations = append(destinations, l.Destination)
	}

	_ = slug // slug verified by ParseBranchName
	return nil
}

// ReadMetadata reads and validates .devdocs.json from docsDir.
// Symlinked docs directory or metadata file is strictly rejected.
func ReadMetadata(docsDir string) (*Metadata, error) {
	docsInfo, err := os.Lstat(docsDir)
	if err != nil {
		return nil, fmt.Errorf("inspecting docs directory %s: %w", docsDir, err)
	}
	if docsInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("docs directory %s is a symlink; symlinked workspaces are rejected", docsDir)
	}

	path := filepath.Join(docsDir, ".devdocs.json")
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspecting metadata file %s: %w", path, err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("metadata file %s is a symlink; symlinked metadata is rejected", path)
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("metadata file %s is not a regular file", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading metadata file %s: %w", path, err)
	}

	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing metadata file %s: %w", path, err)
	}

	if err := ValidateMetadata(&meta); err != nil {
		return nil, fmt.Errorf("validating metadata from %s: %w", path, err)
	}

	return &meta, nil
}

// WriteMetadata writes meta to .devdocs.json in docsDir atomically.
// Uses os.CreateTemp with restricted permissions to prevent symlink race vulnerabilities.
func WriteMetadata(docsDir string, meta *Metadata) error {
	if err := ValidateMetadata(meta); err != nil {
		return fmt.Errorf("cannot write invalid metadata: %w", err)
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("serializing metadata: %w", err)
	}
	data = append(data, '\n')

	path := filepath.Join(docsDir, ".devdocs.json")
	f, err := os.CreateTemp(docsDir, ".devdocs.json.tmp.*")
	if err != nil {
		return fmt.Errorf("creating temp metadata file in %s: %w", docsDir, err)
	}
	tmpPath := f.Name()
	defer func() {
		_ = f.Close()
		_ = os.Remove(tmpPath)
	}()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("writing metadata to %s: %w", tmpPath, err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("syncing temp metadata %s: %w", tmpPath, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing temp metadata %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("renaming metadata file %s -> %s: %w", tmpPath, path, err)
	}

	return nil
}

// ValidateLocalState validates the local state structure.
func ValidateLocalState(state *LocalState) error {
	if state == nil {
		return errors.New("local state is nil")
	}
	if err := ValidateUUID(state.ProjectID); err != nil {
		return fmt.Errorf("state project_id invalid: %w", err)
	}
	if _, _, err := ParseBranchName(state.Branch); err != nil {
		return fmt.Errorf("state branch invalid: %w", err)
	}
	if err := ValidateRemoteURL(state.Remote); err != nil {
		return fmt.Errorf("state remote invalid: %w", err)
	}
	return nil
}

// ReadLocalState reads devdocs-state.json from docsDir/.git/devdocs-state.json.
// Symlinked docs directory, .git directory, or state file is strictly rejected.
func ReadLocalState(docsDir string) (*LocalState, error) {
	docsInfo, err := os.Lstat(docsDir)
	if err != nil {
		return nil, fmt.Errorf("inspecting docs directory %s: %w", docsDir, err)
	}
	if docsInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("docs directory %s is a symlink; symlinked workspaces are rejected", docsDir)
	}

	gitDir := filepath.Join(docsDir, ".git")
	gitInfo, err := os.Lstat(gitDir)
	if err != nil {
		return nil, fmt.Errorf("inspecting docs git directory %s: %w", gitDir, err)
	}
	if gitInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("docs git directory %s is a symlink; symlinked git directories are rejected", gitDir)
	}

	path := filepath.Join(gitDir, "devdocs-state.json")
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("reading local state file %s: %w", path, err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("local state file %s is a symlink; symlinked state is rejected", path)
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("local state file %s is not a regular file", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading local state file %s: %w", path, err)
	}

	var state LocalState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing local state file %s: %w", path, err)
	}

	if err := ValidateLocalState(&state); err != nil {
		return nil, fmt.Errorf("validating local state from %s: %w", path, err)
	}

	return &state, nil
}

// WriteLocalState writes state to docsDir/.git/devdocs-state.json atomically.
// Uses os.CreateTemp with restricted permissions to prevent symlink race vulnerabilities.
func WriteLocalState(docsDir string, state *LocalState) error {
	if err := ValidateLocalState(state); err != nil {
		return fmt.Errorf("cannot write invalid local state: %w", err)
	}

	gitDir := filepath.Join(docsDir, ".git")
	path := filepath.Join(gitDir, "devdocs-state.json")
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("serializing local state: %w", err)
	}
	data = append(data, '\n')

	f, err := os.CreateTemp(gitDir, "devdocs-state.json.tmp.*")
	if err != nil {
		return fmt.Errorf("creating temp state file in %s: %w", gitDir, err)
	}
	tmpPath := f.Name()
	defer func() {
		_ = f.Close()
		_ = os.Remove(tmpPath)
	}()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("writing state to %s: %w", tmpPath, err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("syncing temp state %s: %w", tmpPath, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing temp state %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("renaming state file %s -> %s: %w", tmpPath, path, err)
	}

	return nil
}
