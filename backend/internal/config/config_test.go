package config

import "testing"

func TestArchiveDBConfig_Enabled(t *testing.T) {
	cases := []struct {
		name string
		cfg  ArchiveDBConfig
		want bool
	}{
		{"empty connection string is disabled", ArchiveDBConfig{}, false},
		{"non-empty connection string is enabled", ArchiveDBConfig{ConnectionString: "postgres://x"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.Enabled(); got != tc.want {
				t.Errorf("Enabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLoad_ArchiveDBDefaultsToDisabled(t *testing.T) {
	t.Setenv("ARCHIVE_DATABASE_URL", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ArchiveDB.Enabled() {
		t.Errorf("expected ArchiveDB disabled by default, got ConnectionString=%q", cfg.ArchiveDB.ConnectionString)
	}
}

func TestLoad_ArchiveDBReadsEnv(t *testing.T) {
	t.Setenv("ARCHIVE_DATABASE_URL", "postgres://archive-host/db")
	t.Setenv("ARCHIVE_DB_MAX_OPEN_CONNS", "7")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.ArchiveDB.Enabled() {
		t.Fatal("expected ArchiveDB enabled")
	}
	if cfg.ArchiveDB.ConnectionString != "postgres://archive-host/db" {
		t.Errorf("ConnectionString = %q", cfg.ArchiveDB.ConnectionString)
	}
	if cfg.ArchiveDB.PoolMaxOpenConns != 7 {
		t.Errorf("PoolMaxOpenConns = %d, want 7", cfg.ArchiveDB.PoolMaxOpenConns)
	}
}
