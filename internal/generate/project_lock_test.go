package generate

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const projectLockHelperEnv = "GO_BACKEND_KIT_PROJECT_LOCK_HELPER"
const projectLockHelperRootEnv = "GO_BACKEND_KIT_PROJECT_LOCK_HELPER_ROOT"

func TestProjectLockSerializesConcurrentCallers(t *testing.T) {
	root := t.TempDir()
	canonical, unlockFirst, err := lockProject(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	firstLocked := true
	t.Cleanup(func() {
		if firstLocked {
			_ = unlockFirst()
		}
	})

	assertProjectLockWaitCanceled(t, root)

	if err := unlockFirst(); err != nil {
		t.Fatal(err)
	}
	firstLocked = false
	secondCanonical, unlockSecond, err := lockProject(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if secondCanonical != canonical {
		t.Fatalf("canonical roots differ: %q != %q", secondCanonical, canonical)
	}
	if err := unlockSecond(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectLockSerializesAcrossProcesses(t *testing.T) {
	if os.Getenv(projectLockHelperEnv) == "1" {
		runProjectLockHelper(t)
		return
	}

	root := t.TempDir()
	command := exec.Command(os.Args[0], "-test.run=^TestProjectLockSerializesAcrossProcesses$")
	command.Env = projectLockHelperEnvironment(root)
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	t.Cleanup(func() {
		if waited {
			return
		}
		_ = stdin.Close()
		_ = command.Process.Kill()
		_ = command.Wait()
	})

	output := bufio.NewReader(stdout)
	ready, err := output.ReadString('\n')
	if err != nil {
		t.Fatalf("read helper readiness: %v; stderr: %s", err, stderr.String())
	}
	if strings.TrimSpace(ready) != "locked" {
		t.Fatalf("helper readiness = %q, want locked; stderr: %s", ready, stderr.String())
	}
	assertProjectLockWaitCanceled(t, root)

	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	remainder, readErr := io.ReadAll(output)
	waitErr := command.Wait()
	waited = true
	if readErr != nil {
		t.Fatalf("read helper output: %v", readErr)
	}
	if waitErr != nil {
		t.Fatalf("helper failed: %v; stdout: %s; stderr: %s", waitErr, remainder, stderr.String())
	}

	_, unlock, err := lockProject(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectLockHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := lockProject(ctx, t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Fatalf("lockProject() error = %v, want context.Canceled", err)
	}
}

func TestProjectLockRejectsMissingRootWithoutCreatingIt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	if _, _, err := lockProject(t.Context(), root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lockProject() error = %v, want os.ErrNotExist", err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing project root was created: %v", err)
	}
}

func TestProjectLockCanonicalizesSymlinks(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "project")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	canonical, unlock, err := lockProject(t.Context(), alias)
	if err != nil {
		t.Fatal(err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if canonical != want {
		t.Fatalf("canonical root = %q, want %q", canonical, want)
	}
}

func assertProjectLockWaitCanceled(t *testing.T, root string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	canonical, unlock, err := lockProject(ctx, root)
	if unlock != nil {
		_ = unlock()
		t.Fatalf("lockProject() acquired locked root %q", canonical)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lockProject() error = %v, want context.DeadlineExceeded", err)
	}
}

func runProjectLockHelper(t *testing.T) {
	root := os.Getenv(projectLockHelperRootEnv)
	if root == "" {
		t.Fatal("missing helper project root")
	}
	_, unlock, err := lockProject(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("locked")
	if _, err := io.ReadAll(os.Stdin); err != nil {
		t.Fatal(err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
}

func projectLockHelperEnvironment(root string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, projectLockHelperEnv+"=") ||
			strings.HasPrefix(value, projectLockHelperRootEnv+"=") {
			continue
		}
		environment = append(environment, value)
	}
	return append(environment,
		projectLockHelperEnv+"=1",
		projectLockHelperRootEnv+"="+root,
	)
}
