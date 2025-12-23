package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrMissingEnv indicates the requested configuration environment variable is blank.
var ErrMissingEnv = errors.New("configuration env missing")

// MergeConfig describes all inputs required to render the merged TF2 tree.
type MergeConfig struct {
	BasePath      string          `json:"basePath"`
	TargetBase    string          `json:"targetBase"`
	TargetContent string          `json:"targetContent"`
	Overlays      []Overlay       `json:"overlays"`
	WritablePaths []WritablePath  `json:"writablePaths"`
	CopyTemplates []CopyTemplate  `json:"copyTemplates"`
	Permissions   PermissionPhase `json:"permissions"`
	ExcludePaths  []string        `json:"excludePaths,omitempty"` // Paths to exclude from overlay merge
}

// Overlay represents a stitched layer sourced from a mounted volume.
type Overlay struct {
	Name       string `json:"name"`
	SourcePath string `json:"sourcePath"`
}

// WritablePath configures passthrough directories that should stay writable.
type WritablePath struct {
	Path      string            `json:"path"`
	HostMount string            `json:"hostMount"`
	Template  *WritableTemplate `json:"template,omitempty"`
}

// WritableTemplate describes how to seed a writable path from another source.
type WritableTemplate struct {
	SourceMount string `json:"sourceMount"`
	SourcePath  string `json:"sourcePath"`
	Clean       bool   `json:"clean"`
}

// CopyTemplate mirrors the behaviour of copy-only overlays defined in values.yaml.
type CopyTemplate struct {
	SourceMount string `json:"sourceMount"`
	SourcePath  string `json:"sourcePath"`
	TargetPath  string `json:"targetPath"`
	Clean       bool   `json:"clean"`
	TargetMode  string `json:"targetMode,omitempty"`
	OnlyOnInit  bool   `json:"onlyOnInit,omitempty"` // Skip this copy during watcher re-merges
}

// PermissionPhase mirrors the subset of permissionsInit options that run during merges.
type PermissionPhase struct {
	ApplyDuringMerge bool     `json:"applyDuringMerge"`
	ApplyPaths       []string `json:"applyPaths"`
	User             int      `json:"user"`
	Group            int      `json:"group"`
	Mode             string   `json:"mode"`
}

// WatcherConfig configures the filesystem watcher sidecar.
type WatcherConfig struct {
	WatchPaths          []string `json:"watchPaths"`
	Events              []string `json:"events,omitempty"`
	DebounceSeconds     int      `json:"debounceSeconds"`
	PollIntervalSeconds int      `json:"pollIntervalSeconds"`
}

// PermissionJob defines a single chmod/chown pass executed inside an init container.
type PermissionJob struct {
	Path  string `json:"path"`
	User  int    `json:"user"`
	Group int    `json:"group"`
	Mode  string `json:"mode"`
}

// CopyJob models the entrypoint copy init container.
type CopyJob struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Mode        string `json:"mode"`
}

// FromEnv parses JSON configuration stored in an environment variable.
func FromEnv[T any](envKey string) (*T, error) {
	raw := strings.TrimSpace(os.Getenv(envKey))
	if raw == "" {
		return nil, fmt.Errorf("%w: %s", ErrMissingEnv, envKey)
	}
	var cfg T
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", envKey, err)
	}
	return &cfg, nil
}

// MustFromEnv behaves like FromEnv but panics when parsing fails.
func MustFromEnv[T any](envKey string) *T {
	cfg, err := FromEnv[T](envKey)
	if err != nil {
		panic(err)
	}
	return cfg
}

// ValidatePath ensures required config paths are present.
func ValidatePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("path must not be empty")
	}
	return nil
}
