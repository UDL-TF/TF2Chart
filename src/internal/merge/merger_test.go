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
	info, err := os.Lstat(filepath.Join(targetBase, "tf", "cfg", "server.cfg"))
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
	// With targetMode "writable" and targetPath "tf/templates", the code strips the "tf/" prefix
	// to avoid duplication, so the file ends up at targetContent/templates, not targetContent/tf/templates
	if _, err := os.Stat(filepath.Join(targetContent, "templates", "cfg.cfg")); err != nil {
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

// TestCopyTemplateFromBasePath tests copying files from the base path (/mnt/base)
// to a specific target location. This simulates copying from /tf/test-two in base
// to tf/addons/sourcemod/configs/sourcebans in the view layer.
func TestCopyTemplateFromBasePath(t *testing.T) {
	base := t.TempDir()
	targetBase := filepath.Join(t.TempDir(), "view")
	targetContent := filepath.Join(targetBase, "tf")
	if err := os.MkdirAll(targetContent, 0o755); err != nil {
		t.Fatalf("mkdir target content: %v", err)
	}

	// Create test source files in base path at /tf/test-two
	testSourcePath := filepath.Join(base, "tf", "test-two")
	writeFile(t, filepath.Join(testSourcePath, "sourcebans", "core-functions.txt"), "// Core functions")
	writeFile(t, filepath.Join(testSourcePath, "sourcebans", "sourcebans.cfg"), "// Sourcebans config")
	writeFile(t, filepath.Join(testSourcePath, "sourcebans", "db.cfg"), "// Database config")
	writeFile(t, filepath.Join(testSourcePath, "sourcebans", "nested", "deep.txt"), "// Deep nested file")

	cfg := &config.MergeConfig{
		BasePath:      base,
		TargetBase:    targetBase,
		TargetContent: targetContent,
		CopyTemplates: []config.CopyTemplate{
			{
				// When sourceMount is not specified, it defaults to "/mnt/base"
				// In tests, this translates to using the base directory
				SourceMount: base,                    // Simulates /mnt/base
				SourcePath:  "tf/test-two/sourcebans", // Path within base
				TargetPath:  "tf/addons/sourcemod/configs/sourcebans",
				Clean:       true,
				TargetMode:  "view", // Copy to view layer (merged content)
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

	// Verify files were copied to the target location
	targetDir := filepath.Join(targetContent, "addons", "sourcemod", "configs", "sourcebans")
	
	// Check that all files exist
	checkFile := func(relPath, expectedContent string) {
		fullPath := filepath.Join(targetDir, relPath)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			t.Fatalf("expected file %s to exist: %v", fullPath, err)
		}
		if string(content) != expectedContent {
			t.Errorf("file %s: got %q, want %q", fullPath, string(content), expectedContent)
		}
		// Verify it's a real file, not a symlink
		info, err := os.Lstat(fullPath)
		if err != nil {
			t.Fatalf("stat %s: %v", fullPath, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Errorf("expected physical file at %s, got symlink", fullPath)
		}
	}

	checkFile("core-functions.txt", "// Core functions")
	checkFile("sourcebans.cfg", "// Sourcebans config")
	checkFile("db.cfg", "// Database config")
	checkFile("nested/deep.txt", "// Deep nested file")
}

// TestCopyTemplateFromBasePathWritableMode tests copying from base path
// to a writable location in the target base.
func TestCopyTemplateFromBasePathWritableMode(t *testing.T) {
	base := t.TempDir()
	targetBase := filepath.Join(t.TempDir(), "view")
	targetContent := filepath.Join(targetBase, "tf")
	if err := os.MkdirAll(targetContent, 0o755); err != nil {
		t.Fatalf("mkdir target content: %v", err)
	}

	// Create test source files in base path
	testSourcePath := filepath.Join(base, "tf", "test-two", "configs")
	writeFile(t, filepath.Join(testSourcePath, "config1.cfg"), "config1")
	writeFile(t, filepath.Join(testSourcePath, "config2.cfg"), "config2")

	cfg := &config.MergeConfig{
		BasePath:      base,
		TargetBase:    targetBase,
		TargetContent: targetContent,
		CopyTemplates: []config.CopyTemplate{
			{
				SourceMount: base,
				SourcePath:  "tf/test-two/configs",
				TargetPath:  "tf/custom-configs",
				Clean:       true,
				TargetMode:  "writable", // Copy to writable area
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

	// With targetMode "writable", files should be in targetContent/custom-configs
	targetDir := filepath.Join(targetContent, "custom-configs")
	
	verifyFile := func(filename, expectedContent string) {
		fullPath := filepath.Join(targetDir, filename)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			t.Fatalf("expected file %s: %v", fullPath, err)
		}
		if string(content) != expectedContent {
			t.Errorf("file %s: got %q, want %q", fullPath, string(content), expectedContent)
		}
	}

	verifyFile("config1.cfg", "config1")
	verifyFile("config2.cfg", "config2")
}

// TestCopyTemplateDereferencesSymlinks tests that when copying templates,
// symlinks in the source are dereferenced and the actual file content is copied.
func TestCopyTemplateDereferencesSymlinks(t *testing.T) {
	base := t.TempDir()
	targetBase := filepath.Join(t.TempDir(), "view")
	targetContent := filepath.Join(targetBase, "tf")
	if err := os.MkdirAll(targetContent, 0o755); err != nil {
		t.Fatalf("mkdir target content: %v", err)
	}

	// Create real files in a separate location
	realFiles := t.TempDir()
	writeFile(t, filepath.Join(realFiles, "real1.cfg"), "real content 1")
	writeFile(t, filepath.Join(realFiles, "real2.cfg"), "real content 2")
	writeFile(t, filepath.Join(realFiles, "nested", "real3.cfg"), "real content 3")

	// Create symlinks in the base path pointing to the real files
	testSourcePath := filepath.Join(base, "configs")
	if err := os.MkdirAll(testSourcePath, 0o755); err != nil {
		t.Fatalf("mkdir source path: %v", err)
	}
	if err := os.Symlink(filepath.Join(realFiles, "real1.cfg"), filepath.Join(testSourcePath, "real1.cfg")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	if err := os.Symlink(filepath.Join(realFiles, "real2.cfg"), filepath.Join(testSourcePath, "real2.cfg")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(testSourcePath, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.Symlink(filepath.Join(realFiles, "nested", "real3.cfg"), filepath.Join(testSourcePath, "nested", "real3.cfg")); err != nil {
		t.Fatalf("create nested symlink: %v", err)
	}

	cfg := &config.MergeConfig{
		BasePath:      base,
		TargetBase:    targetBase,
		TargetContent: targetContent,
		CopyTemplates: []config.CopyTemplate{
			{
				SourceMount: base,
				SourcePath:  "configs",
				TargetPath:  "tf/configs",
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

	// Verify files were copied as physical files, not symlinks
	targetDir := filepath.Join(targetContent, "configs")
	
	checkPhysicalFile := func(relPath, expectedContent string) {
		fullPath := filepath.Join(targetDir, relPath)
		
		// Verify it's NOT a symlink
		info, err := os.Lstat(fullPath)
		if err != nil {
			t.Fatalf("stat %s: %v", fullPath, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("expected physical file at %s, got symlink", fullPath)
		}
		
		// Verify content
		content, err := os.ReadFile(fullPath)
		if err != nil {
			t.Fatalf("read file %s: %v", fullPath, err)
		}
		if string(content) != expectedContent {
			t.Errorf("file %s: got %q, want %q", fullPath, string(content), expectedContent)
		}
	}

	checkPhysicalFile("real1.cfg", "real content 1")
	checkPhysicalFile("real2.cfg", "real content 2")
	checkPhysicalFile("nested/real3.cfg", "real content 3")
}

// TestCopyTemplateOnlyOnInit tests that templates with onlyOnInit=true
// are only copied during the first merge run.
func TestCopyTemplateOnlyOnInit(t *testing.T) {
	base := t.TempDir()
	targetBase := filepath.Join(t.TempDir(), "view")
	targetContent := filepath.Join(targetBase, "tf")
	if err := os.MkdirAll(targetContent, 0o755); err != nil {
		t.Fatalf("mkdir target content: %v", err)
	}

	testSourcePath := filepath.Join(base, "templates")
	writeFile(t, filepath.Join(testSourcePath, "initial.cfg"), "initial content")

	cfg := &config.MergeConfig{
		BasePath:      base,
		TargetBase:    targetBase,
		TargetContent: targetContent,
		CopyTemplates: []config.CopyTemplate{
			{
				SourceMount: base,
				SourcePath:  "templates",
				TargetPath:  "tf/configs",
				Clean:       true,
				TargetMode:  "writable",
				OnlyOnInit:  true, // Only copy on first run
			},
		},
		Permissions: config.PermissionPhase{},
	}

	m, err := New(cfg)
	if err != nil {
		t.Fatalf("new merger: %v", err)
	}

	// First run - should copy
	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("run merge (first): %v", err)
	}

	targetFile := filepath.Join(targetContent, "configs", "initial.cfg")
	if _, err := os.Stat(targetFile); err != nil {
		t.Fatalf("file should exist after first run: %v", err)
	}

	// Modify the copied file to simulate server changes
	if err := os.WriteFile(targetFile, []byte("modified by server"), 0o644); err != nil {
		t.Fatalf("modify file: %v", err)
	}

	// Second run - should NOT overwrite because onlyOnInit=true
	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("run merge (second): %v", err)
	}

	content, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("read file after second run: %v", err)
	}
	if string(content) != "modified by server" {
		t.Errorf("file should not be overwritten on second run: got %q", string(content))
	}
}
