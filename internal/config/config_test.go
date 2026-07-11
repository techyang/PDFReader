package config

import (
	"path/filepath"
	"testing"
)

func TestLoad_MissingFileReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if len(cfg.RecentFiles) != 0 {
		t.Fatalf("RecentFiles = %v, want empty", cfg.RecentFiles)
	}
	if cfg.WindowWidth != DefaultWindowWidth || cfg.WindowHeight != DefaultWindowHeight {
		t.Fatalf("default window size = %dx%d, want %dx%d", cfg.WindowWidth, cfg.WindowHeight, DefaultWindowWidth, DefaultWindowHeight)
	}
}

func TestSaveThenLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{
		WindowWidth:  1024,
		WindowHeight: 768,
		SidebarShown: true,
		SidebarTab:   "outline",
	}
	cfg.AddRecent(`C:\docs\a.pdf`)

	if err := cfg.SaveTo(path); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if loaded.WindowWidth != 1024 || loaded.WindowHeight != 768 {
		t.Fatalf("loaded size = %dx%d, want 1024x768", loaded.WindowWidth, loaded.WindowHeight)
	}
	if !loaded.SidebarShown || loaded.SidebarTab != "outline" {
		t.Fatalf("loaded sidebar state = %+v", loaded)
	}
	if len(loaded.RecentFiles) != 1 || loaded.RecentFiles[0].Path != `C:\docs\a.pdf` {
		t.Fatalf("loaded.RecentFiles = %+v", loaded.RecentFiles)
	}
}

func TestLoadFrom_CorruptFileReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := writeFile(path, []byte("{not json")); err != nil {
		t.Fatalf("writeFile: %v", err)
	}

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom should not error on corrupt file, got: %v", err)
	}
	if len(cfg.RecentFiles) != 0 {
		t.Fatalf("RecentFiles = %v, want empty default", cfg.RecentFiles)
	}
}

func TestAddRecent_DedupeAndMoveToFront(t *testing.T) {
	cfg := defaultConfig()
	cfg.AddRecent(`C:\a.pdf`)
	cfg.AddRecent(`C:\b.pdf`)
	cfg.AddRecent(`C:\a.pdf`) // re-open a.pdf, should move to front, not duplicate

	if len(cfg.RecentFiles) != 2 {
		t.Fatalf("len(RecentFiles) = %d, want 2", len(cfg.RecentFiles))
	}
	if cfg.RecentFiles[0].Path != `C:\a.pdf` {
		t.Fatalf("RecentFiles[0].Path = %q, want C:\\a.pdf", cfg.RecentFiles[0].Path)
	}
	if cfg.RecentFiles[1].Path != `C:\b.pdf` {
		t.Fatalf("RecentFiles[1].Path = %q, want C:\\b.pdf", cfg.RecentFiles[1].Path)
	}
}

func TestAddRecent_CapAtMax(t *testing.T) {
	cfg := defaultConfig()
	for i := 0; i < MaxRecentFiles+5; i++ {
		cfg.AddRecent(`C:\docs\` + string(rune('a'+i)) + `.pdf`)
	}
	if len(cfg.RecentFiles) != MaxRecentFiles {
		t.Fatalf("len(RecentFiles) = %d, want %d", len(cfg.RecentFiles), MaxRecentFiles)
	}
}

func TestRemoveRecent(t *testing.T) {
	cfg := defaultConfig()
	cfg.AddRecent(`C:\a.pdf`)
	cfg.AddRecent(`C:\b.pdf`)

	cfg.RemoveRecent(`C:\a.pdf`)

	if len(cfg.RecentFiles) != 1 || cfg.RecentFiles[0].Path != `C:\b.pdf` {
		t.Fatalf("RecentFiles = %+v, want only C:\\b.pdf", cfg.RecentFiles)
	}
}
