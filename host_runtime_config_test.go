package core

import "testing"

func TestApplyEntityIntervalsToCollection(t *testing.T) {
	overlay := &HostCollectionOverlay{}
	entities := []EntityConfig{
		{Name: EntityDebianSystemMetrics, RefreshInterval: "10s"},
		{Name: EntityDebianFileChecks, RefreshInterval: "60s"},
		{Name: EntityDebianPackageUpdates, RefreshInterval: "30m"},
	}
	ApplyEntityIntervalsToCollection(overlay, entities)
	if overlay.SystemInterval != "10s" {
		t.Fatalf("system_interval: got %q", overlay.SystemInterval)
	}
	if overlay.FilesInterval != "60s" {
		t.Fatalf("files_interval: got %q", overlay.FilesInterval)
	}
	if overlay.APTInterval != "30m" {
		t.Fatalf("apt_interval: got %q", overlay.APTInterval)
	}
}

func TestParseRuntimeConfig_entitiesMapToCollection(t *testing.T) {
	raw := []byte(`{
		"runtime_config": {
			"data": {
				"entities": [
					{"name": "debian_system_metrics", "refresh_interval": "15s"},
					{"name": "debian_file_checks", "refresh_interval": "45s"}
				]
			},
			"files": [{"path": "/etc/hosts"}]
		},
		"config_version": "v1"
	}`)
	runtime, version, err := ParseRuntimeConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if version != "v1" {
		t.Fatalf("version: got %q", version)
	}
	if runtime.Collection == nil || runtime.Collection.SystemInterval != "15s" {
		t.Fatalf("collection: %+v", runtime.Collection)
	}
	if runtime.Collection.FilesInterval != "45s" {
		t.Fatalf("files_interval: %q", runtime.Collection.FilesInterval)
	}
	if len(runtime.Files) != 1 || runtime.Files[0].Path != "/etc/hosts" {
		t.Fatalf("files: %+v", runtime.Files)
	}
}

func TestParseRuntimeConfig_explicitCollectionWins(t *testing.T) {
	raw := []byte(`{
		"runtime_config": {
			"collection": {"system_interval": "5s"},
			"data": {"entities": [{"name": "debian_system_metrics", "refresh_interval": "99s"}]}
		}
	}`)
	runtime, _, err := ParseRuntimeConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Collection.SystemInterval != "5s" {
		t.Fatalf("expected explicit collection, got %+v", runtime.Collection)
	}
}
