package devdocs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	excludeHeader = "# devdocs: managed exclusions begin"
	excludeFooter = "# devdocs: managed exclusions end"
)

// EscapeGitIgnorePattern escapes a literal path for use in a root-anchored .gitignore pattern.
func EscapeGitIgnorePattern(relPath string, isDir bool) string {
	relPath = filepath.ToSlash(filepath.Clean(relPath))
	relPath = strings.TrimPrefix(relPath, "/")

	var b strings.Builder
	b.WriteByte('/')

	for i, r := range relPath {
		switch r {
		case '#', '!':
			if i == 0 {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		case '*', '?', '[', ']':
			b.WriteByte('\\')
			b.WriteRune(r)
		case ' ':
			if i == len(relPath)-1 {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}

	res := b.String()
	if isDir && !strings.HasSuffix(res, "/") {
		res += "/"
	}
	return res
}

// UpdateExclusions updates the managed exclusion block in <commonGitDir>/info/exclude.
// It preserves user entries and prior managed patterns, including sibling-worktree links.
func UpdateExclusions(commonGitDir string, extraDestinations []string) error {
	lock, err := AcquireExcludeLock(commonGitDir)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Unlock() }()

	infoDir := filepath.Join(commonGitDir, "info")
	if info, err := os.Lstat(infoDir); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("Git info path %s must be a real directory", infoDir)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspecting Git info directory: %w", err)
	}
	if err := os.MkdirAll(infoDir, 0755); err != nil {
		return fmt.Errorf("creating info directory %s: %w", infoDir, err)
	}

	excludePath := filepath.Join(infoDir, "exclude")
	if info, err := os.Lstat(excludePath); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Git exclusion file %s must be a regular, non-symlink file", excludePath)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspecting Git exclusion file: %w", err)
	}

	var userLines []string
	managedPatterns := map[string]bool{
		"/devdocs":  true,
		"/devdocs/": true,
	}

	if data, err := os.ReadFile(excludePath); err == nil {
		lines := strings.Split(string(data), "\n")
		inManaged := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == excludeHeader {
				inManaged = true
				continue
			}
			if trimmed == excludeFooter {
				inManaged = false
				continue
			}
			if inManaged {
				if trimmed != "" {
					managedPatterns[line] = true
				}
			} else {
				userLines = append(userLines, line)
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("reading exclude file %s: %w", excludePath, err)
	}

	for _, dest := range extraDestinations {
		dest = strings.TrimSpace(dest)
		if dest == "" || dest == "devdocs" {
			continue
		}
		p1 := EscapeGitIgnorePattern(dest, false)
		p2 := EscapeGitIgnorePattern(dest, true)
		managedPatterns[p1] = true
		managedPatterns[p2] = true
	}

	var newLines []string
	// Append user lines, stripping trailing empty lines
	for len(userLines) > 0 && strings.TrimSpace(userLines[len(userLines)-1]) == "" {
		userLines = userLines[:len(userLines)-1]
	}
	newLines = append(newLines, userLines...)

	if len(newLines) > 0 {
		newLines = append(newLines, "")
	}

	newLines = append(newLines, excludeHeader)
	newLines = append(newLines, "/devdocs")
	newLines = append(newLines, "/devdocs/")

	patterns := make([]string, 0, len(managedPatterns))
	for pat := range managedPatterns {
		if pat != "/devdocs" && pat != "/devdocs/" {
			patterns = append(patterns, pat)
		}
	}
	sort.Strings(patterns)
	newLines = append(newLines, patterns...)
	newLines = append(newLines, excludeFooter, "")

	content := strings.Join(newLines, "\n")
	f, err := os.CreateTemp(infoDir, "exclude.tmp.*")
	if err != nil {
		return fmt.Errorf("creating temp exclude file in %s: %w", infoDir, err)
	}
	tmpFile := f.Name()
	defer func() {
		_ = f.Close()
		_ = os.Remove(tmpFile)
	}()

	if _, err := f.Write([]byte(content)); err != nil {
		return fmt.Errorf("writing temp exclude file %s: %w", tmpFile, err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("syncing temp exclude file %s: %w", tmpFile, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing temp exclude file %s: %w", tmpFile, err)
	}

	if err := os.Rename(tmpFile, excludePath); err != nil {
		return fmt.Errorf("updating exclude file %s: %w", excludePath, err)
	}

	return nil
}

// CheckEffectiveIgnore checks whether relPath is effectively ignored in codeRoot.
// Returns an error if not ignored or if un-ignored by a negation pattern (e.g. !relPath).
func CheckEffectiveIgnore(ctx context.Context, codeRoot, relPath string) error {
	relPath = filepath.Clean(relPath)
	cmd := GitCommand(ctx, codeRoot, "check-ignore", "--no-index", "-v", "-z", "--stdin")
	cmd.Stdin = strings.NewReader(relPath + "\x00")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("path %q is not effectively ignored in the code repository; inspect git check-ignore -v", relPath)
	}
	fields := strings.Split(string(output), "\x00")
	if len(fields) != 5 || fields[2] == "" || strings.HasPrefix(fields[2], "!") {
		return fmt.Errorf("path %q is not effectively ignored; remove overriding project rules before proceeding", relPath)
	}
	return nil
}

// CheckTrackedInCodeRepo checks if relPath or any descendant file is currently tracked in the code repository index.
func CheckTrackedInCodeRepo(ctx context.Context, codeRoot, relPath string) error {
	relPath = filepath.Clean(relPath)
	// Run ls-files --stage -z -- <path>
	out, err := RunGitOutput(ctx, codeRoot, "--literal-pathspecs", "ls-files", "--stage", "-z", "--", relPath)
	if err != nil {
		return fmt.Errorf("checking git index for %q: %w", relPath, err)
	}

	if len(out) > 0 {
		return fmt.Errorf("path %q is already tracked in the code repository index; devdocs will not manage tracked paths", relPath)
	}

	return nil
}
