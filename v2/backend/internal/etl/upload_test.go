package etl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUploadWithOptions_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := UploadWithOptions(dir, 1, false); err != nil {
		t.Fatalf("expected nil for empty directory, got: %v", err)
	}
}

func TestUploadWithOptions_DirectoryWithOnlySubdirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := UploadWithOptions(dir, 1, false); err != nil {
		t.Fatalf("expected nil for directory with only subdirs, got: %v", err)
	}
}
