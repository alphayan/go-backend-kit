package generate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	projectMetadataName          = ".gobackend-project.json"
	projectMetadataOwner         = "gobackend"
	projectMetadataSchemaVersion = 1
)

type projectMetadata struct {
	GeneratedBy      string         `json:"generated_by"`
	SchemaVersion    int            `json:"schema_version"`
	GeneratorVersion string         `json:"generator_version"`
	Selection        ProjectOptions `json:"selection"`
	Fingerprint      string         `json:"fingerprint"`
}

func encodeProjectMetadata(version string, opts ProjectOptions) ([]byte, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	fingerprint, err := opts.Fingerprint()
	if err != nil {
		return nil, err
	}
	if version == "" {
		version = "v0.1.0"
	}
	return marshalJSON(projectMetadata{
		GeneratedBy:      projectMetadataOwner,
		SchemaVersion:    projectMetadataSchemaVersion,
		GeneratorVersion: version,
		Selection:        opts,
		Fingerprint:      fingerprint,
	})
}

func parseProjectMetadata(data []byte) (projectMetadata, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var meta projectMetadata
	if err := decoder.Decode(&meta); err != nil {
		return projectMetadata{}, fmt.Errorf("decode project metadata: %w", err)
	}
	if decoder.More() {
		return projectMetadata{}, errors.New("decode project metadata: unexpected trailing data")
	}
	if meta.GeneratedBy != projectMetadataOwner {
		return projectMetadata{}, fmt.Errorf("project metadata owner is %q, want %q; create a new project to change providers", meta.GeneratedBy, projectMetadataOwner)
	}
	if meta.SchemaVersion != projectMetadataSchemaVersion {
		return projectMetadata{}, fmt.Errorf("project metadata schema_version is %d, want %d; create a new project to change providers", meta.SchemaVersion, projectMetadataSchemaVersion)
	}
	if err := meta.Selection.Validate(); err != nil {
		return projectMetadata{}, fmt.Errorf("project metadata selection: %w; create a new project to change providers", err)
	}
	want, err := meta.Selection.Fingerprint()
	if err != nil {
		return projectMetadata{}, err
	}
	if meta.Fingerprint != want {
		return projectMetadata{}, errors.New("project metadata fingerprint does not match the recorded selection; create a new project to change providers")
	}
	return meta, nil
}

func loadProjectOptions(root string) (ProjectOptions, error) {
	data, err := os.ReadFile(filepath.Join(root, projectMetadataName))
	metaPresent := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return ProjectOptions{}, fmt.Errorf("read project metadata: %w", err)
	}

	claimsMetadata, err := manifestClaimsProjectMetadata(root)
	if err != nil {
		return ProjectOptions{}, err
	}
	if !metaPresent {
		if claimsMetadata {
			return ProjectOptions{}, fmt.Errorf("generated manifest claims %s but the file is missing or invalid; create a new project to change providers", projectMetadataName)
		}
		return LegacyProjectOptions(), nil
	}
	meta, err := parseProjectMetadata(data)
	if err != nil {
		return ProjectOptions{}, err
	}
	return meta.Selection, nil
}

func manifestClaimsProjectMetadata(root string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(root, generatedManifestName))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read generated manifest: %w", err)
	}
	manifest, err := parseGeneratedManifest(data)
	if err != nil {
		return false, err
	}
	_, claimed := manifest.Files[projectMetadataName]
	return claimed, nil
}
