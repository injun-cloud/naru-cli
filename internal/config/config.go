// Package config handles the CLI's global config (~/.naru/config.yaml), env
// overrides, and the directory-local project link (.naru).
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Global is the user-level config at ~/.naru/config.yaml (mode 0600).
type Global struct {
	ServerURL string `yaml:"server_url"`
	Token     string `yaml:"token"`
	Username  string `yaml:"username"`
}

func globalPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".naru", "config.yaml"), nil
}

// LoadGlobal reads the global config (empty config if the file is absent).
func LoadGlobal() (*Global, error) {
	path, err := globalPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Global{}, nil
	}
	if err != nil {
		return nil, err
	}
	var g Global
	if err := yaml.Unmarshal(data, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

// SaveGlobal writes the global config with 0600 permissions.
func SaveGlobal(g *Global) error {
	path, err := globalPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := yaml.Marshal(g)
	if err != nil {
		return err
	}
	// Atomic write (temp file in the same dir + rename) so a crash mid-write can't
	// truncate the config, which holds the auth token.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
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

// linkFile is the directory-local project link.
type linkFile struct {
	Project string `yaml:"project"`
}

// LinkedProject returns the project from a .naru file in the current directory, if any.
func LinkedProject() string {
	data, err := os.ReadFile(".naru")
	if err != nil {
		return ""
	}
	var l linkFile
	if yaml.Unmarshal(data, &l) != nil {
		return ""
	}
	return l.Project
}

// SaveLink writes the directory-local project link.
func SaveLink(project string) error {
	data, _ := yaml.Marshal(linkFile{Project: project})
	return os.WriteFile(".naru", data, 0644)
}

// RemoveLink deletes the directory-local .naru link. It returns false (no error)
// when no link exists.
func RemoveLink() (bool, error) {
	if err := os.Remove(".naru"); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Resolve merges file config with env overrides (env wins).
func Resolve() (*Global, error) {
	g, err := LoadGlobal()
	if err != nil {
		return nil, err
	}
	if v := os.Getenv("NARU_SERVER_URL"); v != "" {
		g.ServerURL = v
	}
	if v := os.Getenv("NARU_TOKEN"); v != "" {
		g.Token = v
	}
	if g.ServerURL == "" {
		g.ServerURL = "https://naru.injunweb.com"
	}
	return g, nil
}
