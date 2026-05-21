package core

// Client represents any API client that entities can use
// This allows the core to be generic while agents provide their specific client type
type Client interface{}

// Entity defines the interface for all entities that can be refreshed
type Entity interface {
	// Name returns the unique name of the entity
	Name() string

	// Refresh retrieves the latest data for this entity
	// The client parameter will be cast to the specific client type by the entity implementation
	Refresh(client Client) (interface{}, error)

	// Save saves the entity data to the state manager
	Save(stateManager StateManager, data interface{}) error
}

// StateManager defines the interface for state management
type StateManager interface {
	// SaveEntity saves entity data with timestamped and latest versions
	SaveEntity(entityName string, data interface{}) error

	// Stop stops the state manager (cleanup goroutines, etc.)
	Stop()
}
