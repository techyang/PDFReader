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
