package devdocs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// WorkspaceLock represents an acquired exclusive non-blocking file lock on a workspace.
type WorkspaceLock struct {
	file *os.File
	path string
}

// Unlock releases the workspace lock and closes the file descriptor.
func (l *WorkspaceLock) Unlock() error {
	if l == nil || l.file == nil {
		return nil
	}
	defer func() {
		_ = l.file.Close()
		l.file = nil
	}()
	return syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
}

// AcquireWorkspaceLock acquires a non-blocking exclusive flock for the given code root.
// The lock file is placed in commonGitDir to ensure linked worktrees share the same lock space.
func AcquireWorkspaceLock(codeRoot, commonGitDir string) (*WorkspaceLock, error) {
	canonicalRoot, err := filepath.EvalSymlinks(codeRoot)
	if err != nil {
		canonicalRoot = codeRoot
	}

	h := sha256.Sum256([]byte(canonicalRoot))
	hashHex := hex.EncodeToString(h[:8]) // 16 hex chars

	lockPath := filepath.Join(commonGitDir, fmt.Sprintf("devdocs-%s.lock", hashHex))

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening workspace lock file %s: %w", lockPath, err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, fmt.Errorf("another devdocs command is currently operating on workspace %s (lock %s held)", codeRoot, lockPath)
		}
		return nil, fmt.Errorf("acquiring workspace lock %s: %w", lockPath, err)
	}

	return &WorkspaceLock{
		file: f,
		path: lockPath,
	}, nil
}

// ExcludeLock represents an exclusive lock held while reading/modifying info/exclude.
type ExcludeLock struct {
	file *os.File
}

// Unlock releases the exclude lock.
func (l *ExcludeLock) Unlock() error {
	if l == nil || l.file == nil {
		return nil
	}
	defer func() {
		_ = l.file.Close()
		l.file = nil
	}()
	return syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
}

// AcquireExcludeLock acquires an exclusive lock for updating shared repository exclusions.
func AcquireExcludeLock(commonGitDir string) (*ExcludeLock, error) {
	lockPath := filepath.Join(commonGitDir, "devdocs-exclude.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening exclude lock file %s: %w", lockPath, err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("acquiring exclude lock %s: %w", lockPath, err)
	}

	return &ExcludeLock{file: f}, nil
}
