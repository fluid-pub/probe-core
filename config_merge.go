package core

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
)

// RuntimeConfig is the configuration overlay from the controlplane (JSON).
// Only fields present here override the local config; rest stays from local.
type RuntimeConfig struct {
	Data *struct {
		Entities []EntityConfig `json:"entities"`
	} `json:"data,omitempty"`
	// Host-native probes (e.g. Debian): optional collection, file, and directory overrides.
	Collection  *HostCollectionOverlay `json:"collection,omitempty"`
	Files       []HostFileRule         `json:"files,omitempty"`
	Directories []HostDirectoryRule    `json:"directories,omitempty"`
}

// MergedConfigProvider implements ProbeConfigProvider by merging local config
// with optional runtime_config from the controlplane. Safe for concurrent read;
// call SetRemote to update the overlay and config_version.
type MergedConfigProvider struct {
	mu            sync.RWMutex
	local         ProbeConfigProvider
	runtime       *RuntimeConfig
	configVersion string
	// ragFieldAllowlist: when set, SetRemote validates fields.*.rag against the schema (usable_in_rag).
	ragFieldAllowlist RAGFieldSet
}

// NewMergedConfigProvider wraps a local config provider. Without SetRemote,
// it behaves like the local config only.
func NewMergedConfigProvider(local ProbeConfigProvider) *MergedConfigProvider {
	return &MergedConfigProvider{local: local}
}

// SetRAGFieldAllowlist sets allowed fields for fields.<name>.rag (from schema.yml).
// Call before SetRemote / FetchAndMergeConfig when the control plane may push runtime config.
func (m *MergedConfigProvider) SetRAGFieldAllowlist(allowed RAGFieldSet) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ragFieldAllowlist = allowed
}

// SetRemote updates the runtime overlay and config version from the controlplane.
// runtimeConfig can be nil to clear the overlay. Merged entities are validated
// (name and refresh_interval required).
func (m *MergedConfigProvider) SetRemote(runtimeConfig *RuntimeConfig, configVersion string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if runtimeConfig != nil && runtimeConfig.Data != nil {
		for i, e := range runtimeConfig.Data.Entities {
			if e.Name == "" {
				return fmt.Errorf("runtime entity at index %d has no name", i)
			}
			if e.RefreshInterval == "" {
				return fmt.Errorf("refresh_interval is required for entity %s", e.Name)
			}
		}
	}

	var active []EntityConfig
	if runtimeConfig != nil && runtimeConfig.Data != nil && len(runtimeConfig.Data.Entities) > 0 {
		active = runtimeConfig.Data.Entities
	} else {
		active = m.local.GetEntities()
	}
	if m.ragFieldAllowlist != nil {
		if err := ValidateRAGEntityFields(active, m.ragFieldAllowlist); err != nil {
			return err
		}
	}

	m.runtime = runtimeConfig
	m.configVersion = configVersion
	return nil
}

// GetConfigVersion returns the last applied config version from the controlplane.
func (m *MergedConfigProvider) GetConfigVersion() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.configVersion
}

// Local returns the underlying local config provider (e.g. for probe-specific settings).
func (m *MergedConfigProvider) Local() ProbeConfigProvider {
	return m.local
}

func (m *MergedConfigProvider) GetProbeName() string {
	return m.local.GetProbeName()
}

func (m *MergedConfigProvider) GetProbeVersion() string {
	return m.local.GetProbeVersion()
}

func (m *MergedConfigProvider) GetStateDir() string {
	return m.local.GetStateDir()
}

func (m *MergedConfigProvider) GetCleanupInterval() int {
	return m.local.GetCleanupInterval()
}

// GetEntities returns local entities, or runtime overlay if set.
func (m *MergedConfigProvider) GetEntities() []EntityConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.runtime != nil && m.runtime.Data != nil && len(m.runtime.Data.Entities) > 0 {
		return m.runtime.Data.Entities
	}
	return m.local.GetEntities()
}

// GetControlplane returns the controlplane config from local (never overridden by runtime).
// Implements state.ConfigProvider when local supports it.
func (m *MergedConfigProvider) GetControlplane() *ControlplaneConfig {
	if h, ok := m.local.(interface{ GetControlplane() *ControlplaneConfig }); ok {
		return h.GetControlplane()
	}
	return nil
}

var _ ProbeConfigProvider = (*MergedConfigProvider)(nil)

// ParseRuntimeConfig parses the JSON response from the config API into RuntimeConfig.
// The API can return { "runtime_config": { ... }, "config_version": "..." } or
// just { "data": { "entities": [...] } }.
func ParseRuntimeConfig(data []byte) (runtime *RuntimeConfig, configVersion string, err error) {
	var raw struct {
		RuntimeConfig *RuntimeConfig `json:"runtime_config"`
		ConfigVersion string         `json:"config_version"`
		Data          *struct {
			Entities []EntityConfig `json:"entities"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, "", fmt.Errorf("parse runtime config: %w", err)
	}
	configVersion = raw.ConfigVersion
	if raw.RuntimeConfig != nil {
		runtime := raw.RuntimeConfig
		if runtime.Collection == nil && runtime.Data != nil && len(runtime.Data.Entities) > 0 {
			overlay := &HostCollectionOverlay{}
			ApplyEntityIntervalsToCollection(overlay, runtime.Data.Entities)
			if overlay.SystemInterval != "" || overlay.FilesInterval != "" || overlay.APTInterval != "" ||
				overlay.InstalledPackagesInterval != "" || overlay.ServicesInterval != "" {
				runtime.Collection = overlay
			}
		}
		return runtime, configVersion, nil
	}
	if raw.Data != nil {
		runtime := &RuntimeConfig{Data: raw.Data}
		overlay := &HostCollectionOverlay{}
		ApplyEntityIntervalsToCollection(overlay, raw.Data.Entities)
		if overlay.SystemInterval != "" || overlay.FilesInterval != "" || overlay.APTInterval != "" ||
			overlay.InstalledPackagesInterval != "" || overlay.ServicesInterval != "" {
			runtime.Collection = overlay
		}
		return runtime, configVersion, nil
	}
	return nil, configVersion, nil
}

// ApplyRemoteToBase merges runtime_config into a copy of BaseConfig and returns
// a provider for that merged base. Used when the local config is *BaseConfig
// (e.g. from YAML). If runtimeConfig is nil or has no entities, returns
// BaseProbeConfigProvider(local).
func ApplyRemoteToBase(local *BaseConfig, runtimeConfig *RuntimeConfig, configVersion string) (ProbeConfigProvider, error) {
	if runtimeConfig == nil || runtimeConfig.Data == nil || len(runtimeConfig.Data.Entities) == 0 {
		return &BaseProbeConfigProvider{BaseConfig: local}, nil
	}
	merged := &BaseConfig{
		Probe:        local.Probe,
		State:        local.State,
		Data:         DataConfig{Entities: runtimeConfig.Data.Entities},
		Controlplane: local.Controlplane,
	}
	_ = configVersion
	return &BaseProbeConfigProvider{BaseConfig: merged}, nil
}

// MergedConfigProviderFromBase creates a MergedConfigProvider that uses
// local as base. For state manager (GetControlplane), use NewMergedConfigProvider
// with a local that implements GetControlplane (e.g. state.BaseConfigProvider).
func MergedConfigProviderFromBase(local *BaseConfig) *MergedConfigProvider {
	return NewMergedConfigProvider(&BaseProbeConfigProvider{BaseConfig: local})
}

// LogMergeSummary logs what config is active (local vs merged) for debugging.
func LogMergeSummary(provider ProbeConfigProvider) {
	if m, ok := provider.(*MergedConfigProvider); ok {
		v := m.GetConfigVersion()
		entities := m.GetEntities()
		log.Printf("Config: merged (config_version=%q), entities=%d", v, len(entities))
	} else {
		entities := provider.GetEntities()
		log.Printf("Config: local only, entities=%d", len(entities))
	}
}
