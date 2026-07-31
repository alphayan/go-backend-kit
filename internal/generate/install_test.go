package generate

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallKeepsOldManifestWhenStaleRemovalFails(t *testing.T) {
	root := t.TempDir()
	staleName := "internal/resources/task/model_gen.go"
	staleData := []byte("// " + generatedMarker + "\n\npackage task\n")
	previous := map[string][]byte{staleName: staleData}
	if err := addGeneratedManifest(previous); err != nil {
		t.Fatal(err)
	}
	for name, data := range previous {
		target := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	oldManifest := bytes.Clone(previous[generatedManifestName])

	desired := map[string][]byte{
		"internal/generated/register_gen.go": []byte("// " + generatedMarker + "\n\npackage generated\n"),
	}
	if err := addGeneratedManifest(desired); err != nil {
		t.Fatal(err)
	}
	removeErr := errors.New("simulated locked stale file")
	err := installGeneratedWithRemove(root, desired, func(*os.Root, string) error {
		return removeErr
	})
	if !errors.Is(err, removeErr) {
		t.Fatalf("installGeneratedWithRemove() error = %v, want %v", err, removeErr)
	}
	gotManifest, err := os.ReadFile(filepath.Join(root, generatedManifestName))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotManifest, oldManifest) {
		t.Fatal("failed install replaced the ownership manifest")
	}

	if err := installGenerated(root, desired); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(staleName))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retry did not remove stale file: %v", err)
	}
}

func TestGeneratedManifestRejectsPlatformSpecificTraversal(t *testing.T) {
	for _, name := range []string{
		`internal/resources/task\..\custom/model_gen.go`,
		"internal/resources/.hidden/model_gen.go",
	} {
		if err := validateGeneratedPath(name); err == nil {
			t.Errorf("validateGeneratedPath(%q) error = nil", name)
		}
	}
}
