package core

import "testing"

func TestMergeHostCollectionOverlay(t *testing.T) {
	local := &HostCollectionOverlay{SystemInterval: "30s", FilesInterval: "60s"}
	remote := &HostCollectionOverlay{SystemInterval: "10s"}
	merged := MergeHostCollectionOverlay(local, remote)
	if merged.SystemInterval != "10s" || merged.FilesInterval != "60s" {
		t.Fatalf("merge: %+v", merged)
	}
}

func TestParseRuntimeConfig_wrapsRuntimeConfig(t *testing.T) {
	raw := []byte(`{
		"runtime_config": {
			"collection": {"system_interval": "5s"},
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
	if runtime.Collection == nil || runtime.Collection.SystemInterval != "5s" {
		t.Fatalf("collection: %+v", runtime.Collection)
	}
	if len(runtime.Files) != 1 || runtime.Files[0].Path != "/etc/hosts" {
		t.Fatalf("files: %+v", runtime.Files)
	}
}

func TestParseRuntimeConfig_dataEntitiesOnly(t *testing.T) {
	raw := []byte(`{
		"data": {"entities": [{"name": "pages", "refresh_interval": "15m"}]},
		"config_version": "v2"
	}`)
	runtime, version, err := ParseRuntimeConfig(raw)
	if err != nil || version != "v2" {
		t.Fatalf("err=%v version=%q", err, version)
	}
	if len(runtime.Data.Entities) != 1 || runtime.Data.Entities[0].Name != "pages" {
		t.Fatalf("entities: %+v", runtime.Data.Entities)
	}
	if runtime.Collection != nil {
		t.Fatalf("core must not derive collection from entities: %+v", runtime.Collection)
	}
}
