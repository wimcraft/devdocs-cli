package devdocs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// runDocsGit is a manual Git escape hatch, not a managed synchronization operation.
func runDocsGit(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	ws, err := DiscoverWorkspace(ctx, "")
	if err != nil {
		fmt.Fprintf(errOut, "error: %v\n", err)
		return 1
	}
	docsDir := ws.DocsDir
	// Recovery must also work from inside docs when both Devdocs marker files
	// were removed. Only recognize the conventional immediate nested checkout.
	if !ws.InsideDocsRepo && filepath.Base(ws.CodeRoot) == "devdocs" {
		parent := filepath.Dir(ws.CodeRoot)
		parentRoot, err := RunGitOutput(ctx, parent, "rev-parse", "--show-toplevel")
		if err == nil && filepath.Clean(parentRoot) == parent {
			docsDir = ws.CodeRoot
		}
	}
	gitDir := filepath.Join(docsDir, ".git")
	for _, path := range []string{docsDir, gitDir} {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			fmt.Fprintf(errOut, "error: %s is not a real docs Git checkout; run 'devdocs init', 'devdocs init --adopt', or 'devdocs attach' first\n", docsDir)
			return 1
		}
	}
	actualGitDir, err := RunGitOutput(ctx, docsDir, "rev-parse", "--absolute-git-dir")
	if err != nil || filepath.Clean(actualGitDir) != gitDir {
		fmt.Fprintf(errOut, "error: %s is not the docs Git repository; refusing to fall back to another repository\n", docsDir)
		return 1
	}

	cmd := GitCommand(ctx, docsDir, args...)
	cmd.Stdin = in
	cmd.Stdout = out
	cmd.Stderr = errOut
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			if code := exit.ExitCode(); code >= 0 {
				return code
			}
			if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				return 128 + int(status.Signal())
			}
		}
		fmt.Fprintf(errOut, "error: running Git: %v\n", err)
		return 1
	}
	return 0
}
