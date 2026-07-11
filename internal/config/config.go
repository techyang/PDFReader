package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	DefaultWindowWidth  = 1000
	DefaultWindowHeight = 720
	MaxRecentFiles      = 10
)

// RecentFile is one entry in the recently-opened-files list.
type RecentFile struct {
	Path       string    `json:"path"`
	LastOpened time.Time `json:"lastOpened"`
}

// Config is the persisted application state.
type Config struct {
	RecentFiles  []RecentFile `json:"recentFiles"`
	WindowWidth  int          `json:"windowWidth"`
	WindowHeight int          `json:"windowHeight"`
	SidebarShown bool         `json:"sidebarShown"`
	SidebarTab   string       `json:"sidebarTab"` // "outline" or "thumbnails"
}

func defaultConfig() *Config {
	return &Config{
		WindowWidth:  DefaultWindowWidth,
		WindowHeight: DefaultWindowHeight,
		SidebarShown: true,
		SidebarTab:   "outline",
	}
}

// Path returns the platform config file path: %APPDATA%\PDFReader\config.json
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "PDFReader", "config.json"), nil
}

// Load reads the config from the standard location. If the file is
// missing or corrupt, it silently returns default values instead of
// failing, so a bad config never blocks app startup.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return defaultConfig(), nil
	}
	return LoadFrom(path)
}

// LoadFrom reads the config from an explicit path (used by tests).
func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultConfig(), nil
	}

	cfg := defaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return defaultConfig(), nil
	}
	return cfg, nil
}

// Save writes the config to the standard location, creating the parent
// directory if needed.
func (c *Config) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	return c.SaveTo(path)
}

// SaveTo writes the config to an explicit path (used by tests).
func (c *Config) SaveTo(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(path, data)
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// AddRecent adds path to the front of the recent-files list, moving it to
// the front (without duplicating) if it's already present, and capping the
// list at MaxRecentFiles.
func (c *Config) AddRecent(path string) {
	filtered := make([]RecentFile, 0, len(c.RecentFiles)+1)
	filtered = append(filtered, RecentFile{Path: path, LastOpened: time.Now()})
	for _, rf := range c.RecentFiles {
		if rf.Path == path {
			continue
		}
		filtered = append(filtered, rf)
	}
	if len(filtered) > MaxRecentFiles {
		filtered = filtered[:MaxRecentFiles]
	}
	c.RecentFiles = filtered
}

// RemoveRecent removes path from the recent-files list, if present.
func (c *Config) RemoveRecent(path string) {
	filtered := make([]RecentFile, 0, len(c.RecentFiles))
	for _, rf := range c.RecentFiles {
		if rf.Path == path {
			continue
		}
		filtered = append(filtered, rf)
	}
	c.RecentFiles = filtered
}
