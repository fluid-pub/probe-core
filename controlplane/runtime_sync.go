package controlplane

import (
	"context"
	"log"
	"sync"
	"time"
)

// RuntimeApplyFunc merges control plane runtime_config JSON into the probe and returns on success.
type RuntimeApplyFunc func(runtimeConfigJSON []byte, configVersion string) error

// RuntimeSync tracks config_version and coordinates fetch + heartbeat-driven reload.
type RuntimeSync struct {
	client  Client
	mu      sync.RWMutex
	version string
}

// NewRuntimeSync creates a sync helper bound to a control plane client.
func NewRuntimeSync(client Client) *RuntimeSync {
	return &RuntimeSync{client: client}
}

// GetVersion returns the last applied config version (empty until first successful apply).
func (s *RuntimeSync) GetVersion() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.version
}

// SetVersion records the applied config version (used after startup fetch or reload).
func (s *RuntimeSync) SetVersion(version string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.version = version
}

// FetchAndApply loads runtime config from GET /probes/config and calls apply.
// No-op when the endpoint returns an empty runtime_config.
func (s *RuntimeSync) FetchAndApply(apply RuntimeApplyFunc) error {
	raw, version, err := s.client.FetchConfig()
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	if err := apply(raw, version); err != nil {
		return err
	}
	if version != "" {
		s.SetVersion(version)
	}
	return nil
}

// RunHeartbeat periodically pings with config_version and refetches when the server reports
// configuration_changed.
func (s *RuntimeSync) RunHeartbeat(ctx context.Context, interval time.Duration, apply RuntimeApplyFunc) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pingOnce(apply)
		}
	}
}

func (s *RuntimeSync) pingOnce(apply RuntimeApplyFunc) {
	if !s.client.IsRegistered() {
		return
	}
	status, err := s.client.PingWithVersion(s.GetVersion())
	if err != nil {
		log.Printf("config heartbeat ping failed: %v", err)
		return
	}
	if status != PingStatusConfigurationChanged {
		return
	}
	if err := s.FetchAndApply(apply); err != nil {
		log.Printf("fetch config after configuration_changed failed: %v", err)
		return
	}
	log.Printf("runtime config reloaded from control plane (version=%q)", s.GetVersion())
}
