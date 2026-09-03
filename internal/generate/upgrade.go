package generate

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const scaffoldBaselineName = ".gobackend-scaffold.json"

// Kept separate from the generated-output manifest: scaffold files are editable,
// and resource generation must never adopt those edits as an upstream baseline.
type scaffoldBaseline struct {
	GeneratedBy string            `json:"generated_by"`
	Version     int               `json:"version"`
	Module      string            `json:"module"`
	Selection   ProjectOptions    `json:"selection"`
	Files       map[string]string `json:"files"`
}

type UpgradeOptions struct {
	Apply    bool
	Baseline string
	Keep     []string
}

type UpgradeChange struct {
	Path   string `json:"path"`
	Action string `json:"action"`
}

type UpgradeReport struct {
	Directory string          `json:"directory,omitempty"`
	Applied   bool            `json:"applied"`
	Changes   []UpgradeChange `json:"changes"`
}

func scaffoldPaths(opts ProjectOptions) (map[string]bool, error) {
	files, err := selectedScaffoldFiles(opts)
	if err != nil {
		return nil, err
	}
	names := map[string]bool{"go.sum": true}
	for _, file := range files {
		names[file.Output] = true
	}
	return names, nil
}

func captureScaffoldBaseline(root, module string, opts ProjectOptions) (scaffoldBaseline, error) {
	names, err := scaffoldPaths(opts)
	if err != nil {
		return scaffoldBaseline{}, err
	}
	dir, err := os.OpenRoot(root)
	if err != nil {
		return scaffoldBaseline{}, err
	}
	defer dir.Close()
	baseline := scaffoldBaseline{GeneratedBy: "gobackend", Version: 1, Module: module, Selection: opts.normalized(), Files: map[string]string{}}
	for name := range names {
		file, err := readUpgradeFile(dir, name)
		if err != nil {
			return scaffoldBaseline{}, err
		}
		if file.exists {
			baseline.Files[name] = generatedDigest(file.data)
		}
	}
	return baseline, nil
}

func saveScaffoldBaseline(root, module string, opts ProjectOptions) error {
	baseline, err := captureScaffoldBaseline(root, module, opts)
	if err != nil {
		return err
	}
	return writeScaffoldBaseline(root, baseline)
}

func writeScaffoldBaseline(root string, baseline scaffoldBaseline) error {
	data, err := marshalJSON(baseline)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(root, ".gobackend-baseline-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Chmod(0o644); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	dir, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer dir.Close()
	return replaceFile(dir, filepath.Base(file.Name()), scaffoldBaselineName)
}

// Resource addition may change the module graph. Advance only module files that
// still matched the previous baseline BEFORE tidy, never user-edited modules.
func trackedModuleBaseline(root string) (scaffoldBaseline, []string, error) {
	dir, err := os.OpenRoot(root)
	if err != nil {
		return scaffoldBaseline{}, nil, err
	}
	defer dir.Close()
	file, err := readUpgradeFile(dir, scaffoldBaselineName)
	if err != nil || !file.exists {
		return scaffoldBaseline{}, nil, err
	}
	module, err := readModule(root)
	if err != nil {
		return scaffoldBaseline{}, nil, err
	}
	opts, err := loadProjectOptions(root)
	if err != nil {
		return scaffoldBaseline{}, nil, err
	}
	baseline, err := parseScaffoldBaseline(file.data, module, opts)
	if err != nil {
		return baseline, nil, err
	}
	var tracked []string
	for _, name := range []string{"go.mod", "go.sum"} {
		current, err := readUpgradeFile(dir, name)
		if err != nil {
			return baseline, nil, err
		}
		if current.exists && generatedDigest(current.data) == baseline.Files[name] {
			tracked = append(tracked, name)
		}
	}
	return baseline, tracked, nil
}

func parseScaffoldBaseline(data []byte, module string, opts ProjectOptions) (scaffoldBaseline, error) {
	var baseline scaffoldBaseline
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&baseline); err != nil {
		return baseline, fmt.Errorf("decode scaffold baseline: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return baseline, errors.New("scaffold baseline contains trailing data")
	}
	if baseline.GeneratedBy != "gobackend" || baseline.Version != 1 || baseline.Module != module || baseline.Selection.normalized() != opts.normalized() || baseline.Files == nil {
		return baseline, errors.New("scaffold baseline does not match this module/provider selection")
	}
	names, err := scaffoldPaths(opts)
	if err != nil {
		return baseline, err
	}
	for name, digest := range baseline.Files {
		if !names[name] {
			return baseline, fmt.Errorf("scaffold baseline claims unsupported path %q", name)
		}
		if _, err := hex.DecodeString(digest); err != nil || len(digest) != 64 {
			return baseline, fmt.Errorf("invalid scaffold digest for %s", name)
		}
	}
	return baseline, nil
}

type upgradeFile struct {
	data   []byte
	mode   fs.FileMode
	exists bool
}

// Reject even in-root symlinks: a source-looking link must not expose .env or
// redirect an upgrade. os.Root also confines access during path replacement races.
func readUpgradeFile(root *os.Root, name string) (upgradeFile, error) {
	if !fs.ValidPath(name) || strings.ContainsAny(name, "\\:") {
		return upgradeFile{}, fmt.Errorf("unsafe upgrade path %q", name)
	}
	parts := strings.Split(name, "/")
	for i := range parts {
		info, err := root.Lstat(filepath.FromSlash(strings.Join(parts[:i+1], "/")))
		if errors.Is(err, os.ErrNotExist) {
			return upgradeFile{}, nil
		}
		if err != nil {
			return upgradeFile{}, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return upgradeFile{}, fmt.Errorf("upgrade refuses symlink %s", name)
		}
		if i == len(parts)-1 {
			if !info.Mode().IsRegular() {
				return upgradeFile{}, fmt.Errorf("upgrade target is not a regular file: %s", name)
			}
			data, err := root.ReadFile(filepath.FromSlash(name))
			return upgradeFile{data: data, mode: info.Mode().Perm(), exists: true}, err
		}
		if !info.IsDir() {
			return upgradeFile{}, fmt.Errorf("upgrade parent is not a directory: %s", name)
		}
	}
	return upgradeFile{}, nil
}

func sameUpgradeFile(a, b upgradeFile) bool {
	return a.exists == b.exists && (!a.exists || (a.mode == b.mode && bytes.Equal(a.data, b.data)))
}

// Upgrade defaults to a preview. It never modifies .env, resource definitions,
// migrations, or files outside the selected scaffold and generated manifests.
func (g Generator) Upgrade(ctx context.Context, root string, options UpgradeOptions) (report UpgradeReport, err error) {
	// The shared lock helper creates .gobackend; do not let a link redirect that write.
	if info, statErr := os.Lstat(filepath.Join(root, ".gobackend")); statErr == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return report, errors.New("upgrade requires a real .gobackend directory, not a symlink")
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return report, statErr
	}
	if info, statErr := os.Lstat(filepath.Join(root, ".gobackend", projectLockName)); statErr == nil && !info.Mode().IsRegular() {
		return report, errors.New("upgrade requires a regular lock file, not a symlink")
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return report, statErr
	}
	root, unlock, err := lockProject(ctx, root)
	if err != nil {
		return report, err
	}
	defer func() { err = errors.Join(err, unlock()) }()
	dir, err := os.OpenRoot(root)
	if err != nil {
		return report, err
	}
	defer dir.Close()
	// Validate these paths before existing metadata helpers read them.
	for _, name := range []string{"go.mod", projectMetadataName, generatedManifestName, scaffoldBaselineName} {
		if _, err := readUpgradeFile(dir, name); err != nil {
			return report, err
		}
	}
	module, err := readModule(root)
	if err != nil {
		return report, err
	}
	opts, err := loadProjectOptions(root)
	if err != nil {
		return report, err
	}
	scaffoldNames, err := scaffoldPaths(opts)
	if err != nil {
		return report, err
	}
	keep := map[string]bool{}
	for _, name := range options.Keep {
		if !scaffoldNames[name] {
			return report, fmt.Errorf("--keep is only allowed for selected scaffold files: %s", name)
		}
		keep[name] = true
	}
	baselineFile, err := readUpgradeFile(dir, scaffoldBaselineName)
	if err != nil {
		return report, err
	}
	var baseline scaffoldBaseline
	if baselineFile.exists {
		if options.Baseline != "" {
			return report, errors.New("project already has a scaffold baseline; do not replace it with --baseline")
		}
		baseline, err = parseScaffoldBaseline(baselineFile.data, module, opts)
	} else if options.Baseline != "" {
		baselineRoot, resolveErr := filepath.EvalSymlinks(options.Baseline)
		if resolveErr != nil {
			return report, resolveErr
		}
		baselineRoot, err = filepath.Abs(baselineRoot)
		if err != nil {
			return report, err
		}
		if baselineRoot == root {
			return report, errors.New("baseline must be a separate pristine old project, not the current project")
		}
		baselineDir, openErr := os.OpenRoot(baselineRoot)
		if openErr != nil {
			return report, openErr
		}
		for _, name := range []string{"go.mod", projectMetadataName, generatedManifestName} {
			if _, err := readUpgradeFile(baselineDir, name); err != nil {
				baselineDir.Close()
				return report, err
			}
		}
		baselineDir.Close()
		oldModule, moduleErr := readModule(baselineRoot)
		oldOpts, optsErr := loadProjectOptions(baselineRoot)
		if moduleErr != nil || optsErr != nil || oldModule != module || oldOpts.normalized() != opts.normalized() {
			return report, errors.New("explicit baseline must have the same module/provider selection")
		}
		baseline, err = captureScaffoldBaseline(baselineRoot, module, opts)
	} else {
		return report, errors.New("no scaffold baseline; provide --baseline with a verified pristine project from the original generator, same module/providers/resources")
	}
	if err != nil {
		return report, err
	}
	oldGenerated, exists, err := readGeneratedManifestFromRoot(dir)
	if err != nil {
		return report, err
	}
	if !exists {
		return report, errors.New("no generated manifest; run the original generator's generate command before upgrading")
	}
	// Resource files are inputs, not upgrade targets. Reject links before parsing.
	if info, statErr := dir.Lstat("resources"); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return report, errors.New("upgrade refuses symlink resources directory")
	}
	entries, err := os.ReadDir(filepath.Join(root, "resources"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return report, err
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".yaml") || strings.HasSuffix(entry.Name(), ".yml") {
			if _, err := readUpgradeFile(dir, "resources/"+entry.Name()); err != nil {
				return report, err
			}
		}
	}
	resources, err := loadResources(filepath.Join(root, "resources"))
	if err != nil {
		return report, err
	}
	inputNames := map[string]bool{generatedManifestName: true, projectMetadataName: true, scaffoldBaselineName: true}
	for name := range scaffoldNames {
		inputNames[name] = true
	}
	for name := range oldGenerated.Files {
		inputNames[name] = true
	}
	generated, err := renderGenerated(module, resources, opts)
	if err != nil {
		return report, err
	}
	for name := range generated {
		inputNames[name] = true
	}
	for _, resource := range resources {
		inputNames["internal/resources/"+resource.Package+"/gormgen/query_gen.go"] = true
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".yaml") || strings.HasSuffix(entry.Name(), ".yml") {
			inputNames["resources/"+entry.Name()] = true
		}
	}
	observed := map[string]upgradeFile{}
	for name := range inputNames {
		file, err := readUpgradeFile(dir, name)
		if err != nil {
			return report, err
		}
		observed[name] = file
	}
	stage, err := os.MkdirTemp(filepath.Join(root, ".gobackend"), "upgrade-")
	if err != nil {
		return report, err
	}
	// Retain candidate files and backups for review/recovery, including on failure.
	report.Directory = stage
	candidate := filepath.Join(stage, "candidate")
	if err := g.New(ctx, candidate, module, opts); err != nil {
		return report, err
	}
	if g.Version == "" {
		g.Version = CurrentVersion
	}
	desired, err := renderDesired(ctx, candidate, module, g.Version, resources, opts)
	if err != nil {
		return report, err
	}
	if err := installGenerated(candidate, desired); err != nil {
		return report, err
	}
	if err := tidyModule(ctx, candidate); err != nil {
		return report, err
	}
	if err := saveScaffoldBaseline(candidate, module, opts); err != nil {
		return report, err
	}
	candidateDir, err := os.OpenRoot(candidate)
	if err != nil {
		return report, err
	}
	defer candidateDir.Close()
	newBaselineFile, err := readUpgradeFile(candidateDir, scaffoldBaselineName)
	if err != nil {
		return report, err
	}
	newBaseline, err := parseScaffoldBaseline(newBaselineFile.data, module, opts)
	if err != nil {
		return report, err
	}
	names := map[string]bool{generatedManifestName: true, scaffoldBaselineName: true}
	for name := range newBaseline.Files {
		names[name] = true
	}
	for name := range oldGenerated.Files {
		names[name] = true
	}
	for name := range desired {
		names[name] = true
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	slices.Sort(ordered)
	before, after := observed, map[string]upgradeFile{}
	conflicts := false
	for _, name := range ordered {
		current, err := readUpgradeFile(dir, name)
		if err != nil {
			return report, err
		}
		if expected, ok := observed[name]; ok && !sameUpgradeFile(current, expected) {
			return report, fmt.Errorf("file changed during upgrade: %s", name)
		}
		next, err := readUpgradeFile(candidateDir, name)
		if err != nil {
			return report, err
		}
		if current.exists {
			next.mode = current.mode
		}
		before[name], after[name] = current, next
		if sameUpgradeFile(current, next) {
			continue
		}
		if keep[name] {
			after[name] = current
			report.Changes = append(report.Changes, UpgradeChange{name, "keep"})
			continue
		}
		oldDigest, owned := oldGenerated.Files[name]
		if scaffoldNames[name] {
			oldDigest, owned = baseline.Files[name]
			// Preserve user edits if upstream did not change this scaffold file.
			if owned && next.exists && oldDigest == generatedDigest(next.data) {
				after[name] = current
				continue
			}
		}
		if name == generatedManifestName || name == scaffoldBaselineName {
			owned, oldDigest = current.exists, generatedDigest(current.data)
		}
		action := "update"
		switch {
		case !current.exists && !owned:
			action = "add"
		case !current.exists || !owned || generatedDigest(current.data) != oldDigest:
			action, conflicts = "conflict", true
		case !next.exists:
			action = "remove"
		}
		report.Changes = append(report.Changes, UpgradeChange{name, action})
	}
	plan, err := marshalJSON(report)
	if err != nil {
		return report, err
	}
	if err := os.WriteFile(filepath.Join(stage, "plan.json"), plan, 0o600); err != nil {
		return report, err
	}
	if conflicts {
		return report, errors.New("upgrade has conflicts; review candidate files, merge manually, then explicitly --keep each resolved scaffold path")
	}
	if !options.Apply {
		return report, nil
	}
	latestResources, err := loadResources(filepath.Join(root, "resources"))
	if err != nil {
		return report, err
	}
	inputSchema, err := marshalJSON(resources)
	if err != nil {
		return report, err
	}
	latestSchema, err := marshalJSON(latestResources)
	if err != nil {
		return report, err
	}
	if !bytes.Equal(inputSchema, latestSchema) {
		return report, errors.New("resource definitions changed during upgrade")
	}
	// Prepare all backups before replacing anything. Journals and originals stay
	// available if the process/machine dies; rerunning accepts already-updated files.
	stageRelative := filepath.ToSlash(filepath.Join(".gobackend", filepath.Base(stage)))
	var changes []string
	for _, change := range report.Changes {
		if change.Action == "keep" {
			continue
		}
		changes = append(changes, change.Path)
		for _, pair := range []struct {
			subdir string
			file   upgradeFile
		}{{"before", before[change.Path]}, {"next", after[change.Path]}} {
			if !pair.file.exists {
				continue
			}
			target := filepath.Join(stage, pair.subdir, filepath.FromSlash(change.Path))
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return report, err
			}
			if err := os.WriteFile(target, pair.file.data, pair.file.mode); err != nil {
				return report, err
			}
		}
	}
	// Install the ownership checkpoints last, after all content changes.
	slices.SortFunc(changes, func(a, b string) int {
		checkpoint := func(name string) bool { return name == generatedManifestName || name == scaffoldBaselineName }
		if checkpoint(a) != checkpoint(b) {
			if checkpoint(a) {
				return 1
			}
			return -1
		}
		return strings.Compare(a, b)
	})
	if err := applyUpgrade(ctx, dir, stageRelative, changes, before, after, replaceFile); err != nil {
		return report, err
	}
	report.Applied = true
	completed, err := marshalJSON(report)
	if err != nil {
		return report, err
	}
	if err := os.WriteFile(filepath.Join(stage, "plan.json"), completed, 0o600); err != nil {
		return report, fmt.Errorf("upgrade applied but recovery log update failed: %w", err)
	}
	return report, nil
}

func applyUpgrade(ctx context.Context, root *os.Root, stage string, names []string, before, after map[string]upgradeFile, replace func(*os.Root, string, string) error) error {
	// Detect edits made while rendering/downloading, including preserved files.
	for name, expected := range before {
		current, err := readUpgradeFile(root, name)
		if err != nil {
			return err
		}
		if !sameUpgradeFile(current, expected) {
			return fmt.Errorf("file changed during upgrade: %s", name)
		}
	}
	var applied []string
	rollback := func(cause error) error {
		for _, name := range slices.Backward(applied) {
			current, err := readUpgradeFile(root, name)
			if err != nil || !sameUpgradeFile(current, after[name]) {
				cause = errors.Join(cause, fmt.Errorf("cannot roll back concurrently modified %s", name))
				continue
			}
			original := before[name]
			if !original.exists {
				cause = errors.Join(cause, root.Remove(filepath.FromSlash(name)))
				continue
			}
			tmp := filepath.FromSlash(stage + "/next/" + name)
			if err := root.MkdirAll(filepath.Dir(tmp), 0o700); err != nil {
				cause = errors.Join(cause, err)
				continue
			}
			if err := root.WriteFile(tmp, original.data, original.mode); err != nil {
				cause = errors.Join(cause, err)
				continue
			}
			cause = errors.Join(cause, replaceFile(root, tmp, filepath.FromSlash(name)))
		}
		return cause
	}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return rollback(err)
		}
		current, err := readUpgradeFile(root, name)
		if err != nil {
			return rollback(err)
		}
		if !sameUpgradeFile(current, before[name]) {
			return rollback(fmt.Errorf("file changed during upgrade: %s", name))
		}
		native := filepath.FromSlash(name)
		if after[name].exists {
			if err := root.MkdirAll(filepath.Dir(native), 0o755); err != nil {
				return rollback(err)
			}
			err = replace(root, filepath.FromSlash(stage+"/next/"+name), native)
		} else {
			err = root.Remove(native)
		}
		if err != nil {
			return rollback(fmt.Errorf("upgrade %s: %w", name, err))
		}
		applied = append(applied, name)
	}
	return nil
}
