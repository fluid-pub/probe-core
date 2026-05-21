package controlplane

// PingStatus is the status returned by the controlplane in response to a ping.
const (
	PingStatusPong                 = "pong"
	PingStatusConfigurationChanged = "configuration_changed"
)

// Client defines the interface for controlplane communication
type Client interface {
	// Register registers the probe with the controlplane
	Register(probeName, probeVersion string) error

	// PushSchema pushes the probe's schema to the controlplane
	PushSchema(schemaData map[string]interface{}) error

	// PushState pushes entity state to the controlplane
	PushState(entityName string, stateData interface{}) error

	// IsRegistered returns whether the probe is currently registered
	IsRegistered() bool

	// Ping sends a heartbeat ping to the controlplane (fire-and-forget).
	Ping() error

	// PingWithVersion sends a heartbeat with config_version and returns the response status
	// (pong or configuration_changed). Used to detect when the probe should fetch config.
	PingWithVersion(configVersion string) (status string, err error)

	// FetchConfig fetches runtime config from the dedicated config endpoint.
	// Returns nil runtimeConfig and empty version if the endpoint returns no config.
	FetchConfig() (runtimeConfigJSON []byte, configVersion string, err error)
}
