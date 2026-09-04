package generate

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func upgradeTestProject(t *testing.T) (string, Generator) {
	t.Helper()
	kit, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	g := Generator{Version: "v0.1.0", DevelopmentReplace: kit}
	root := filepath.Join(t.TempDir(), "api")
	if err := g.New(t.Context(), root, "example.com/upgrade-test", DefaultProjectOptions()); err != nil {
		t.Fatal(err)
	}
	return root, g
}

func TestUpgradePreviewApplyAndKeepUserFiles(t *testing.T) {
	root, g := upgradeTestProject(t)
	if err := g.Add(t.Context(), root, filepath.Join(g.DevelopmentReplace, "examples/product.yaml")); err != nil {
		t.Fatal(err)
	}
	originalModule, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	custom := "package app\n\n// User-owned file must survive upgrades.\n"
	if err := os.WriteFile(filepath.Join(root, "internal/app/custom.go"), []byte(custom), 0o600); err != nil {
		t.Fatal(err)
	}
	// A secret-bearing symlink is outside the upgrade inventory and is untouched.
	secret := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secret, []byte("do not copy this value"), 0o600); err != nil {
		t.Fatal(err)
	}
	linked := os.Symlink(secret, filepath.Join(root, ".env")) == nil
	if !linked {
		if err := os.WriteFile(filepath.Join(root, ".env"), []byte("private fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	g.Version = "v0.2.0"
	preview, err := g.Upgrade(t.Context(), root, UpgradeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Applied || len(preview.Changes) == 0 {
		t.Fatalf("preview = %+v", preview)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "go.mod")); !bytes.Equal(got, originalModule) {
		t.Fatal("preview edited module")
	}
	if _, err := os.Stat(filepath.Join(preview.Directory, "candidate", ".env")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("candidate copied .env")
	}
	report, err := g.Upgrade(t.Context(), root, UpgradeOptions{Apply: true})
	if err != nil || !report.Applied {
		t.Fatalf("apply = %+v, %v", report, err)
	}
	if got, _ := os.ReadFile(filepath.Join(report.Directory, "before", "go.mod")); !bytes.Equal(got, originalModule) {
		t.Fatal("original module was not backed up")
	}
	if got, _ := os.ReadFile(filepath.Join(root, "internal/app/custom.go")); string(got) != custom {
		t.Fatal("custom file changed")
	}
	if linked {
		if got, err := os.Readlink(filepath.Join(root, ".env")); err != nil || got != secret {
			t.Fatal(".env changed")
		}
	} else if got, err := os.ReadFile(filepath.Join(root, ".env")); err != nil || string(got) != "private fixture" {
		t.Fatal(".env changed")
	}
	if err := g.Check(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "go", "test", "-race", "./...")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("upgraded project: %v\n%s", err, out)
	}
	second, err := g.Upgrade(t.Context(), root, UpgradeOptions{Apply: true})
	if err != nil || len(second.Changes) != 0 {
		t.Fatalf("idempotent upgrade = %+v, %v", second, err)
	}
}

func TestUpgradeConflictsAndExplicitResolution(t *testing.T) {
	root, g := upgradeTestProject(t)
	name := "internal/app/app.go"
	newSource, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	oldSource := append(bytes.Clone(newSource), []byte("\n// Previous upstream revision.\n")...)
	if err := os.WriteFile(filepath.Join(root, name), oldSource, 0o644); err != nil {
		t.Fatal(err)
	}
	// Construct a known prior upstream revision, then make a user edit after it.
	if err := saveScaffoldBaseline(root, "example.com/upgrade-test", DefaultProjectOptions()); err != nil {
		t.Fatal(err)
	}
	customized := append(bytes.Clone(oldSource), []byte("// User customization.\n")...)
	if err := os.WriteFile(filepath.Join(root, name), customized, 0o644); err != nil {
		t.Fatal(err)
	}
	g.Version = "v0.2.0"
	report, err := g.Upgrade(t.Context(), root, UpgradeOptions{Apply: true})
	if err == nil || !strings.Contains(err.Error(), "conflicts") || report.Applied {
		t.Fatalf("conflict accepted: %+v, %v", report, err)
	}
	if got, _ := os.ReadFile(filepath.Join(root, name)); !bytes.Equal(got, customized) {
		t.Fatal("conflict overwritten")
	}
	if _, err := os.Stat(filepath.Join(report.Directory, "before")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("conflict partially applied")
	}
	// An operator merges the new upstream and retains their customization.
	merged := append(bytes.Clone(newSource), []byte("\n// User customization.\n")...)
	if err := os.WriteFile(filepath.Join(root, name), merged, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Upgrade(t.Context(), root, UpgradeOptions{Apply: true, Keep: []string{name}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(root, name)); !bytes.Equal(got, merged) {
		t.Fatal("explicit keep overwritten")
	}
	if _, err := g.Upgrade(t.Context(), root, UpgradeOptions{Apply: true}); err != nil {
		t.Fatalf("unchanged upstream did not preserve merged file: %v", err)
	}
	if _, err := g.Upgrade(t.Context(), root, UpgradeOptions{Keep: []string{".env"}}); err == nil {
		t.Fatal("keep accepted private file")
	}
}

func TestUpgradeRequiresExplicitLegacyBaseline(t *testing.T) {
	root, g := upgradeTestProject(t)
	baseline, _ := upgradeTestProject(t)
	if err := os.Remove(filepath.Join(root, scaffoldBaselineName)); err != nil {
		t.Fatal(err)
	}
	g.Version = "v0.2.0"
	if _, err := g.Upgrade(t.Context(), root, UpgradeOptions{}); err == nil || !strings.Contains(err.Error(), "no scaffold baseline") {
		t.Fatalf("missing baseline: %v", err)
	}
	if _, err := g.Upgrade(t.Context(), root, UpgradeOptions{Baseline: root}); err == nil {
		t.Fatal("current project accepted as pristine baseline")
	}
	report, err := g.Upgrade(t.Context(), root, UpgradeOptions{Baseline: baseline, Apply: true})
	if err != nil || !report.Applied {
		t.Fatalf("legacy upgrade = %+v, %v", report, err)
	}
	if err := g.Check(t.Context(), root); err != nil {
		t.Fatal(err)
	}
}

func TestUpgradeSessionProduction(t *testing.T) {
	kit, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	g := Generator{Version: "v0.1.0", DevelopmentReplace: kit}
	opts := DefaultProjectOptions()
	opts.Auth, opts.Database, opts.Profile = AuthSession, DatabasePostgres, ProfileProduction
	root := filepath.Join(t.TempDir(), "session")
	if err := g.New(t.Context(), root, "example.com/session-upgrade", opts); err != nil {
		t.Fatal(err)
	}
	if err := g.Add(t.Context(), root, filepath.Join(kit, "examples/product.yaml")); err != nil {
		t.Fatal(err)
	}
	view := filepath.Join(root, "web/src/App.vue")
	data, err := os.ReadFile(view)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte("\n<!-- User branding. -->\n")...)
	if err := os.WriteFile(view, data, 0o644); err != nil {
		t.Fatal(err)
	}
	g.Version = "v0.2.0"
	if _, err := g.Upgrade(t.Context(), root, UpgradeOptions{Apply: true}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(view); !bytes.Equal(got, data) {
		t.Fatal("unchanged upstream overwrote user branding")
	}
	if err := g.Check(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "go", "test", "-race", "./...")
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("session production upgrade: %v\n%s", err, output)
	}
}

func TestUpgradeRejectsForgedBaselineAndSymlinks(t *testing.T) {
	root, g := upgradeTestProject(t)
	file, err := os.ReadFile(filepath.Join(root, scaffoldBaselineName))
	if err != nil {
		t.Fatal(err)
	}
	base, err := parseScaffoldBaseline(file, "example.com/upgrade-test", DefaultProjectOptions())
	if err != nil {
		t.Fatal(err)
	}
	base.Files[".env"] = strings.Repeat("0", 64)
	if err := writeScaffoldBaseline(root, base); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Upgrade(t.Context(), root, UpgradeOptions{Apply: true}); err == nil {
		t.Fatal("forged baseline accepted")
	}
	if err := os.WriteFile(filepath.Join(root, scaffoldBaselineName), file, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"go.mod", "internal/app/app.go", ".gobackend/generator.lock"} {
		t.Run(path, func(t *testing.T) {
			original, err := os.ReadFile(filepath.Join(root, path))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = os.Remove(filepath.Join(root, path))
				if err := os.WriteFile(filepath.Join(root, path), original, 0o644); err != nil {
					t.Error(err)
				}
			})
			target := filepath.Join(t.TempDir(), "outside")
			if err := os.WriteFile(target, original, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(root, path)); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(root, path)); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			if _, err := g.Upgrade(t.Context(), root, UpgradeOptions{Apply: true}); err == nil {
				t.Fatal("symlink accepted")
			}
			if err := os.Remove(filepath.Join(root, path)); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, path), original, 0o644); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestModuleTidyDoesNotAdoptUserEditedBaseline(t *testing.T) {
	root, g := upgradeTestProject(t)
	state, err := os.ReadFile(filepath.Join(root, scaffoldBaselineName))
	if err != nil {
		t.Fatal(err)
	}
	before, err := parseScaffoldBaseline(state, "example.com/upgrade-test", DefaultProjectOptions())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "go.mod")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte("\n// User-owned module customization.\n")...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := g.Add(t.Context(), root, filepath.Join(g.DevelopmentReplace, "examples/product.yaml")); err != nil {
		t.Fatal(err)
	}
	state, err = os.ReadFile(filepath.Join(root, scaffoldBaselineName))
	if err != nil {
		t.Fatal(err)
	}
	after, err := parseScaffoldBaseline(state, "example.com/upgrade-test", DefaultProjectOptions())
	if err != nil {
		t.Fatal(err)
	}
	if after.Files["go.mod"] != before.Files["go.mod"] {
		t.Fatal("tidy adopted user edits as upstream")
	}
}

func TestApplyUpgradeRollbackAndConcurrentEdits(t *testing.T) {
	for _, operation := range []string{"replace", "remove", "concurrent"} {
		t.Run(operation, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, "stage/next"), 0o700); err != nil {
				t.Fatal(err)
			}
			before := map[string]upgradeFile{"a": {[]byte("old-a"), 0o755, true}, "b": {[]byte("old-b"), 0o644, true}}
			after := map[string]upgradeFile{"a": {[]byte("new-a"), 0o755, true}, "b": {[]byte("new-b"), 0o644, true}}
			if operation == "remove" {
				after["a"] = upgradeFile{}
			}
			for name, file := range before {
				if err := os.WriteFile(filepath.Join(dir, name), file.data, file.mode); err != nil {
					t.Fatal(err)
				}
				info, err := os.Stat(filepath.Join(dir, name))
				if err != nil {
					t.Fatal(err)
				}
				file.mode = info.Mode().Perm()
				before[name] = file
				if after[name].exists {
					next := after[name]
					next.mode = file.mode
					after[name] = next
				}
				if err := os.WriteFile(filepath.Join(dir, "stage/next", name), after[name].data, file.mode); err != nil {
					t.Fatal(err)
				}
			}
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			injected := errors.New("injected write failure")
			err = applyUpgrade(t.Context(), root, "stage", []string{"a", "b"}, before, after, func(r *os.Root, src, dst string) error {
				if dst == "b" {
					if operation == "concurrent" {
						if err := r.WriteFile("a", []byte("new user edit"), 0o755); err != nil {
							t.Fatal(err)
						}
					}
					return injected
				}
				return replaceFile(r, src, dst)
			})
			if !errors.Is(err, injected) {
				t.Fatalf("failure = %v", err)
			}
			got, err := root.ReadFile("a")
			if err != nil {
				t.Fatal(err)
			}
			want := "old-a"
			if operation == "concurrent" {
				want = "new user edit"
			}
			if string(got) != want {
				t.Fatalf("rollback a = %q, want %q", got, want)
			}
			info, err := root.Stat("a")
			if err != nil || info.Mode().Perm() != before["a"].mode {
				t.Fatalf("rollback mode: %v, %v", info, err)
			}
		})
	}
}

func TestUpgradeRemovesRetiredScaffoldFile(t *testing.T) {
	kit, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	g := Generator{Version: "v0.1.0", DevelopmentReplace: kit}
	opts := DefaultProjectOptions()
	opts.Auth = AuthSession
	root := filepath.Join(t.TempDir(), "session")
	if err := g.New(t.Context(), root, "example.com/session-retired", opts); err != nil {
		t.Fatal(err)
	}
	// Model a project generated before .npmrc was retired: the file exists and
	// the upstream baseline records its digest.
	upstream := []byte("engine-strict=true\nsave-exact=true\n")
	recordRetired := func(content []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, ".npmrc"), content, 0o644); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(root, scaffoldBaselineName))
		if err != nil {
			t.Fatal(err)
		}
		base, err := parseScaffoldBaseline(data, "example.com/session-retired", opts)
		if err != nil {
			t.Fatal(err)
		}
		base.Files[".npmrc"] = generatedDigest(upstream)
		if err := writeScaffoldBaseline(root, base); err != nil {
			t.Fatal(err)
		}
	}
	recordRetired(upstream)
	g.Version = "v0.2.0"
	preview, err := g.Upgrade(t.Context(), root, UpgradeOptions{})
	if err != nil || !slices.Contains(preview.Changes, UpgradeChange{".npmrc", "remove"}) {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".npmrc")); err != nil {
		t.Fatal("preview removed the retired file")
	}
	report, err := g.Upgrade(t.Context(), root, UpgradeOptions{Apply: true})
	if err != nil || !report.Applied {
		t.Fatalf("apply = %+v, %v", report, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".npmrc")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retired file survived apply: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(report.Directory, "before", ".npmrc")); err != nil || !bytes.Equal(got, upstream) {
		t.Fatalf("retired file was not backed up: %v", err)
	}
	if err := g.Check(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	// A user-modified retired file is a conflict, resolved only by explicit --keep.
	recordRetired([]byte("registry=https://example.test\n"))
	if _, err := g.Upgrade(t.Context(), root, UpgradeOptions{Apply: true}); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("modified retired file accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".npmrc")); err != nil {
		t.Fatal("conflict removed the modified retired file")
	}
	if _, err := g.Upgrade(t.Context(), root, UpgradeOptions{Apply: true, Keep: []string{".npmrc"}}); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(root, ".npmrc")); err != nil || string(got) != "registry=https://example.test\n" {
		t.Fatalf("kept retired file changed: %v", err)
	}
	// A retired path that no baseline ever recorded is a user file and is left alone.
	if _, err := g.Upgrade(t.Context(), root, UpgradeOptions{Apply: true}); err != nil {
		t.Fatalf("user-owned retired path blocked upgrade: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".npmrc")); err != nil {
		t.Fatal("user-owned file at a retired path was removed")
	}
}
