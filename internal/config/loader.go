package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadFile reads and parses a single YAML mock definition file.
func LoadFile(path string) (*MockFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file %q: %w", path, err)
	}

	var mf MockFile
	if err := yaml.Unmarshal(data, &mf); err != nil {
		return nil, fmt.Errorf("parsing file %q: %w", path, err)
	}

	return &mf, nil
}

// LoadFiles loads one or more YAML files and merges them into a single
// MockConfig. Defaults are taken from the first file only.
func LoadFiles(paths ...string) (*MockConfig, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("no files provided")
	}

	files := make([]*MockFile, 0, len(paths))
	for _, p := range paths {
		mf, err := LoadFile(p)
		if err != nil {
			return nil, err
		}
		files = append(files, mf)
	}

	return MergeConfigs(files...), nil
}

// MergeConfigs concatenates the routes and scenarios from all provided
// MockFiles into a single MockConfig. Defaults are sourced from the first file.
func MergeConfigs(files ...*MockFile) *MockConfig {
	cfg := &MockConfig{}

	for i, f := range files {
		if f == nil {
			continue
		}
		if i == 0 {
			cfg.Defaults = f.Defaults
		}
		cfg.Routes = append(cfg.Routes, f.Routes...)
		cfg.Scenarios = append(cfg.Scenarios, f.Scenarios...)
	}

	return cfg
}
