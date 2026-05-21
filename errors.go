package core

import "fmt"

var (
	ErrProbeNameMissing     = fmt.Errorf("probe name is missing")
	ErrNoEntitiesConfigured = fmt.Errorf("at least one entity must be configured")
	ErrStateDirMissing      = fmt.Errorf("state directory is missing")
)

func ErrEntityNameMissing(index int) error {
	return fmt.Errorf("entity at index %d has no name", index)
}

func ErrRefreshIntervalRequired(entityName string) error {
	return fmt.Errorf("refresh_interval is required for entity %s", entityName)
}

func ErrUnknownEntity(entityName string) error {
	return fmt.Errorf("unknown entity: %s", entityName)
}
