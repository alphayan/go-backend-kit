package generate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

const projectLockName = "generator.lock"
const projectLockRetryDelay = 10 * time.Millisecond

func lockProject(ctx context.Context, root string) (string, func() error, error) {
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", nil, fmt.Errorf("resolve project root: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", nil, fmt.Errorf("resolve project root symlinks: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", nil, fmt.Errorf("stat project root: %w", err)
	}
	if !info.IsDir() {
		return "", nil, fmt.Errorf("project root %q is not a directory", canonical)
	}
	lockDir := filepath.Join(canonical, ".gobackend")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return "", nil, fmt.Errorf("create project lock directory: %w", err)
	}
	lockPath := filepath.Join(lockDir, projectLockName)
	fileLock := flock.New(lockPath)
	locked, err := fileLock.TryLockContext(ctx, projectLockRetryDelay)
	if err != nil {
		return "", nil, fmt.Errorf("lock project: %w", err)
	}
	if !locked {
		return "", nil, fmt.Errorf("lock project: lock unavailable")
	}
	unlock := func() error {
		if err := fileLock.Unlock(); err != nil {
			return fmt.Errorf("unlock project: %w", err)
		}
		return nil
	}
	return canonical, unlock, nil
}
