package devdocs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// adoptionEntries inspects names without following symlinks or reading documents.
func adoptionEntries(ctx context.Context, docsDir string) (map[string]bool, error) {
	entries, err := os.ReadDir(docsDir)
	if err != nil {
		return nil, fmt.Errorf("reading existing docs directory: %w", err)
	}
	names := make(map[string]bool, len(entries))
	for _, entry := range entries {
		name := strings.ToLower(entry.Name())
		if name == ".devdocs.json" {
			return nil, fmt.Errorf("existing metadata %q must be inspected before adoption; refusing to overwrite it", entry.Name())
		}
		names[name] = true
	}
	if err := filepath.WalkDir(docsDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != docsDir && strings.EqualFold(entry.Name(), ".git") {
			return fmt.Errorf("existing Git metadata at %s; adoption cannot take over or embed another repository", path)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	bare, err := RunGitOutput(ctx, docsDir, "rev-parse", "--is-bare-repository")
	if err != nil {
		return nil, fmt.Errorf("checking existing directory for Git metadata: %w", err)
	}
	if bare == "true" {
		return nil, fmt.Errorf("existing directory %s is a bare Git repository; refusing to adopt it", docsDir)
	}
	return names, nil
}

type initialFile struct {
	path string
	info os.FileInfo
}

type initialFiles []initialFile

// create exclusively creates an initialization-owned file, never an existing one.
func (created *initialFiles) create(dir, name string, contents []byte) error {
	path := filepath.Join(dir, name)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return fmt.Errorf("creating initial file %s without replacement: %w", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	*created = append(*created, initialFile{path: path, info: info})
	if _, err := file.Write(contents); err != nil {
		return fmt.Errorf("writing initial file %s: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("syncing initial file %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("closing initial file %s: %w", path, err)
	}
	return nil
}

func (created initialFiles) rollback() {
	for _, entry := range created {
		info, err := os.Lstat(entry.path)
		if err == nil && os.SameFile(info, entry.info) {
			_ = os.Remove(entry.path)
		}
	}
}

// checkInitialIgnores honors the adopted tree's .gitignore as well as global rules.
func checkInitialIgnores(ctx context.Context, worktree, gitDir string, names []string) error {
	cmd := GitCommand(ctx, worktree, "--git-dir="+gitDir, "--work-tree="+worktree,
		"check-ignore", "--no-index", "-v", "-z", "--stdin")
	cmd.Stdin = strings.NewReader(strings.Join(names, "\x00") + "\x00")
	output, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return nil // No matching ignore rules.
		}
		return fmt.Errorf("checking initial-file ignore rules: %w", err)
	}
	fields := strings.Split(string(output), "\x00")
	if len(fields)%4 != 1 {
		return errors.New("unexpected Git ignore diagnostic format")
	}
	var ignored []string
	for i := 0; i+3 < len(fields); i += 4 {
		if !strings.HasPrefix(fields[i+2], "!") {
			ignored = append(ignored, fmt.Sprintf("%q (rule %q in %s)", fields[i+3], fields[i+2], fields[i]))
		}
	}
	if len(ignored) != 0 {
		return fmt.Errorf("required initial files are ignored: %s; inspect git check-ignore -v and update the intended ignore rules before retrying", strings.Join(ignored, "; "))
	}
	return nil
}
