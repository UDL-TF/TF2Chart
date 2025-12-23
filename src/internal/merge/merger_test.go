package merge

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/UDL-TF/TF2Chart/src/internal/config"
)

func TestMergerCreatesSymlinks(t *testing.T) {
	base := t.TempDir()
	targetBase := filepath.Join(t.TempDir(), "view")
	targetContent := filepath.Join(targetBase, "tf")
	if err := os.MkdirAll(targetContent, 0o755); err != nil {
		t.Fatalf("mkdir target content: %v", err)
	}
	writeFile(t, filepath.Join(base, "file.txt"), "base")
	overlay := t.TempDir()
	writeFile(t, filepath.Join(overlay, "overlay.txt"), "ov")

	cfg := &config.MergeConfig{
		BasePath:      base,
		TargetBase:    targetBase,
		TargetContent: targetContent,
		Overlays: []config.Overlay{{
			Name:       "overlay",
			SourcePath: overlay,
		}},
		Permissions: config.PermissionPhase{},
	}
	m, err := New(cfg)
	if err != nil {
		t.Fatalf("new merger: %v", err)
	}
	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("run merge: %v", err)
	}
	assertSymlink(t, filepath.Join(targetBase, "file.txt"))
	assertSymlink(t, filepath.Join(targetContent, "overlay.txt"))
}

func TestWritableTemplateCopiesPhysicalFiles(t *testing.T) {
	base := t.TempDir()
	targetBase := filepath.Join(t.TempDir(), "view")
	targetContent := filepath.Join(targetBase, "tf")
	if err := os.MkdirAll(targetContent, 0o755); err != nil {
		t.Fatalf("mkdir target content: %v", err)
	}
	templateSrc := t.TempDir()
	writeFile(t, filepath.Join(templateSrc, "cfg", "server.cfg"), "cfg")

	cfg := &config.MergeConfig{
		BasePath:      base,
		TargetBase:    targetBase,
		TargetContent: targetContent,
		WritablePaths: []config.WritablePath{
			{
				Path:      "tf/cfg",
				HostMount: base,
				Template: &config.WritableTemplate{
					SourceMount: templateSrc,
					SourcePath:  "cfg",
					Clean:       true,
				},
			},
		},
		Permissions: config.PermissionPhase{},
	}
	m, err := New(cfg)
	if err != nil {
		t.Fatalf("new merger: %v", err)
	}
	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("run merge: %v", err)
	}
	info, err := os.Lstat(filepath.Join(targetContent, "tf", "cfg", "server.cfg"))
	if err != nil {
		t.Fatalf("stat copied file: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("expected physical file, found symlink")
	}
}

func TestCopyTemplateTargetModeWritable(t *testing.T) {
	base := t.TempDir()
	targetBase := filepath.Join(t.TempDir(), "view")
	targetContent := filepath.Join(targetBase, "tf")
	if err := os.MkdirAll(targetContent, 0o755); err != nil {
		t.Fatalf("mkdir target content: %v", err)
	}
	overlay := t.TempDir()
	writeFile(t, filepath.Join(overlay, "templates", "cfg.cfg"), "data")

	cfg := &config.MergeConfig{
		BasePath:      base,
		TargetBase:    targetBase,
		TargetContent: targetContent,
		CopyTemplates: []config.CopyTemplate{
			{
				SourceMount: overlay,
				SourcePath:  "templates",
				TargetPath:  "tf/templates",
				Clean:       true,
				TargetMode:  "writable",
			},
		},
		Permissions: config.PermissionPhase{},
	}
	m, err := New(cfg)
	if err != nil {
		t.Fatalf("new merger: %v", err)
	}
	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("run merge: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetContent, "tf", "templates", "cfg.cfg")); err != nil {
		t.Fatalf("expected copied template in writable target: %v", err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertSymlink(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink at %s", path)
	}
}
