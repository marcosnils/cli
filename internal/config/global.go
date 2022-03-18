package config

import (
	"errors"
	"fmt"
	"io/fs"
	"io/ioutil"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

type ConfigFS struct {
	fsys fs.FS
}

func NewConfigFS(fsys fs.FS) *ConfigFS {
	return &ConfigFS{
		fsys: fsys,
	}
}

// FileConfig defines a pscale configuration from a file.
type FileConfig struct {
	Organization string `yaml:"org" json:"org"`
	Database     string `yaml:"database,omitempty" json:"database,omitempty"`
	Branch       string `yaml:"branch,omitempty" json:"branch,omitempty"`
}

// NewFileConfig reads the file config from the designated path and returns a
// new FileConfig.
func (c *ConfigFS) NewFileConfig(path string) (*FileConfig, error) {
	out, err := fs.ReadFile(c.fsys, path)
	if err != nil {
		return nil, err
	}

	var cfg FileConfig
	err = yaml.Unmarshal(out, &cfg)
	if err != nil {
		return nil, fmt.Errorf("can't unmarshal file %q: %s", path, err)
	}

	return &cfg, nil
}

// DefaultConfig returns the file config from the default config path.
func (c *ConfigFS) DefaultConfig() (*FileConfig, error) {
	configFile, err := DefaultConfigPath()
	if err != nil {
		return nil, err
	}
	return c.NewFileConfig(configFile)
}

// ProjectConfig returns the file config from the git project
func (c *ConfigFS) ProjectConfig() (*FileConfig, error) {
	configFile, err := ProjectConfigPath()
	if err != nil {
		return nil, err
	}
	return c.NewFileConfig(configFile)
}

// Write persists the file config at the designated path.
func (f *FileConfig) Write(path string) error {
	if path == "" {
		return errors.New("path is empty")
	}

	if f.Organization == "" {
		return errors.New("fileconfig.Organization must be set")
	}

	d, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("can't marshal file config: %s", err)
	}

	return ioutil.WriteFile(path, d, 0644)
}

// WriteDefault persists the file config to the default global path.
func (f *FileConfig) WriteDefault() error {
	configFile, err := DefaultConfigPath()
	if err != nil {
		return err
	}

	return f.Write(configFile)
}

// WriteProject persists the file config at the default path which is pulled
// from the root of the git repository if a user is in one.
func (f *FileConfig) WriteProject() error {
	cfgFile, err := ProjectConfigPath()
	if err != nil {
		return err
	}

	return f.Write(cfgFile)
}

// DefaultConfigPath returns the default path for the config file.
func DefaultConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, configName), nil
}
