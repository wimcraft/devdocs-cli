package devdocs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// CleanGitEnv returns the current process environment with Git redirection variables removed.
func CleanGitEnv() []string {
	removeVars := map[string]bool{
		"GIT_DIR":                          true,
		"GIT_WORK_TREE":                    true,
		"GIT_INDEX_FILE":                   true,
		"GIT_COMMON_DIR":                   true,
		"GIT_OBJECT_DIRECTORY":             true,
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
	}

	var env []string
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq > 0 {
			key := kv[:eq]
			if removeVars[key] {
				continue
			}
		}
		env = append(env, kv)
	}
	return env
}

// GitCommand creates an exec.Cmd for git with sanitized environment.
func GitCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = CleanGitEnv()
	return cmd
}

// RunGit executes a git command in dir and returns an error with sanitized stderr on failure.
func RunGit(ctx context.Context, dir string, args ...string) error {
	cmd := GitCommand(ctx, dir, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errOutput := strings.TrimSpace(stderr.String())
		errOutput = RedactRemoteURL(errOutput)
		if errOutput != "" {
			return fmt.Errorf("git %s: %w: %s", args[0], err, errOutput)
		}
		return fmt.Errorf("git %s: %w", args[0], err)
	}
	return nil
}

// RunGitOutput executes a git command in dir and returns trimmed stdout as string.
func RunGitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := GitCommand(ctx, dir, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errOutput := strings.TrimSpace(stderr.String())
		errOutput = RedactRemoteURL(errOutput)
		if errOutput != "" {
			return "", fmt.Errorf("git %s: %w: %s", args[0], err, errOutput)
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// RunGitLines executes a git command in dir and returns stdout lines.
func RunGitLines(ctx context.Context, dir string, args ...string) ([]string, error) {
	out, err := RunGitOutput(ctx, dir, args...)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines, nil
}

var gitVersionRegex = regexp.MustCompile(`git version (\d+)\.(\d+)(?:\.(\d+))?`)

// CheckGitVersion verifies that the installed Git version is at least 2.39.0.
func CheckGitVersion(ctx context.Context) error {
	out, err := RunGitOutput(ctx, "", "--version")
	if err != nil {
		return fmt.Errorf("running git --version: %w", err)
	}

	matches := gitVersionRegex.FindStringSubmatch(out)
	if len(matches) < 3 {
		return fmt.Errorf("unable to parse git version from %q", out)
	}

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return fmt.Errorf("parsing git major version: %w", err)
	}
	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return fmt.Errorf("parsing git minor version: %w", err)
	}
	patch := 0
	if len(matches) >= 4 && matches[3] != "" {
		p, err := strconv.Atoi(matches[3])
		if err == nil {
			patch = p
		}
	}

	// Minimum required is 2.39.0
	if major < 2 || (major == 2 && minor < 39) {
		return fmt.Errorf("installed git version %d.%d.%d is older than required minimum 2.39.0", major, minor, patch)
	}

	return nil
}

// CheckAuthorIdentity checks if user.name and user.email are configured.
func CheckAuthorIdentity(ctx context.Context, codeRoot string) (name, email string, isLocal bool, err error) {
	// First check local repository config if codeRoot is valid
	if codeRoot != "" {
		localName, _ := RunGitOutput(ctx, codeRoot, "config", "--local", "user.name")
		localEmail, _ := RunGitOutput(ctx, codeRoot, "config", "--local", "user.email")
		if localName != "" && localEmail != "" {
			return localName, localEmail, true, nil
		}
	}

	// Fallback to effective config (including global/system)
	effectiveName, _ := RunGitOutput(ctx, codeRoot, "config", "user.name")
	effectiveEmail, _ := RunGitOutput(ctx, codeRoot, "config", "user.email")

	if effectiveName == "" || effectiveEmail == "" {
		return "", "", false, errors.New("git author identity is not configured; configure user.name and user.email with 'git config' before initializing devdocs")
	}

	return effectiveName, effectiveEmail, false, nil
}

// DiscoverWorkspace discovers the containing code repository and docs workspace from cwd.
// If cwd is inside devdocs, it resolves the parent containing code repository.
func DiscoverWorkspace(ctx context.Context, cwd string) (*Workspace, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("getting current working directory: %w", err)
		}
	}

	topLevel, err := RunGitOutput(ctx, cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, errors.New("not inside a git repository")
	}

	canonicalTop, err := filepath.EvalSymlinks(topLevel)
	if err != nil {
		canonicalTop = topLevel
	}

	insideDocs := false
	codeRoot := canonicalTop

	// Check if the discovered git repository is actually the docs repository
	if isDocsRepository(canonicalTop) {
		insideDocs = true
		parentDir := filepath.Dir(canonicalTop)
		parentTop, err := RunGitOutput(ctx, parentDir, "rev-parse", "--show-toplevel")
		if err != nil {
			return nil, fmt.Errorf("inside devdocs directory %s, but parent %s is not a git repository", canonicalTop, parentDir)
		}
		canonicalParent, err := filepath.EvalSymlinks(parentTop)
		if err != nil {
			canonicalParent = parentTop
		}
		codeRoot = canonicalParent
	}

	// Discover code repository git dir and common git dir (handles linked worktrees)
	gitDir, err := RunGitOutput(ctx, codeRoot, "rev-parse", "--git-dir")
	if err != nil {
		return nil, fmt.Errorf("resolving git directory for %s: %w", codeRoot, err)
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(codeRoot, gitDir)
	}
	gitDir = filepath.Clean(gitDir)

	commonGitDir, err := RunGitOutput(ctx, codeRoot, "rev-parse", "--git-common-dir")
	if err != nil {
		return nil, fmt.Errorf("resolving common git directory for %s: %w", codeRoot, err)
	}
	if !filepath.IsAbs(commonGitDir) {
		// Note: git rev-parse --git-common-dir can be relative to gitDir or codeRoot
		if _, err := os.Stat(filepath.Join(gitDir, commonGitDir)); err == nil {
			commonGitDir = filepath.Join(gitDir, commonGitDir)
		} else {
			commonGitDir = filepath.Join(codeRoot, commonGitDir)
		}
	}
	commonGitDir = filepath.Clean(commonGitDir)

	docsDir := filepath.Join(codeRoot, "devdocs")

	return &Workspace{
		CodeRoot:       codeRoot,
		DocsDir:        docsDir,
		GitDir:         gitDir,
		CommonGitDir:   commonGitDir,
		InsideDocsRepo: insideDocs,
	}, nil
}

func isDocsRepository(path string) bool {
	if filepath.Base(path) == "devdocs" {
		if _, err := os.Stat(filepath.Join(path, ".devdocs.json")); err == nil {
			return true
		}
	}
	if _, err := os.Stat(filepath.Join(path, ".git", "devdocs-state.json")); err == nil {
		return true
	}
	return false
}

// CheckGitOperationInProgress checks if any Git operation is currently active in the docs repo.
func CheckGitOperationInProgress(ctx context.Context, docsDir string) error {
	gitDir := filepath.Join(docsDir, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		return nil // Not initialized yet
	}

	if _, err := os.Stat(filepath.Join(gitDir, "MERGE_HEAD")); err == nil {
		return fmt.Errorf("a git merge is in progress in %s; resolve conflicts or abort with 'devdocs git merge --abort'", docsDir)
	}

	rebasePaths := []string{
		filepath.Join(gitDir, "rebase-apply"),
		filepath.Join(gitDir, "rebase-merge"),
		filepath.Join(gitDir, "REBASE_HEAD"),
	}
	for _, p := range rebasePaths {
		if _, err := os.Stat(p); err == nil {
			return fmt.Errorf("a git rebase is in progress in %s; finish or abort it with 'devdocs git rebase --abort'", docsDir)
		}
	}

	if _, err := os.Stat(filepath.Join(gitDir, "CHERRY_PICK_HEAD")); err == nil {
		return fmt.Errorf("a git cherry-pick is in progress in %s; finish or abort it with 'devdocs git cherry-pick --abort'", docsDir)
	}

	if _, err := os.Stat(filepath.Join(gitDir, "REVERT_HEAD")); err == nil {
		return fmt.Errorf("a git revert is in progress in %s; finish or abort it with 'devdocs git revert --abort'", docsDir)
	}

	// Check for unmerged index entries
	unmerged, err := RunGitOutput(ctx, docsDir, "ls-files", "-u")
	if err == nil && unmerged != "" {
		return fmt.Errorf("unmerged index entries exist in %s; resolve and commit them before continuing", docsDir)
	}

	return nil
}

// ValidateWorkspacePins verifies that the current docs repository matches its pinned identity,
// remote URL, push URL, fetch refspec, clean Git operation state, and repository isolation.
func ValidateWorkspacePins(ctx context.Context, ws *Workspace, meta *Metadata, state *LocalState) error {
	if meta.ProjectID != state.ProjectID {
		return fmt.Errorf("metadata project_id %q does not match local state %q", meta.ProjectID, state.ProjectID)
	}
	if meta.Branch != state.Branch {
		return fmt.Errorf("metadata branch %q does not match local state %q", meta.Branch, state.Branch)
	}

	// 1. Verify current branch in docs repo matches state.Branch
	currentBranch, err := RunGitOutput(ctx, ws.DocsDir, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return fmt.Errorf("checking docs branch: HEAD is detached or unreadable (expected branch %q)", state.Branch)
	}
	if currentBranch != state.Branch {
		return fmt.Errorf("docs repository is on branch %q, expected pinned branch %q", currentBranch, state.Branch)
	}

	// 2. Verify remote URL
	remoteURL, err := RunGitOutput(ctx, ws.DocsDir, "config", "--get-all", "remote.origin.url")
	if err != nil || remoteURL != state.Remote {
		return fmt.Errorf("docs origin remote URL %q does not match pinned remote %q", RedactRemoteURL(remoteURL), RedactRemoteURL(state.Remote))
	}

	// 3. Verify push URL if set
	pushURL, _ := RunGitOutput(ctx, ws.DocsDir, "config", "--get-all", "remote.origin.pushurl")
	if pushURL != "" && pushURL != state.Remote {
		return fmt.Errorf("docs origin pushurl %q does not match pinned remote %q", RedactRemoteURL(pushURL), RedactRemoteURL(state.Remote))
	}

	// 4. Verify fetch refspec
	fetchRefspec, err := RunGitOutput(ctx, ws.DocsDir, "config", "--get-all", "remote.origin.fetch")
	expectedRefspec := fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", state.Branch, state.Branch)
	altRefspec := fmt.Sprintf("refs/heads/%s:refs/remotes/origin/%s", state.Branch, state.Branch)
	if err != nil || (fetchRefspec != expectedRefspec && fetchRefspec != altRefspec) {
		return fmt.Errorf("docs origin fetch refspec %q does not match expected %q", fetchRefspec, expectedRefspec)
	}

	// 5. Verify no Git operations in progress
	if err := CheckGitOperationInProgress(ctx, ws.DocsDir); err != nil {
		return err
	}

	// 6. Verify docs directory isolation in code repo
	if err := CheckTrackedInCodeRepo(ctx, ws.CodeRoot, "devdocs"); err != nil {
		return err
	}
	if err := CheckEffectiveIgnore(ctx, ws.CodeRoot, "devdocs"); err != nil {
		return err
	}

	// 7. Verify installed links isolation in code repo
	for _, inst := range state.InstalledLinks {
		if err := CheckTrackedInCodeRepo(ctx, ws.CodeRoot, inst.Destination); err != nil {
			return fmt.Errorf("installed link destination %q is tracked in code repository: %w", inst.Destination, err)
		}
		if err := CheckEffectiveIgnore(ctx, ws.CodeRoot, inst.Destination); err != nil {
			return fmt.Errorf("installed link destination %q exclusion is ineffective: %w", inst.Destination, err)
		}
	}

	return nil
}
