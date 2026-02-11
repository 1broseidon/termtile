package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type SourceKind string

const (
	SourceDefault SourceKind = "default"
	SourceBuiltin SourceKind = "builtin"
	SourceFile    SourceKind = "file"
)

type Source struct {
	Kind   SourceKind
	Name   string // for builtin/default
	File   string
	Line   int
	Column int
}

type LoadResult struct {
	Config      *Config
	Sources     map[string]Source // YAML-path -> last writer source (file only)
	LayoutBases map[string]string // layout name -> builtin base name
	Files       []string          // all loaded files, in load order
}

const (
	projectConfigDirName     = ".termtile"
	projectWorkspaceFileName = "workspace.yaml"
	projectLocalOverrideName = "local.yaml"
	projectSourcePathPrefix  = "project_workspace"
)

func DefaultConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".config", "termtile", "config.yaml"), nil
}

// Load reads the merged configuration from the standard location and returns an
// effective config ready for use by the daemon.
func Load() (*Config, error) {
	res, err := LoadWithSources()
	if err != nil {
		return nil, err
	}
	return res.Config, nil
}

// LoadWithSources loads config and returns file-level sources for introspection.
func LoadWithSources() (*LoadResult, error) {
	path, err := DefaultConfigPath()
	if err != nil {
		return nil, err
	}
	return LoadFromPath(path)
}

// LoadWithProjectSources loads global config plus project-scoped overrides from
// .termtile/workspace.yaml and .termtile/local.yaml under projectRoot.
func LoadWithProjectSources(projectRoot string) (*LoadResult, error) {
	path, err := DefaultConfigPath()
	if err != nil {
		return nil, err
	}
	return LoadFromPathWithProject(path, projectRoot)
}

func LoadFromPath(path string) (*LoadResult, error) {
	return loadFromPath(path, "")
}

// LoadFromPathWithProject loads config from path and merges project-scoped
// overrides from projectRoot/.termtile/workspace.yaml and local.yaml.
func LoadFromPathWithProject(path string, projectRoot string) (*LoadResult, error) {
	return loadFromPath(path, projectRoot)
}

func loadFromPath(path string, projectRoot string) (*LoadResult, error) {
	raw := RawConfig{}
	sources := map[string]Source{}
	var files []string

	if exists, err := pathExists(path); err != nil {
		return nil, err
	} else if exists {
		seen := make(map[string]struct{})
		var stack []string
		globalRaw, globalSources, globalFiles, err := loadRawMerged(path, seen, stack)
		if err != nil {
			return nil, err
		}
		raw = raw.merge(globalRaw)
		for key, src := range globalSources {
			sources[key] = src
		}
		files = append(files, globalFiles...)
	}

	if strings.TrimSpace(projectRoot) != "" {
		projectRaw, projectSources, projectFiles, err := loadRawProjectWorkspaceMerged(projectRoot)
		if err != nil {
			return nil, err
		}
		if projectRaw != nil {
			raw.ProjectWorkspace = projectRaw
			for key, src := range projectSources {
				sources[key] = src
			}
			files = append(files, projectFiles...)
		}
	}

	cfg, layoutBases, err := BuildEffectiveConfig(raw)
	if err != nil {
		return nil, attachSourceContext(err, sources)
	}
	if err := cfg.Validate(); err != nil {
		return nil, attachSourceContext(err, sources)
	}

	return &LoadResult{
		Config:      cfg,
		Sources:     sources,
		LayoutBases: layoutBases,
		Files:       files,
	}, nil
}

func loadRawProjectWorkspaceMerged(projectRoot string) (*RawProjectWorkspaceConfig, map[string]Source, []string, error) {
	root, err := canonicalPath(projectRoot)
	if err != nil {
		return nil, nil, nil, err
	}

	projectDir := filepath.Join(root, projectConfigDirName)
	workspacePath := filepath.Join(projectDir, projectWorkspaceFileName)
	localPath := filepath.Join(projectDir, projectLocalOverrideName)

	type candidate struct {
		path     string
		required bool
	}
	candidates := []candidate{
		{path: workspacePath, required: false},
		{path: localPath, required: false},
	}

	merged := RawProjectWorkspaceConfig{}
	sources := map[string]Source{}
	var files []string
	loaded := false

	for _, candidate := range candidates {
		exists, err := pathExists(candidate.path)
		if err != nil {
			return nil, nil, nil, err
		}
		if !exists {
			if candidate.required {
				return nil, nil, nil, fmt.Errorf("%s: failed to read: file does not exist", candidate.path)
			}
			continue
		}

		raw, fileSources, filePath, err := loadRawProjectWorkspace(candidate.path)
		if err != nil {
			return nil, nil, nil, err
		}
		merged = mergeRawProjectWorkspace(merged, raw)
		for key, src := range fileSources {
			sources[prefixedSourcePath(projectSourcePathPrefix, key)] = src
		}
		files = append(files, filePath)
		loaded = true
	}

	if !loaded {
		return nil, map[string]Source{}, nil, nil
	}

	return &merged, sources, files, nil
}

func loadRawProjectWorkspace(path string) (RawProjectWorkspaceConfig, map[string]Source, string, error) {
	canon, err := canonicalPath(path)
	if err != nil {
		return RawProjectWorkspaceConfig{}, nil, "", err
	}

	data, err := os.ReadFile(canon)
	if err != nil {
		return RawProjectWorkspaceConfig{}, nil, "", fmt.Errorf("%s: failed to read: %w", canon, err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return RawProjectWorkspaceConfig{}, nil, "", fmt.Errorf("%s: failed to parse yaml: %w", canon, err)
	}

	var raw RawProjectWorkspaceConfig
	if err := decodeStrictYAML(data, &raw); err != nil {
		return RawProjectWorkspaceConfig{}, nil, "", fmt.Errorf("%s: %w", canon, err)
	}

	sources := collectSources(&doc, canon)
	return raw, sources, canon, nil
}

type includeRef struct {
	Value  string
	Source Source
}

func loadRawMerged(path string, seen map[string]struct{}, stack []string) (RawConfig, map[string]Source, []string, error) {
	canon, err := canonicalPath(path)
	if err != nil {
		return RawConfig{}, nil, nil, err
	}
	for _, existing := range stack {
		if existing == canon {
			return RawConfig{}, nil, nil, fmt.Errorf("include cycle detected: %s -> %s", strings.Join(stack, " -> "), canon)
		}
	}
	if _, ok := seen[canon]; ok {
		// Already merged elsewhere; but still include for order? Skip to avoid duplicates.
		return RawConfig{}, map[string]Source{}, nil, nil
	}
	seen[canon] = struct{}{}

	data, err := os.ReadFile(canon)
	if err != nil {
		return RawConfig{}, nil, nil, fmt.Errorf("%s: failed to read: %w", canon, err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return RawConfig{}, nil, nil, fmt.Errorf("%s: failed to parse yaml: %w", canon, err)
	}

	var raw RawConfig
	if err := decodeStrictYAML(data, &raw); err != nil {
		return RawConfig{}, nil, nil, fmt.Errorf("%s: %w", canon, err)
	}

	sources := collectSources(&doc, canon)
	refs := collectIncludeRefs(&doc, canon)

	merged := RawConfig{}
	mergedSources := map[string]Source{}
	var files []string

	for _, ref := range refs {
		paths, err := expandInclude(canon, ref.Value)
		if err != nil {
			return RawConfig{}, nil, nil, fmt.Errorf("%s:%d:%d: include %q: %w", ref.Source.File, ref.Source.Line, ref.Source.Column, ref.Value, err)
		}
		for _, incPath := range paths {
			incRaw, incSources, incFiles, err := loadRawMerged(incPath, seen, append(stack, canon))
			if err != nil {
				return RawConfig{}, nil, nil, err
			}
			merged = merged.merge(incRaw)
			for p, src := range incSources {
				mergedSources[p] = src
			}
			files = append(files, incFiles...)
		}
	}

	// Apply this file last (overrides includes).
	merged = merged.merge(raw)
	for p, src := range sources {
		mergedSources[p] = src
	}
	files = append(files, canon)

	return merged, mergedSources, files, nil
}

func decodeStrictYAML(data []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}
	return nil
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve %q: %w", path, err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// Best-effort; still use abs.
		return abs, nil
	}
	return real, nil
}

func expandInclude(baseFile string, include string) ([]string, error) {
	path, err := resolvePathRelativeToFile(baseFile, include)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{path}, nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		files = append(files, filepath.Join(path, name))
	}
	sort.Strings(files)
	return files, nil
}

func resolvePathRelativeToFile(baseFile string, include string) (string, error) {
	if include == "" {
		return "", fmt.Errorf("path is empty")
	}
	if strings.HasPrefix(include, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if include == "~" {
			include = home
		} else if strings.HasPrefix(include, "~/") {
			include = filepath.Join(home, include[2:])
		}
	}
	if filepath.IsAbs(include) {
		return include, nil
	}
	return filepath.Join(filepath.Dir(baseFile), include), nil
}

func prefixedSourcePath(prefix string, path string) string {
	if strings.TrimSpace(path) == "" {
		return prefix
	}
	if prefix == "" {
		return path
	}
	return prefix + "." + path
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func collectSources(doc *yaml.Node, file string) map[string]Source {
	out := make(map[string]Source)
	if doc == nil {
		return out
	}
	node := doc
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		node = node.Content[0]
	}
	collectSourcesRec(node, file, "", out)
	return out
}

func collectSourcesRec(node *yaml.Node, file string, prefix string, out map[string]Source) {
	if node == nil {
		return
	}
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]
			key := keyNode.Value
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			out[path] = Source{
				Kind:   SourceFile,
				File:   file,
				Line:   valNode.Line,
				Column: valNode.Column,
			}
			collectSourcesRec(valNode, file, path, out)
		}
	case yaml.SequenceNode:
		// Track the sequence itself.
		if prefix != "" {
			out[prefix] = Source{
				Kind:   SourceFile,
				File:   file,
				Line:   node.Line,
				Column: node.Column,
			}
		}
	}
}

func collectIncludeRefs(doc *yaml.Node, file string) []includeRef {
	if doc == nil {
		return nil
	}
	node := doc
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]
		if keyNode.Value != "include" {
			continue
		}

		switch valNode.Kind {
		case yaml.ScalarNode:
			return []includeRef{{
				Value: valNode.Value,
				Source: Source{
					Kind:   SourceFile,
					File:   file,
					Line:   valNode.Line,
					Column: valNode.Column,
				},
			}}
		case yaml.SequenceNode:
			refs := make([]includeRef, 0, len(valNode.Content))
			for _, item := range valNode.Content {
				if item.Kind != yaml.ScalarNode {
					continue
				}
				refs = append(refs, includeRef{
					Value: item.Value,
					Source: Source{
						Kind:   SourceFile,
						File:   file,
						Line:   item.Line,
						Column: item.Column,
					},
				})
			}
			return refs
		default:
			return nil
		}
	}
	return nil
}

func attachSourceContext(err error, sources map[string]Source) error {
	verr, ok := err.(*ValidationError)
	if !ok || verr == nil {
		return err
	}
	if verr.Path == "" {
		return err
	}
	if src, ok := sources[verr.Path]; ok {
		verr.Source = src
	}
	return verr
}

func defaultLayoutBases(cfg *Config) map[string]string {
	out := make(map[string]string)
	for name := range cfg.Layouts {
		out[name] = name
	}
	return out
}
