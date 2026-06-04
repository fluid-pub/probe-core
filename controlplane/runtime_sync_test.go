package controlplane

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeConfigClient struct {
	mu           sync.Mutex
	registered   bool
	pingStatus   string
	fetchBody    []byte
	fetchVersion string
	pingVersions []string
	fetchCount   int
}

func (f *fakeConfigClient) Register(_, _ string) error {
	f.mu.Lock()
	f.registered = true
	f.mu.Unlock()
	return nil
}
func (f *fakeConfigClient) PushSchema(map[string]interface{}) error { return nil }
func (f *fakeConfigClient) PushState(string, interface{}) error     { return nil }
func (f *fakeConfigClient) IsRegistered() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.registered
}
func (f *fakeConfigClient) Ping() error { _, err := f.PingWithVersion(""); return err }
func (f *fakeConfigClient) PingWithVersion(v string) (string, error) {
	f.mu.Lock()
	f.pingVersions = append(f.pingVersions, v)
	status := f.pingStatus
	f.mu.Unlock()
	if status == "" {
		return PingStatusPong, nil
	}
	return status, nil
}
func (f *fakeConfigClient) FetchConfig() ([]byte, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fetchCount++
	return f.fetchBody, f.fetchVersion, nil
}

func TestRuntimeSync_FetchAndApply(t *testing.T) {
	client := &fakeConfigClient{
		registered:   true,
		fetchBody:    []byte(`{"runtime_config":{"collection":{"system_interval":"5s"}},"config_version":"srv-1"}`),
		fetchVersion: "srv-1",
	}
	sync := NewRuntimeSync(client)
	var applied string
	err := sync.FetchAndApply(func(raw []byte, version string) error {
		applied = version
		return nil
	})
	if err != nil || applied != "srv-1" || sync.GetVersion() != "srv-1" {
		t.Fatalf("apply: err=%v applied=%q version=%q", err, applied, sync.GetVersion())
	}
}

func TestRuntimeSync_HeartbeatConfigurationChanged(t *testing.T) {
	client := &fakeConfigClient{
		registered:   true,
		pingStatus:   PingStatusConfigurationChanged,
		fetchBody:    []byte(`{"runtime_config":{},"config_version":"srv-2"}`),
		fetchVersion: "srv-2",
	}
	sync := NewRuntimeSync(client)
	sync.SetVersion("client-old")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sync.RunHeartbeat(ctx, 20*time.Millisecond, func([]byte, string) error { return nil })
	time.Sleep(80 * time.Millisecond)
	cancel()
	time.Sleep(30 * time.Millisecond)
	if client.fetchCount < 1 {
		t.Fatalf("expected fetch after configuration_changed, count=%d", client.fetchCount)
	}
}
