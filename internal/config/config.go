package config

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

const DefaultURL = "http://localhost:8080"

// File is $XDG_CONFIG_HOME/owui/config.toml. Mode 0600 because it holds a key.
type File struct {
	URL             string   `toml:"url"`
	APIKey          string   `toml:"api_key,omitempty"`
	Token           string   `toml:"token,omitempty"`
	UserID          string   `toml:"user_id,omitempty"`
	Model           string   `toml:"model,omitempty"`
	WebSearch       bool     `toml:"web_search"`
	CodeInterpreter bool     `toml:"code_interpreter"`
	ImageGeneration bool     `toml:"image_generation"`
	Memory          bool     `toml:"memory"`
	ToolIDs         []string `toml:"tool_ids,omitempty"`
	TerminalID      string   `toml:"terminal_id,omitempty"`
	Path            string   `toml:"-"`
}

func DefaultPath() string {
	if p := os.Getenv("OWUI_CONFIG"); p != "" {
		return p
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "owui", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "owui", "config.toml")
	}
	return filepath.Join(home, ".config", "owui", "config.toml")
}

func Load(path string) (File, error) {
	f := File{URL: DefaultURL, Path: path}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return f, nil
		}
		return f, err
	}
	if err := toml.Unmarshal(b, &f); err != nil {
		return f, err
	}
	f.Path = path
	if f.URL == "" {
		f.URL = DefaultURL
	}
	return f, nil
}

// ApplyEnv lets OWUI_URL and OWUI_API_KEY override the file.
func ApplyEnv(f *File) {
	if v := os.Getenv("OWUI_URL"); v != "" {
		f.URL = v
	}
	if v := os.Getenv("OWUI_API_KEY"); v != "" {
		f.APIKey = v
	}
}

// Credential is the bearer token sent on every request.
func (f File) Credential() string {
	if f.APIKey != "" {
		return f.APIKey
	}
	return f.Token
}

func (f File) Save() error {
	if f.Path == "" {
		return errors.New("config path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o700); err != nil {
		return err
	}
	b, err := toml.Marshal(f)
	if err != nil {
		return err
	}
	tmp := f.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.Path)
}
