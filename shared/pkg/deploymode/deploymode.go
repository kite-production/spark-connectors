// Package deploymode provides SPARK_DEPLOY_MODE parsing and behavior helpers.
//
// Valid modes: dev, staging, production. Default: dev.
// Each mode controls log level and CORS policy per spec §6.7 (FR-20, FR-21, FR-22).
package deploymode

import (
	"fmt"
	"os"
	"strings"
)

// Mode represents the deployment environment.
type Mode string

const (
	Dev        Mode = "dev"
	Staging    Mode = "staging"
	Production Mode = "production"
)

// EnvVar is the environment variable name for the deploy mode.
const EnvVar = "SPARK_DEPLOY_MODE"

var validModes = map[Mode]bool{
	Dev:        true,
	Staging:    true,
	Production: true,
}

// Parse reads SPARK_DEPLOY_MODE from the environment and validates it.
// Returns Dev if the variable is unset or empty.
// Returns an error if the value is not a valid mode.
func Parse() (Mode, error) {
	raw := os.Getenv(EnvVar)
	if raw == "" {
		return Dev, nil
	}
	return FromString(raw)
}

// FromString converts a string to a Mode, returning an error for invalid values.
func FromString(s string) (Mode, error) {
	m := Mode(strings.ToLower(strings.TrimSpace(s)))
	if !validModes[m] {
		return "", fmt.Errorf("invalid %s value %q: must be one of dev, staging, production", EnvVar, s)
	}
	return m, nil
}

// String returns the string representation of the mode.
func (m Mode) String() string {
	return string(m)
}

// LogLevel returns the recommended log level for this mode.
// dev → debug, staging → info, production → info.
func (m Mode) LogLevel() string {
	if m == Dev {
		return "debug"
	}
	return "info"
}

// PermissiveCORS returns true if the mode allows permissive CORS policy.
// dev and staging → true, production → false.
func (m Mode) PermissiveCORS() bool {
	return m != Production
}

// AcceptSelfSignedTLS returns true if self-signed TLS certificates are acceptable.
// dev and staging → true, production → false.
func (m Mode) AcceptSelfSignedTLS() bool {
	return m != Production
}

// IsDev returns true if the mode is Dev.
func (m Mode) IsDev() bool {
	return m == Dev
}

// IsProduction returns true if the mode is Production.
func (m Mode) IsProduction() bool {
	return m == Production
}
