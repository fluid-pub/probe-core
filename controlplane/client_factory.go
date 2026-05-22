package controlplane

import (
	"fmt"
	"log"

	"fluid/probes/core"
)

// RuntimeConfigMerger is implemented by core.MergedConfigProvider.
// Used to avoid core depending on controlplane.
type RuntimeConfigMerger interface {
	SetRemote(runtimeConfig *core.RuntimeConfig, configVersion string) error
}

func NewClientFromConfig(cfg *core.ControlplaneConfig, probeName, probeVersion string, schemaPath ...string) (Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("controlplane configuration is nil")
	}

	if cfg.Parameters == nil {
		return nil, fmt.Errorf("controlplane parameters are required")
	}

	if cfg.Parameters.OrganizationUUID == "" {
		return nil, fmt.Errorf("organization UUID is required")
	}

	if cfg.Parameters.Token == "" {
		return nil, fmt.Errorf("token is required")
	}

	baseURL, err := ParseBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}

	client, err := NewHTTPClient(
		baseURL,
		cfg.Parameters.OrganizationUUID,
		cfg.Parameters.Token,
		probeName,
		probeVersion,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
	}

	if err := client.Register(probeName, probeVersion); err != nil {
		return nil, fmt.Errorf("failed to register: %w", err)
	}

	pushSchema(client, schemaPath, probeName, probeVersion)

	log.Printf("Successfully connected to controlplane at %s (HTTP)", baseURL)

	return client, nil
}

func pushSchema(client Client, schemaPath []string, probeName, probeVersion string) {
	if len(schemaPath) > 0 && schemaPath[0] != "" {
		schema, err := core.LoadSchema(schemaPath[0])
		if err != nil {
			log.Printf("Warning: failed to load schema from %s: %v", schemaPath[0], err)
		} else if err := client.PushSchema(schema.ToMap()); err != nil {
			log.Printf("Warning: failed to push schema: %v", err)
		}
		return
	}
	schema, err := core.LoadSchema()
	if err != nil {
		log.Printf("Warning: schema file not found, skipping schema push: %v", err)
	} else if err := client.PushSchema(schema.ToMap()); err != nil {
		log.Printf("Warning: failed to push schema: %v", err)
	}
}

// FetchAndMergeConfig fetches runtime config from the controlplane and merges it
// into the given merger (e.g. *core.MergedConfigProvider). Call at startup after
// Connect/Join so the probe starts with CP config if available.
func FetchAndMergeConfig(merger RuntimeConfigMerger, client Client) {
	data, configVersion, err := client.FetchConfig()
	if err != nil {
		log.Printf("Fetch config at startup failed (using local config only): %v", err)
		return
	}
	if len(data) == 0 {
		return
	}
	runtime, version, err := core.ParseRuntimeConfig(data)
	if err != nil {
		log.Printf("Parse runtime config failed: %v", err)
		return
	}
	if version != "" {
		configVersion = version
	}
	if err := merger.SetRemote(runtime, configVersion); err != nil {
		log.Printf("Set remote config failed: %v", err)
		return
	}
	log.Printf("Merged runtime config from controlplane (version=%q)", configVersion)
}
