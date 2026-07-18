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

func TestAddRecent_DedupeCaseInsensitive(t *testing.T) {
	cfg := defaultConfig()
	cfg.AddRecent(`C:\a.pdf`)
	cfg.AddRecent(`C:\A.PDF`) // same file on Windows (case-insensitive filesystem), different casing

	if len(cfg.RecentFiles) != 1 {
		t.Fatalf("len(RecentFiles) = %d, want 1 (case-insensitive dedupe), got %+v", len(cfg.RecentFiles), cfg.RecentFiles)
	}
	if cfg.RecentFiles[0].Path != `C:\A.PDF` {
		t.Fatalf("RecentFiles[0].Path = %q, want C:\\A.PDF (most recently used casing)", cfg.RecentFiles[0].Path)
	}
}

func TestAddRecent_CapAtMax(t *testing.T) {
	cfg := defaultConfig()
	total := MaxRecentFiles + 5
	for i := 0; i < total; i++ {
		cfg.AddRecent(`C:\docs\` + string(rune('a'+i)) + `.pdf`)
	}
	if len(cfg.RecentFiles) != MaxRecentFiles {
		t.Fatalf("len(RecentFiles) = %d, want %d", len(cfg.RecentFiles), MaxRecentFiles)
	}

	// The newest MaxRecentFiles entries must survive, most-recently-added
	// first; the oldest 5 (a.pdf..e.pdf) must have been evicted. This pins
	// down WHICH entries survive and in what order, not just the count, so
	// a regression that truncates from the wrong end or reverses order
	// would be caught.
	want := make([]string, MaxRecentFiles)
	for i := 0; i < MaxRecentFiles; i++ {
		want[i] = `C:\docs\` + string(rune('a'+total-1-i)) + `.pdf`
	}
	for i, rf := range cfg.RecentFiles {
		if rf.Path != want[i] {
			t.Fatalf("RecentFiles[%d].Path = %q, want %q (full: %+v)", i, rf.Path, want[i], cfg.RecentFiles)
		}
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

func TestRemoveRecent_CaseInsensitive(t *testing.T) {
	cfg := defaultConfig()
	cfg.AddRecent(`C:\a.pdf`)
	cfg.AddRecent(`C:\b.pdf`)

	cfg.RemoveRecent(`C:\A.PDF`) // same file, different casing

	if len(cfg.RecentFiles) != 1 || cfg.RecentFiles[0].Path != `C:\b.pdf` {
		t.Fatalf("RecentFiles = %+v, want only C:\\b.pdf", cfg.RecentFiles)
	}
}

func TestSaveThenLoad_ContinuousMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{ContinuousMode: true}
	if err := cfg.SaveTo(path); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if !loaded.ContinuousMode {
		t.Fatalf("loaded.ContinuousMode = false, want true")
	}
}

func TestLoad_MissingFileDefaultsToSinglePageMode(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadFrom(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.ContinuousMode {
		t.Fatalf("cfg.ContinuousMode = true, want false (default is single-page mode)")
	}
}

func TestSaveThenLoad_PrintSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{
		LastPrinter:      "Microsoft Print to PDF",
		LastGrayscale:    true,
		LastDuplex:       true,
		LastPaperName:    "A4",
		LastOrientation:  "landscape",
		LastScaleMode:    "percent",
		LastScalePercent: 75,
	}
	if err := cfg.SaveTo(path); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if loaded.LastPrinter != "Microsoft Print to PDF" {
		t.Fatalf("loaded.LastPrinter = %q, want %q", loaded.LastPrinter, "Microsoft Print to PDF")
	}
	if !loaded.LastGrayscale || !loaded.LastDuplex {
		t.Fatalf("loaded grayscale/duplex = %v/%v, want true/true", loaded.LastGrayscale, loaded.LastDuplex)
	}
	if loaded.LastPaperName != "A4" || loaded.LastOrientation != "landscape" {
		t.Fatalf("loaded paper/orientation = %q/%q, want A4/landscape", loaded.LastPaperName, loaded.LastOrientation)
	}
	if loaded.LastScaleMode != "percent" || loaded.LastScalePercent != 75 {
		t.Fatalf("loaded scale = %q/%d, want percent/75", loaded.LastScaleMode, loaded.LastScalePercent)
	}
}

func TestLoad_MissingFileDefaultsToNoPrinter(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadFrom(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.LastPrinter != "" {
		t.Fatalf("cfg.LastPrinter = %q, want empty (fall back to system default printer at print time)", cfg.LastPrinter)
	}
}
