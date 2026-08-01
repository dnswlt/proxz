package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Site is one configured Atlassian Data Center instance.
type Site struct {
	BaseURL string `json:"base_url"`
	// Token is scrambled at rest; see secret.go. Use Site.PAT() to read it.
	Token string `json:"token"`
	// AllowedPrefixes restricts which request paths may be fetched for this
	// site. Empty means defaultAllowedPrefixes.
	AllowedPrefixes []string `json:"allowed_prefixes,omitempty"`
}

// defaultAllowedPrefixes covers the REST APIs of Jira, Confluence and
// Bitbucket Data Center. Anything outside these is refused, so a stray path
// cannot turn proxz into a general-purpose authenticated fetcher for the host.
var defaultAllowedPrefixes = []string{"/rest/"}

func (s *Site) prefixes() []string {
	if len(s.AllowedPrefixes) == 0 {
		return defaultAllowedPrefixes
	}
	return s.AllowedPrefixes
}

// PAT returns the unscrambled personal access token.
func (s *Site) PAT() (string, error) {
	return unscramble(s.Token)
}

// Config is the on-disk configuration.
type Config struct {
	Sites map[string]*Site `json:"sites"`
}

func (c *Config) siteNames() []string {
	names := make([]string, 0, len(c.Sites))
	for name := range c.Sites {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// site looks up a configured site, with an error listing what is available.
func (c *Config) site(name string) (*Site, error) {
	s, ok := c.Sites[name]
	if !ok {
		if len(c.Sites) == 0 {
			return nil, fmt.Errorf("no sites configured; run: proxz login %s <url>", name)
		}
		return nil, fmt.Errorf("unknown site %q; configured sites: %s",
			name, strings.Join(c.siteNames(), ", "))
	}
	return s, nil
}

// configPath resolves the config location, honouring PROXZ_CONFIG and
// XDG_CONFIG_HOME before falling back to ~/.config/proxz/config.json.
func configPath() (string, error) {
	if p := os.Getenv("PROXZ_CONFIG"); p != "" {
		return p, nil
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "proxz", "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating home directory: %w", err)
	}
	return filepath.Join(home, ".config", "proxz", "config.json"), nil
}

// loadConfig reads the config file. A missing file yields an empty config so
// that `proxz login` works on a fresh machine.
func loadConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return &Config{Sites: map[string]*Site{}}, nil
	}
	if err != nil {
		return nil, err
	}
	// Refuse to use a config that anyone but the owner can read. Silently
	// carrying on would defeat the point of scrambling the tokens.
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		return nil, fmt.Errorf("%s is readable by others (mode %#o); run: chmod 600 %s", path, mode, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if cfg.Sites == nil {
		cfg.Sites = map[string]*Site{}
	}
	return &cfg, nil
}

// save writes the config atomically with owner-only permissions.
func (c *Config) save() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	// Write to a temp file in the same directory, then rename, so a crash
	// cannot leave a half-written config behind.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
