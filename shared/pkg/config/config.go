// Package config provides a shared configuration loader that reads YAML files
// and merges with environment variable overrides.
package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Load reads a YAML config file into dst. If the file does not exist,
// dst is left unchanged (callers should pre-populate defaults).
func Load(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("parsing config %s: %w", path, err)
	}
	return nil
}

// EnvOrDefault returns the value of the named environment variable,
// or fallback if the variable is unset or empty.
func EnvOrDefault(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// EnvIntOrDefault returns the integer value of the named environment variable,
// or fallback if the variable is unset, empty, or not a valid integer.
func EnvIntOrDefault(name string, fallback int) int {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
