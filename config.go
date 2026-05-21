package core

import (
	"strings"
	"time"
)

// BaseConfig contains the core configuration that all agents share
type BaseConfig struct {
	Probe        ProbeConfig
	State        StateConfig
	Data         DataConfig
	Controlplane *ControlplaneConfig `yaml:"controlplane,omitempty"`
}

// ProbeConfig contains probe metadata
type ProbeConfig struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

// StateConfig contains state management configuration
type StateConfig struct {
	Dir             string `yaml:"dir"`
	Format          string `yaml:"format"`
	CleanupInterval int    `yaml:"cleanup_interval,omitempty"` // Frequency in minutes to run cleanup
}

// DataConfig contains data collection configuration
type DataConfig struct {
	Entities []EntityConfig `yaml:"entities"`
}

// EntityFieldConfig holds per-schema-field options (e.g. rag on body for Confluence).
type EntityFieldConfig struct {
	RAG bool `yaml:"rag,omitempty" json:"rag,omitempty"`
}

// EntityConfig contains configuration for a single entity
type EntityConfig struct {
	Name            string `yaml:"name" json:"name"`
	RefreshInterval string `yaml:"refresh_interval" json:"refresh_interval"` // Required: refresh interval for this entity
	// Fields sets options per schema field name (e.g. fields.body.rag: true).
	Fields    map[string]EntityFieldConfig `yaml:"fields,omitempty" json:"fields,omitempty"`
	Retention *RetentionConfig             `yaml:"retention_frequencies,omitempty" json:"retention_frequencies,omitempty"` // Optional, per-entity retention configuration
}

// FieldRAG reports whether RAG is enabled for fieldName on this entity.
func (e *EntityConfig) FieldRAG(fieldName string) bool {
	if e == nil || e.Fields == nil {
		return false
	}
	fc, ok := e.Fields[fieldName]
	return ok && fc.RAG
}

// RetentionConfig defines retention frequencies for state files
type RetentionConfig struct {
	Seconds *int `yaml:"seconds,omitempty" json:"seconds,omitempty"` // 0 = unlimited, nil = not configured
	Minutes *int `yaml:"minutes,omitempty" json:"minutes,omitempty"`
	Hours   *int `yaml:"hours,omitempty" json:"hours,omitempty"`
	Days    *int `yaml:"days,omitempty" json:"days,omitempty"`
	Weeks   *int `yaml:"weeks,omitempty" json:"weeks,omitempty"`
	Months  *int `yaml:"months,omitempty" json:"months,omitempty"`
	Years   *int `yaml:"years,omitempty" json:"years,omitempty"`
}

// ControlplaneParameters contains controlplane connection parameters
type ControlplaneParameters struct {
	OrganizationUUID string `yaml:"organization_uuid"`
	Token            string `yaml:"token"`
}

// ControlplaneConfig contains controlplane connection configuration
type ControlplaneConfig struct {
	WebSocketURL      string                  `yaml:"websocket_url"`
	APIVersion        string                  `yaml:"api_version,omitempty"`
	Parameters        *ControlplaneParameters `yaml:"parameters"`
	QueueSize         int     `yaml:"queue_size,omitempty"`         // Default: 100
	MaxRetries        int     `yaml:"max_retries,omitempty"`        // Default: 3
	RetryBackoff      string  `yaml:"retry_backoff,omitempty"`      // Default: "1s"
	HeartbeatInterval string  `yaml:"heartbeat_interval,omitempty"` // Default: "30s"
}

// ControlplaneConnectionRequested reports whether the resolved configuration
// requests a WebSocket connection to the control plane (URL + organization + token).
// When true, startup must fail if that connection cannot be established.
func ControlplaneConnectionRequested(cp *ControlplaneConfig) bool {
	if cp == nil {
		return false
	}
	if strings.TrimSpace(cp.WebSocketURL) == "" || cp.Parameters == nil {
		return false
	}
	return strings.TrimSpace(cp.Parameters.OrganizationUUID) != "" &&
		strings.TrimSpace(cp.Parameters.Token) != ""
}

// GetRefreshInterval returns the refresh interval as a time.Duration
func (e *EntityConfig) GetRefreshInterval() (time.Duration, error) {
	if e.RefreshInterval == "" {
		return 0, ErrRefreshIntervalRequired(e.Name)
	}
	return time.ParseDuration(e.RefreshInterval)
}

// ValidateBaseConfig validates the base configuration
func ValidateBaseConfig(cfg *BaseConfig) error {
	if cfg.Probe.Name == "" {
		return ErrProbeNameMissing
	}

	if len(cfg.Data.Entities) == 0 {
		return ErrNoEntitiesConfigured
	}

	for i, entity := range cfg.Data.Entities {
		if entity.Name == "" {
			return ErrEntityNameMissing(i)
		}
	}

	if cfg.State.Dir == "" {
		return ErrStateDirMissing
	}

	// Set default cleanup interval if not specified
	if cfg.State.CleanupInterval <= 0 {
		cfg.State.CleanupInterval = 1 // Default: 1 minute
	}

	return nil
}
