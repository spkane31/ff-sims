package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverLeaguePaths_Basic(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "345674", "2024"), 0755); err != nil {
		t.Fatal(err)
	}

	paths, err := discoverLeaguePaths(root, "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}
	if paths[0].externalID != "345674" {
		t.Errorf("expected externalID 345674, got %s", paths[0].externalID)
	}
	if paths[0].year != 2024 {
		t.Errorf("expected year 2024, got %d", paths[0].year)
	}
	if paths[0].dir != filepath.Join(root, "345674", "2024") {
		t.Errorf("unexpected dir: %s", paths[0].dir)
	}
}

func TestDiscoverLeaguePaths_LeagueFilter(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "345674", "2024"), 0755)
	os.MkdirAll(filepath.Join(root, "999999", "2024"), 0755)

	paths, err := discoverLeaguePaths(root, "345674", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 path after league filter, got %d", len(paths))
	}
	if paths[0].externalID != "345674" {
		t.Errorf("expected externalID 345674, got %s", paths[0].externalID)
	}
}

func TestDiscoverLeaguePaths_YearFilter(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "345674", "2023"), 0755)
	os.MkdirAll(filepath.Join(root, "345674", "2024"), 0755)

	paths, err := discoverLeaguePaths(root, "", 2024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 path after year filter, got %d", len(paths))
	}
	if paths[0].year != 2024 {
		t.Errorf("expected year 2024, got %d", paths[0].year)
	}
}

func TestDiscoverLeaguePaths_BothFilters(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "345674", "2023"), 0755)
	os.MkdirAll(filepath.Join(root, "345674", "2024"), 0755)
	os.MkdirAll(filepath.Join(root, "999999", "2024"), 0755)

	paths, err := discoverLeaguePaths(root, "345674", 2024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 path with both filters, got %d", len(paths))
	}
	if paths[0].externalID != "345674" || paths[0].year != 2024 {
		t.Errorf("unexpected path: %+v", paths[0])
	}
}

func TestDiscoverLeaguePaths_SkipsNonYearDirs(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "345674", "2024"), 0755)
	os.MkdirAll(filepath.Join(root, "345674", "notayear"), 0755)

	paths, err := discoverLeaguePaths(root, "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 path (non-year dir skipped), got %d", len(paths))
	}
	if paths[0].year != 2024 {
		t.Errorf("expected year 2024, got %d", paths[0].year)
	}
}

func TestDiscoverLeaguePaths_SkipsFilesAtLeagueLevel(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "345674", "2024"), 0755)
	os.WriteFile(filepath.Join(root, "readme.txt"), []byte("ignore"), 0644)

	paths, err := discoverLeaguePaths(root, "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 path (top-level file skipped), got %d", len(paths))
	}
}

func TestDiscoverLeaguePaths_EmptyDataDir(t *testing.T) {
	root := t.TempDir()

	paths, err := discoverLeaguePaths(root, "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("expected 0 paths for empty data dir, got %d", len(paths))
	}
}

func TestDiscoverLeaguePaths_MultipleLeaguesAndYears(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "345674", "2023"), 0755)
	os.MkdirAll(filepath.Join(root, "345674", "2024"), 0755)
	os.MkdirAll(filepath.Join(root, "999999", "2024"), 0755)

	paths, err := discoverLeaguePaths(root, "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 3 {
		t.Fatalf("expected 3 paths, got %d", len(paths))
	}
}
