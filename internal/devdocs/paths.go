package devdocs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ValidateSourcePath validates a source path relative to devdocs/.
// Raw path components are checked before normalization to ensure traversals like unused/../AGENTS.md are rejected.
func ValidateSourcePath(rel string) error {
	trimmed := strings.TrimSpace(rel)
	if trimmed == "" || trimmed == "." {
		return fmt.Errorf("source path cannot be empty or '.'")
	}

	slashPath := filepath.ToSlash(trimmed)
	if strings.HasPrefix(slashPath, "/") || strings.HasPrefix(slashPath, "\\") {
		return fmt.Errorf("source path %q must be relative, not absolute", rel)
	}

	parts := strings.Split(slashPath, "/")
	for _, part := range parts {
		if part == "" {
			return fmt.Errorf("source path %q cannot contain empty path components", rel)
		}
		if part == ".." {
			return fmt.Errorf("source path %q cannot contain '..' traversal", rel)
		}
		if part == "." {
			return fmt.Errorf("source path %q cannot contain '.' components", rel)
		}
		if strings.EqualFold(part, ".git") {
			return fmt.Errorf("source path %q cannot target or enter .git", rel)
		}
	}

	cleaned := filepath.Clean(slashPath)
	if cleaned == ".devdocs.json" || strings.HasPrefix(cleaned, ".devdocs.json/") {
		return fmt.Errorf("source path %q cannot target internal metadata .devdocs.json", rel)
	}

	return nil
}

// ValidateDestinationPath validates a destination path relative to the code checkout root.
// Raw path components are checked before normalization to prevent hidden traversals.
func ValidateDestinationPath(rel string) error {
	trimmed := strings.TrimSpace(rel)
	if trimmed == "" || trimmed == "." {
		return fmt.Errorf("destination path cannot be empty or '.'")
	}

	slashPath := filepath.ToSlash(trimmed)
	if strings.HasPrefix(slashPath, "/") || strings.HasPrefix(slashPath, "\\") {
		return fmt.Errorf("destination path %q must be relative, not absolute", rel)
	}

	parts := strings.Split(slashPath, "/")
	for _, part := range parts {
		if part == "" {
			return fmt.Errorf("destination path %q cannot contain empty path components", rel)
		}
		if part == ".." {
			return fmt.Errorf("destination path %q cannot contain '..' traversal", rel)
		}
		if part == "." {
			return fmt.Errorf("destination path %q cannot contain '.' components", rel)
		}
		if strings.EqualFold(part, ".git") {
			return fmt.Errorf("destination path %q cannot target or enter .git", rel)
		}
	}

	cleaned := filepath.Clean(slashPath)
	if cleaned == "devdocs" || strings.HasPrefix(cleaned, "devdocs/") {
		return fmt.Errorf("destination path %q cannot target or enter devdocs/", rel)
	}

	return nil
}

// CheckParentSymlinkEscapes verifies that none of the parent directory components of relPath
// under rootDir are symlinks pointing outside rootDir.
func CheckParentSymlinkEscapes(rootDir, relPath string) error {
	canonicalRoot, err := filepath.EvalSymlinks(rootDir)
	if err != nil {
		canonicalRoot = rootDir
	}

	cleaned := filepath.Clean(filepath.ToSlash(relPath))
	parts := strings.Split(filepath.Dir(cleaned), "/")

	current := rootDir
	for _, part := range parts {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				// Parent does not exist yet; newly created parents will be ordinary directories.
				break
			}
			return fmt.Errorf("checking path component %s: %w", current, err)
		}

		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return fmt.Errorf("evaluating symlink %s: %w", current, err)
			}
			rel, err := filepath.Rel(canonicalRoot, resolved)
			if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
				return fmt.Errorf("path component %s is a symlink pointing outside root %s", current, rootDir)
			}
		}
	}

	return nil
}

// CheckSourceSymlinkSafety verifies that sourceFullPath (whether a symlink or directory) does not
// escape docsDir or target reserved Git or Devdocs metadata.
func CheckSourceSymlinkSafety(docsDir, sourceFullPath string) error {
	canonicalDocs, err := filepath.EvalSymlinks(docsDir)
	if err != nil {
		canonicalDocs = docsDir
	}

	info, err := os.Lstat(sourceFullPath)
	if err != nil {
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(sourceFullPath)
		if err != nil {
			return fmt.Errorf("evaluating source symlink %s: %w", sourceFullPath, err)
		}
		rel, err := filepath.Rel(canonicalDocs, resolved)
		if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			return fmt.Errorf("source %q is a symlink pointing outside devdocs: %s", sourceFullPath, resolved)
		}
		relSlash := filepath.ToSlash(rel)
		for _, part := range strings.Split(relSlash, "/") {
			if strings.EqualFold(part, ".git") {
				return fmt.Errorf("source symlink targets internal .git metadata: %s", sourceFullPath)
			}
		}
		if relSlash == ".devdocs.json" || strings.HasPrefix(relSlash, ".devdocs.json/") {
			return fmt.Errorf("source symlink targets internal .devdocs.json metadata: %s", sourceFullPath)
		}
	}

	if info.IsDir() {
		return CheckSourceDirectorySymlinks(docsDir, sourceFullPath)
	}

	return nil
}

// CheckSourceDirectorySymlinks walks sourceDir and verifies that no symlink inside it escapes docsDir
// or targets internal metadata.
func CheckSourceDirectorySymlinks(docsDir, sourceDir string) error {
	canonicalDocs, err := filepath.EvalSymlinks(docsDir)
	if err != nil {
		canonicalDocs = docsDir
	}

	return filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				return fmt.Errorf("evaluating symlink %s: %w", path, err)
			}
			rel, err := filepath.Rel(canonicalDocs, resolved)
			if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
				return fmt.Errorf("source symlink %s points outside devdocs: %s", path, resolved)
			}
			relSlash := filepath.ToSlash(rel)
			for _, part := range strings.Split(relSlash, "/") {
				if strings.EqualFold(part, ".git") {
					return fmt.Errorf("source symlink %s targets internal .git metadata: %s", path, resolved)
				}
			}
			if relSlash == ".devdocs.json" || strings.HasPrefix(relSlash, ".devdocs.json/") {
				return fmt.Errorf("source symlink %s targets internal .devdocs.json metadata: %s", path, resolved)
			}
		}
		return nil
	})
}

// ComputeRelativeLinkTarget computes the relative path target from destPath to sourceFullPath.
func ComputeRelativeLinkTarget(destFullPath, sourceFullPath string) (string, error) {
	destDir := filepath.Dir(destFullPath)
	rel, err := filepath.Rel(destDir, sourceFullPath)
	if err != nil {
		return "", fmt.Errorf("computing relative link target from %s to %s: %w", destDir, sourceFullPath, err)
	}
	return filepath.ToSlash(rel), nil
}

// CheckDestinationOverlaps checks if newDest overlaps with any existing destination.
func CheckDestinationOverlaps(newDest string, existing []string) error {
	newClean := filepath.ToSlash(filepath.Clean(newDest))

	for _, ext := range existing {
		extClean := filepath.ToSlash(filepath.Clean(ext))
		if newClean == extClean {
			return fmt.Errorf("destination %q is already declared", newDest)
		}
		if strings.HasPrefix(newClean, extClean+"/") {
			return fmt.Errorf("destination %q overlaps with parent link destination %q", newDest, ext)
		}
		if strings.HasPrefix(extClean, newClean+"/") {
			return fmt.Errorf("destination %q overlaps with child link destination %q", newDest, ext)
		}
	}

	return nil
}
