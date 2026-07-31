package generate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const generatedManifestName = ".gobackend-generated.json"

const (
	generatedManifestOwner   = "gobackend"
	generatedManifestVersion = 1
)

type generatedManifest struct {
	GeneratedBy string            `json:"generated_by"`
	Version     int               `json:"version"`
	Files       map[string]string `json:"files"`
}

func addGeneratedManifest(desired map[string][]byte) error {
	manifest := generatedManifest{
		GeneratedBy: generatedManifestOwner,
		Version:     generatedManifestVersion,
		Files:       make(map[string]string, len(desired)),
	}
	for name, data := range desired {
		if name == generatedManifestName {
			continue
		}
		if err := validateGeneratedPath(name); err != nil {
			return err
		}
		manifest.Files[name] = generatedDigest(data)
	}
	data, err := marshalJSON(manifest)
	if err != nil {
		return fmt.Errorf("encode generated manifest: %w", err)
	}
	desired[generatedManifestName] = data
	return nil
}

func readGeneratedManifestFromRoot(root *os.Root) (generatedManifest, bool, error) {
	data, err := root.ReadFile(generatedManifestName)
	if errors.Is(err, os.ErrNotExist) {
		return generatedManifest{}, false, nil
	}
	if err != nil {
		return generatedManifest{}, false, fmt.Errorf("read generated manifest: %w", err)
	}
	manifest, err := parseGeneratedManifest(data)
	if err != nil {
		return generatedManifest{}, false, err
	}
	return manifest, true, nil
}

func parseGeneratedManifest(data []byte) (generatedManifest, error) {
	var manifest generatedManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return generatedManifest{}, fmt.Errorf("decode generated manifest: %w", err)
	}
	if manifest.GeneratedBy != generatedManifestOwner {
		return generatedManifest{}, fmt.Errorf("generated manifest owner is %q, want %q", manifest.GeneratedBy, generatedManifestOwner)
	}
	if manifest.Version != generatedManifestVersion {
		return generatedManifest{}, fmt.Errorf("generated manifest version is %d, want %d", manifest.Version, generatedManifestVersion)
	}
	if manifest.Files == nil {
		return generatedManifest{}, errors.New("generated manifest has no files")
	}
	for name, digest := range manifest.Files {
		if err := validateGeneratedPath(name); err != nil {
			return generatedManifest{}, err
		}
		if len(digest) != sha256.Size*2 {
			return generatedManifest{}, fmt.Errorf("generated manifest digest for %s is invalid", name)
		}
		if _, err := hex.DecodeString(digest); err != nil {
			return generatedManifest{}, fmt.Errorf("generated manifest digest for %s is invalid: %w", name, err)
		}
	}
	return manifest, nil
}

func staleGenerated(root string, desired map[string][]byte) ([]string, error) {
	rootDir, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open project root: %w", err)
	}
	defer func() { _ = rootDir.Close() }()
	manifest, exists, err := readGeneratedManifestFromRoot(rootDir)
	if err != nil {
		return nil, err
	}
	if !exists {
		return legacyStaleGenerated(root, desired)
	}

	var stale []string
	for name, digest := range manifest.Files {
		if _, ok := desired[name]; ok {
			continue
		}
		data, err := rootDir.ReadFile(filepath.FromSlash(name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read stale generated file %s: %w", name, err)
		}
		if got := generatedDigest(data); got != digest {
			return nil, fmt.Errorf("modified stale generated file %s; refusing to delete it", name)
		}
		stale = append(stale, name)
	}
	sort.Strings(stale)
	return stale, nil
}

func legacyStaleGenerated(root string, desired map[string][]byte) ([]string, error) {
	resourcesRoot := filepath.Join(root, "internal", "resources")
	packages, err := os.ReadDir(resourcesRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var stale []string
	for _, pkg := range packages {
		if !pkg.IsDir() {
			continue
		}
		packageRoot := filepath.Join(resourcesRoot, pkg.Name())
		entries, err := os.ReadDir(packageRoot)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || (!strings.HasSuffix(entry.Name(), "_gen.go") && !strings.HasSuffix(entry.Name(), "_gen_test.go")) {
				continue
			}
			name := path.Join("internal", "resources", pkg.Name(), entry.Name())
			if _, ok := desired[name]; ok {
				continue
			}
			data, err := os.ReadFile(filepath.Join(packageRoot, entry.Name()))
			if err != nil {
				return nil, err
			}
			if strings.Contains(string(data), generatedMarker) {
				stale = append(stale, name)
			}
		}
	}
	sort.Strings(stale)
	return stale, nil
}

func validateGeneratedPath(name string) error {
	if name == "" || strings.Contains(name, `\`) || name != path.Clean(name) || path.IsAbs(name) || name == "." || strings.HasPrefix(name, "../") {
		return fmt.Errorf("generated manifest path %q is unsafe", name)
	}
	native := filepath.FromSlash(name)
	if filepath.IsAbs(native) {
		return fmt.Errorf("generated manifest path %q is unsafe", name)
	}
	if !isGeneratedOutputPath(name) {
		return fmt.Errorf("generated manifest path %q is outside managed outputs", name)
	}
	return nil
}

func isGeneratedOutputPath(name string) bool {
	switch name {
	case "internal/generated/register_gen.go",
		"tools/gormschema/main_gen.go",
		"openapi/embed_gen.go",
		"openapi/openapi_gen.json":
		return true
	}
	parts := strings.Split(name, "/")
	if len(parts) == 4 && parts[0] == "internal" && parts[1] == "resources" && isResourcePackage(parts[2]) {
		return strings.HasSuffix(parts[3], "_gen.go") || strings.HasSuffix(parts[3], "_gen_test.go")
	}
	return isGORMHelperPath(name)
}

func isGORMHelperPath(name string) bool {
	parts := strings.Split(name, "/")
	return len(parts) == 5 &&
		parts[0] == "internal" &&
		parts[1] == "resources" &&
		isResourcePackage(parts[2]) &&
		parts[3] == "gormgen" &&
		parts[4] == "query_gen.go"
}

func isResourcePackage(name string) bool {
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for i := 1; i < len(name); i++ {
		letter := name[i] >= 'a' && name[i] <= 'z'
		digit := name[i] >= '0' && name[i] <= '9'
		if !letter && !digit {
			return false
		}
	}
	return true
}

func generatedDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
