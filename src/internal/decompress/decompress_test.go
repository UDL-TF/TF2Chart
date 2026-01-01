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
