# ETL Directory-Driven Discovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the flag-driven single-league ETL upload with a directory-walker that discovers `{leagueExternalID}/{year}/` subtrees automatically, with `--league-id` and `--year` as optional filters.

**Architecture:** `uploadCmd.RunE` is replaced with a walker that reads two directory levels (`leagueExternalID` → `year`), then calls `resolveLeagueID` + `etl.UploadWithOptions` for each discovered pair. A helper function `discoverLeaguePaths` encapsulates the directory walk logic so it can be unit-tested without a database. `UploadWithOptions` gains an early-return guard for directories with no files.

**Tech Stack:** Go 1.25, `github.com/spf13/cobra`, standard library `os`, `path/filepath`, `strconv`

## Global Constraints

- `--platform` applies to all discovered leagues (single global flag, default `ESPN`)
- `--league-id` is now an optional filter on `upload` (still required on `xwins`)
- `--year` on `upload` uses value 0 to mean "all years" (consistent with `xwins` convention)
- `etl xwins` behavior is unchanged
- No changes to any database models or API layer

---

### Task 1: Empty-dir guard in `UploadWithOptions`

**Files:**
- Modify: `v2/backend/internal/etl/upload.go` (insert guard after `os.ReadDir`, ~line 697)
- Create: `v2/backend/internal/etl/upload_test.go`

**Interfaces:**
- Produces: `UploadWithOptions(dir string, leagueID uint, calculateExpectedWins bool) error` — unchanged signature; now returns `nil` with a warning log when `dir` contains no files (or only subdirectories)

- [ ] **Step 1: Write the failing test**

Create `v2/backend/internal/etl/upload_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to confirm they already pass (regression baseline)**

```bash
cd v2/backend && go test ./internal/etl/... -run "TestUploadWithOptions_Empty|TestUploadWithOptions_DirectoryWithOnly" -v
```

Expected: both tests **PASS** — the existing loops already iterate over zero entries and return `nil`. These tests document and protect this behavior. The next step adds the warning log.

- [ ] **Step 3: Add the empty-dir warning log**

In `v2/backend/internal/etl/upload.go`, find the block after `os.ReadDir` (around line 697) and add the guard. The result should look like:

```go
files, err := os.ReadDir(directory)
if err != nil {
    return fmt.Errorf("failed to read directory %s: %w", directory, err)
}

fileCount := 0
for _, f := range files {
    if !f.IsDir() {
        fileCount++
    }
}
if fileCount == 0 {
    logging.Warnf("No files found in %s, skipping", directory)
    return nil
}

// Regex to extract file type from filename (pattern: {type}_{year}.json)
re := regexp.MustCompile(`^(.+)_\d{4}\.json$`)
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
cd v2/backend && go test ./internal/etl/... -run "TestUploadWithOptions_Empty|TestUploadWithOptions_DirectoryWithOnly" -v
```

Expected:
```
--- PASS: TestUploadWithOptions_EmptyDirectory (0.00s)
--- PASS: TestUploadWithOptions_DirectoryWithOnlySubdirectories (0.00s)
PASS
```

- [ ] **Step 5: Verify the full package still builds**

```bash
cd v2/backend && go build ./internal/etl/...
```

Expected: no output (clean build).

- [ ] **Step 6: Commit**

```bash
git add v2/backend/internal/etl/upload.go v2/backend/internal/etl/upload_test.go
git commit -m "feat(etl): warn and skip empty data directories in UploadWithOptions"
```

---

### Task 2: Directory walker and flag changes in `uploadCmd`

**Files:**
- Modify: `v2/backend/cmd/etl/main.go`
- Create: `v2/backend/cmd/etl/main_test.go`

**Interfaces:**
- Consumes: `etl.UploadWithOptions(dir string, leagueID uint, calculateExpectedWins bool) error` (from Task 1)
- Produces:
  - `discoverLeaguePaths(dataDir, leagueFilter string, yearFilter uint) ([]leaguePath, error)` — unexported, returns one entry per valid `(leagueExternalID, year)` pair
  - `leaguePath` struct: `{ externalID string; year uint; dir string }`

- [ ] **Step 1: Write the failing tests**

Create `v2/backend/cmd/etl/main_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd v2/backend && go test ./cmd/etl/... -v
```

Expected: compile error — `discoverLeaguePaths` and `leaguePath` not defined yet.

- [ ] **Step 3: Implement `discoverLeaguePaths` and update `main.go`**

Replace the contents of `v2/backend/cmd/etl/main.go` with the following. Key changes from the original:
  - Add `uploadYear uint` to the package-level vars block
  - Add `"path/filepath"` and `"strconv"` to imports
  - Add `leaguePath` struct and `discoverLeaguePaths` function
  - Replace `uploadCmd.RunE` with the directory walker (remove the `--league-id is required` check)
  - Add `--year` flag to `uploadCmd`

```go
package main

import (
	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/etl"
	"backend/internal/logging"
	"backend/internal/models"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

var (
	dataDir          string
	skipExpectedWins bool
	processYear      uint
	uploadYear       uint
	leagueExternalID string
	platform         string
)

type leaguePath struct {
	externalID string
	year       uint
	dir        string
}

// discoverLeaguePaths walks dataDir two levels deep to find {leagueExternalID}/{year}/
// subdirectories. leagueFilter and yearFilter are optional (zero value = no filter).
func discoverLeaguePaths(dataDir, leagueFilter string, yearFilter uint) ([]leaguePath, error) {
	leagueDirs, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read data directory %s: %w", dataDir, err)
	}

	var paths []leaguePath
	for _, entry := range leagueDirs {
		if !entry.IsDir() {
			continue
		}
		externalID := entry.Name()
		if leagueFilter != "" && externalID != leagueFilter {
			continue
		}

		yearDirs, err := os.ReadDir(filepath.Join(dataDir, externalID))
		if err != nil {
			return nil, fmt.Errorf("failed to read league directory %s: %w", externalID, err)
		}

		for _, yearEntry := range yearDirs {
			if !yearEntry.IsDir() {
				continue
			}
			yearStr := yearEntry.Name()
			parsed, err := strconv.ParseUint(yearStr, 10, 32)
			if err != nil {
				logging.Warnf("Skipping non-year directory %q in league %s", yearStr, externalID)
				continue
			}
			year := uint(parsed)
			if yearFilter > 0 && year != yearFilter {
				continue
			}
			paths = append(paths, leaguePath{
				externalID: externalID,
				year:       year,
				dir:        filepath.Join(dataDir, externalID, yearStr),
			})
		}
	}
	return paths, nil
}

// TODO(temporal): migrate to Temporal workflow — see issue for Temporal migration
func main() {
	rootCmd := &cobra.Command{
		Use:   "etl",
		Short: "ETL service for fantasy football simulations",
		Long:  "Extract, Transform, Load service for processing fantasy football data",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logging.Infof("ETL service started")
			logging.Infof("Using data directory: %s", dataDir)
		},
	}

	rootCmd.PersistentFlags().StringVar(&dataDir, "data-dir", "./data", "Directory containing data files")
	rootCmd.PersistentFlags().StringVar(&leagueExternalID, "league-id", "", "Platform-assigned league ID filter (e.g. 345674); optional for upload")
	rootCmd.PersistentFlags().StringVar(&platform, "platform", "ESPN", "Fantasy platform: ESPN, Sleeper, Yahoo")

	uploadCmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload data to the database",
		Long:  "Process and upload data files to the database",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := discoverLeaguePaths(dataDir, leagueExternalID, uploadYear)
			if err != nil {
				return err
			}
			if len(paths) == 0 {
				logging.Warnf("No league/year directories found under %s", dataDir)
				return nil
			}
			for _, p := range paths {
				leagueID, err := resolveLeagueID(p.externalID, platform)
				if err != nil {
					return err
				}
				logging.Infof("Processing league %s (internal ID %d), year %d from %s",
					p.externalID, leagueID, p.year, p.dir)
				if err := etl.UploadWithOptions(p.dir, leagueID, !skipExpectedWins); err != nil {
					return err
				}
			}
			return nil
		},
	}
	uploadCmd.Flags().BoolVar(&skipExpectedWins, "skip-expected-wins", false, "Skip expected wins calculations during ETL")
	uploadCmd.Flags().UintVar(&uploadYear, "year", 0, "Specific year to process (0 = all years)")

	xwinsCmd := &cobra.Command{
		Use:   "xwins",
		Short: "Calculate expected wins",
		Long:  "Calculate expected wins for fantasy football teams based on their performance",
		RunE: func(cmd *cobra.Command, args []string) error {
			if leagueExternalID == "" {
				return fmt.Errorf("--league-id is required")
			}
			leagueID, err := resolveLeagueID(leagueExternalID, platform)
			if err != nil {
				return err
			}
			if processYear > 0 {
				logging.Infof("Running expected wins calculation for year %d only", processYear)
			} else {
				logging.Infof("Running expected wins calculation for all years")
			}
			return etl.ProcessExpectedWinsWithYear(leagueID, processYear)
		},
	}
	xwinsCmd.Flags().UintVar(&processYear, "year", 0, "Specific year to process for expected wins (0 = all years, starting with most recent)")

	rootCmd.AddCommand(uploadCmd)
	rootCmd.AddCommand(xwinsCmd)

	rootCmd.SilenceUsage = true
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	if err := rootCmd.Execute(); err != nil {
		logging.Errorf("Error executing command: %v", err)
		os.Exit(1)
	}
}

// resolveLeagueID initialises the database and looks up the internal league ID
// by (external_id, platform). Returns an error if the league is not found.
func resolveLeagueID(externalID, plt string) (uint, error) {
	cfg, err := config.Load()
	if err != nil {
		return 0, fmt.Errorf("error loading configuration: %w", err)
	}
	if err := database.Initialize(cfg); err != nil {
		return 0, fmt.Errorf("error initialising database: %w", err)
	}

	var league models.League
	err = database.DB.Where("external_id = ? AND platform = ?", externalID, plt).First(&league).Error
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			return 0, fmt.Errorf("error looking up league: %w", err)
		}
		league = models.League{
			Name:       fmt.Sprintf("%s League %s", plt, externalID),
			Platform:   plt,
			ExternalID: externalID,
		}
		if createErr := database.DB.Create(&league).Error; createErr != nil {
			return 0, fmt.Errorf("error creating league: %w", createErr)
		}
		logging.Infof("Created new league %q (platform=%s) with internal ID %d", externalID, plt, league.ID)
	} else {
		logging.Infof("Resolved league %q (platform=%s) to internal ID %d", externalID, plt, league.ID)
	}
	return league.ID, nil
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
cd v2/backend && go test ./cmd/etl/... -v
```

Expected:
```
--- PASS: TestDiscoverLeaguePaths_Basic (0.00s)
--- PASS: TestDiscoverLeaguePaths_LeagueFilter (0.00s)
--- PASS: TestDiscoverLeaguePaths_YearFilter (0.00s)
--- PASS: TestDiscoverLeaguePaths_BothFilters (0.00s)
--- PASS: TestDiscoverLeaguePaths_SkipsNonYearDirs (0.00s)
--- PASS: TestDiscoverLeaguePaths_SkipsFilesAtLeagueLevel (0.00s)
--- PASS: TestDiscoverLeaguePaths_EmptyDataDir (0.00s)
--- PASS: TestDiscoverLeaguePaths_MultipleLeaguesAndYears (0.00s)
PASS
```

- [ ] **Step 5: Confirm the full backend builds**

```bash
cd v2/backend && go build ./...
```

Expected: no output (clean build).

- [ ] **Step 6: Run the full test suite**

```bash
cd v2/backend && go test ./...
```

Expected: all tests pass (no regressions in simulation tests).

- [ ] **Step 7: Commit**

```bash
git add v2/backend/cmd/etl/main.go v2/backend/cmd/etl/main_test.go
git commit -m "feat(etl): replace single-league upload with directory-driven discovery

- uploadCmd now walks {data-dir}/{leagueExternalID}/{year}/ subtrees
- --league-id and --year are optional filters on upload (--league-id
  remains required on xwins)
- discoverLeaguePaths is unit-tested without a database connection"
```
