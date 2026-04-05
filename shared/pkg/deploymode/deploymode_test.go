package deploymode

import (
	"os"
	"testing"
)

func TestFromString(t *testing.T) {
	tests := []struct {
		input   string
		want    Mode
		wantErr bool
	}{
		{"dev", Dev, false},
		{"staging", Staging, false},
		{"production", Production, false},
		{"DEV", Dev, false},
		{"Production", Production, false},
		{" dev ", Dev, false},
		{"invalid", "", true},
		{"prod", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := FromString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("FromString(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("FromString(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		want    Mode
		wantErr bool
	}{
		{"unset defaults to dev", "", Dev, false},
		{"dev mode", "dev", Dev, false},
		{"staging mode", "staging", Staging, false},
		{"production mode", "production", Production, false},
		{"invalid rejects", "invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVal == "" {
				os.Unsetenv(EnvVar)
			} else {
				os.Setenv(EnvVar, tt.envVal)
				defer os.Unsetenv(EnvVar)
			}

			got, err := Parse()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Parse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestModeLogLevel(t *testing.T) {
	if Dev.LogLevel() != "debug" {
		t.Errorf("Dev.LogLevel() = %q, want %q", Dev.LogLevel(), "debug")
	}
	if Staging.LogLevel() != "info" {
		t.Errorf("Staging.LogLevel() = %q, want %q", Staging.LogLevel(), "info")
	}
	if Production.LogLevel() != "info" {
		t.Errorf("Production.LogLevel() = %q, want %q", Production.LogLevel(), "info")
	}
}

func TestModePermissiveCORS(t *testing.T) {
	if !Dev.PermissiveCORS() {
		t.Error("Dev.PermissiveCORS() should be true")
	}
	if !Staging.PermissiveCORS() {
		t.Error("Staging.PermissiveCORS() should be true")
	}
	if Production.PermissiveCORS() {
		t.Error("Production.PermissiveCORS() should be false")
	}
}

func TestModeAcceptSelfSignedTLS(t *testing.T) {
	if !Dev.AcceptSelfSignedTLS() {
		t.Error("Dev.AcceptSelfSignedTLS() should be true")
	}
	if !Staging.AcceptSelfSignedTLS() {
		t.Error("Staging.AcceptSelfSignedTLS() should be true")
	}
	if Production.AcceptSelfSignedTLS() {
		t.Error("Production.AcceptSelfSignedTLS() should be false")
	}
}

func TestModeString(t *testing.T) {
	if Dev.String() != "dev" {
		t.Errorf("Dev.String() = %q, want %q", Dev.String(), "dev")
	}
}

func TestModeHelpers(t *testing.T) {
	if !Dev.IsDev() {
		t.Error("Dev.IsDev() should be true")
	}
	if Dev.IsProduction() {
		t.Error("Dev.IsProduction() should be false")
	}
	if !Production.IsProduction() {
		t.Error("Production.IsProduction() should be true")
	}
}
