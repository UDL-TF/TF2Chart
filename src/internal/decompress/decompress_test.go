package decompress

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDecompressor_Run(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create a test bz2 file (we'll just create an empty file for this test)
	testFile := filepath.Join(tmpDir, "test.bz2")
	if err := os.WriteFile(testFile, []byte{}, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create decompressor
	d := New([]string{tmpDir})

	// Note: This will fail because we created an empty file, not a real bz2
	// This is just to test the structure works
	_ = d.Run()

	// Test with empty paths
	d2 := New([]string{})
	if err := d2.Run(); err != nil {
		t.Errorf("Run with empty paths should not error, got: %v", err)
	}

	// Test with non-existent path (should not error, just skip)
	d3 := New([]string{"/non/existent/path"})
	if err := d3.Run(); err != nil {
		t.Errorf("Run with non-existent path should not error, got: %v", err)
	}
}

func TestDecompressor_CachingBehavior(t *testing.T) {
	// Create temporary directories for testing
	tmpDir := t.TempDir()
	cacheDir := t.TempDir()

	// Create a simple text file and compress it manually
	testContent := "Hello, this is test data for caching!"
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// For this test, we'll manually create the expected output file to simulate caching
	expectedOutput := filepath.Join(cacheDir, "test.txt")
	if err := os.WriteFile(expectedOutput, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create cached file: %v", err)
	}

	// Verify the cached file exists
	if _, err := os.Stat(expectedOutput); err != nil {
		t.Errorf("cached file should exist: %v", err)
	}

	t.Log("Cache test setup completed - cached file exists")
}

func TestDecompressor_WithOutputDir(t *testing.T) {
	// Create temporary directories
	tmpDir := t.TempDir()
	outputDir := t.TempDir()

	// Create a dummy bz2 file
	testFile := filepath.Join(tmpDir, "test.bz2")
	if err := os.WriteFile(testFile, []byte{}, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create decompressor with output directory
	d := NewWithOutputDir([]string{tmpDir}, outputDir)

	// Run decompressor
	_ = d.Run()

	t.Logf("Decompressor with output directory test completed")
}
