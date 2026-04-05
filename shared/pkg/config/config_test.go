package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_fileNotFound(t *testing.T) {
	var dst struct{ Name string }
	if err := Load("/nonexistent/path.yaml", &dst); err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
}

func TestLoad_validYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte("name: kite\nport: 8080\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var dst struct {
		Name string `yaml:"name"`
		Port int    `yaml:"port"`
	}
	if err := Load(path, &dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dst.Name != "kite" {
		t.Errorf("expected name=kite, got %s", dst.Name)
	}
	if dst.Port != 8080 {
		t.Errorf("expected port=8080, got %d", dst.Port)
	}
}

func TestLoad_invalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte(":::invalid"), 0644); err != nil {
		t.Fatal(err)
	}

	var dst struct{}
	if err := Load(path, &dst); err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestEnvOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		envKey   string
		envVal   string
		fallback string
		want     string
	}{
		{"env set", "TEST_CONFIG_VAR", "custom", "default", "custom"},
		{"env unset", "TEST_CONFIG_UNSET", "", "default", "default"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVal != "" {
				t.Setenv(tt.envKey, tt.envVal)
			}
			got := EnvOrDefault(tt.envKey, tt.fallback)
			if got != tt.want {
				t.Errorf("EnvOrDefault(%q, %q) = %q, want %q", tt.envKey, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestEnvIntOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		envKey   string
		envVal   string
		fallback int
		want     int
	}{
		{"valid int", "TEST_INT_VAR", "42", 0, 42},
		{"invalid int", "TEST_INT_BAD", "abc", 99, 99},
		{"unset", "TEST_INT_UNSET", "", 10, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVal != "" {
				t.Setenv(tt.envKey, tt.envVal)
			}
			got := EnvIntOrDefault(tt.envKey, tt.fallback)
			if got != tt.want {
				t.Errorf("EnvIntOrDefault(%q, %d) = %d, want %d", tt.envKey, tt.fallback, got, tt.want)
			}
		})
	}
}
