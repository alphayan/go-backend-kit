package generate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEncodeAndParseProjectMetadata(t *testing.T) {
	opts := DefaultProjectOptions()
	data, err := encodeProjectMetadata("v0.2.0", opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"generated_by": "gobackend"`,
		`"schema_version": 1`,
		`"generator_version": "v0.2.0"`,
		`"http": "echo"`,
		`"database": "sqlite"`,
		`"cache": "none"`,
		`"messaging": "none"`,
		`"logging": "slog"`,
		`"auth": "none"`,
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("metadata missing %s:\n%s", want, data)
		}
	}
	if !strings.Contains(string(data), "\"profile\": \"personal\"") {
		t.Fatalf("metadata missing personal profile:\n%s", data)
	}
	meta, err := parseProjectMetadata(data)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Selection != opts {
		t.Fatalf("selection = %+v, want %+v", meta.Selection, opts)
	}
	wantFingerprint, err := opts.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if meta.Fingerprint != wantFingerprint {
		t.Fatalf("fingerprint = %q, want %q", meta.Fingerprint, wantFingerprint)
	}
}

func TestEncodeProjectMetadataDefaultsToCurrentVersion(t *testing.T) {
	data, err := encodeProjectMetadata("", DefaultProjectOptions())
	if err != nil {
		t.Fatal(err)
	}
	meta, err := parseProjectMetadata(data)
	if err != nil {
		t.Fatal(err)
	}
	if meta.GeneratorVersion != CurrentVersion {
		t.Fatalf("generator version = %q, want %q", meta.GeneratorVersion, CurrentVersion)
	}
}

func TestParseProjectMetadataAcceptsLegacySelectionWithoutProfile(t *testing.T) {
	opts := DefaultProjectOptions()
	legacy := projectMetadata{
		GeneratedBy:      projectMetadataOwner,
		SchemaVersion:    projectMetadataSchemaVersion,
		GeneratorVersion: "v0.1.0",
		Selection:        opts,
	}
	legacy.Selection.Profile = ""
	fingerprint, err := legacy.Selection.legacyFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	legacy.Fingerprint = fingerprint
	data, err := marshalJSON(legacy)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseProjectMetadata(data)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Selection.Profile != ProfilePersonal {
		t.Fatalf("legacy profile = %q, want %q", parsed.Selection.Profile, ProfilePersonal)
	}
}

func TestParseProjectMetadataRejectsUnknownFields(t *testing.T) {
	opts := DefaultProjectOptions()
	data, err := encodeProjectMetadata("v0.2.0", opts)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	raw["extra"] = true
	tampered, err := marshalJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseProjectMetadata(tampered); err == nil {
		t.Fatal("unknown metadata field was accepted")
	}
}

func TestParseProjectMetadataRejectsTamperedFingerprint(t *testing.T) {
	opts := DefaultProjectOptions()
	data, err := encodeProjectMetadata("v0.2.0", opts)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	raw["fingerprint"] = strings.Repeat("a", 64)
	tampered, err := marshalJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	_, err = parseProjectMetadata(tampered)
	if err == nil || !strings.Contains(err.Error(), "fingerprint") || !strings.Contains(err.Error(), "create a new project") {
		t.Fatalf("error = %v, want fingerprint tamper message", err)
	}
}

func TestParseProjectMetadataRejectsInvalidSelection(t *testing.T) {
	opts := DefaultProjectOptions()
	data, err := encodeProjectMetadata("v0.2.0", opts)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	selection := raw["selection"].(map[string]any)
	selection["http"] = "chi"
	tampered, err := marshalJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseProjectMetadata(tampered); err == nil || !strings.Contains(err.Error(), `invalid http value "chi"`) {
		t.Fatalf("error = %v, want invalid http value", err)
	}
}

func TestLoadProjectOptionsInfersLegacyWhenMetadataIsAbsent(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/legacy\n\ngo 1.26.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	opts, err := loadProjectOptions(root)
	if err != nil {
		t.Fatal(err)
	}
	if opts != LegacyProjectOptions() {
		t.Fatalf("loadProjectOptions() = %+v, want legacy postgres selection", opts)
	}
}

func TestLoadProjectOptionsFailsClosedWhenManifestClaimsMissingMetadata(t *testing.T) {
	root := t.TempDir()
	manifest := generatedManifest{
		GeneratedBy: generatedManifestOwner,
		Version:     generatedManifestVersion,
		Files: map[string]string{
			projectMetadataName: strings.Repeat("0", 64),
		},
	}
	data, err := marshalJSON(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, generatedManifestName), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadProjectOptions(root); err == nil || !strings.Contains(err.Error(), projectMetadataName) {
		t.Fatalf("loadProjectOptions() error = %v, want missing metadata failure", err)
	}
}
