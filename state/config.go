package state

import (
	"fluid/probes/core"
)

// ConfigProvider provides access to state configuration
type ConfigProvider interface {
	GetStateDir() string
	GetCleanupInterval() int
	GetEntities() []core.EntityConfig
	GetControlplane() *core.ControlplaneConfig
	GetProbeName() string
	GetProbeVersion() string
}

// BaseConfigProvider is a helper struct that implements ConfigProvider using core.BaseConfig
type BaseConfigProvider struct {
	*core.BaseConfig
}

// GetStateDir returns the state directory
func (p *BaseConfigProvider) GetStateDir() string {
	return p.State.Dir
}

// GetCleanupInterval returns the cleanup interval in minutes
func (p *BaseConfigProvider) GetCleanupInterval() int {
	return p.State.CleanupInterval
}

// GetEntities returns the entity configurations
func (p *BaseConfigProvider) GetEntities() []core.EntityConfig {
	return p.Data.Entities
}

// GetControlplane returns the controlplane configuration
func (p *BaseConfigProvider) GetControlplane() *core.ControlplaneConfig {
	return p.Controlplane
}

// GetProbeName returns the probe name
func (p *BaseConfigProvider) GetProbeName() string {
	return p.Probe.Name
}

// GetProbeVersion returns the probe version
func (p *BaseConfigProvider) GetProbeVersion() string {
	return p.Probe.Version
}
