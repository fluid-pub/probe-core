package core

import "testing"

func TestParseRuntimeConfig_wrapsRuntimeConfig(t *testing.T) {
	raw := []byte(`{
		"runtime_config": {
			"data": {"entities": [{"name": "pages", "refresh_interval": "15m"}]}
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
	if len(runtime.Data.Entities) != 1 || runtime.Data.Entities[0].Name != "pages" {
		t.Fatalf("entities: %+v", runtime.Data.Entities)
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
	if len(runtime.Data.Entities) != 1 {
		t.Fatalf("entities: %+v", runtime.Data.Entities)
	}
}
