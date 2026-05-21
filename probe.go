package core

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// ProbeConfigProvider provides access to probe configuration
type ProbeConfigProvider interface {
	GetProbeName() string
	GetProbeVersion() string
	GetStateDir() string
	GetCleanupInterval() int
	GetEntities() []EntityConfig
}

// BaseProbeConfigProvider is a helper struct that implements ProbeConfigProvider using BaseConfig
type BaseProbeConfigProvider struct {
	*BaseConfig
}

// GetProbeName returns the probe name
func (p *BaseProbeConfigProvider) GetProbeName() string {
	return p.Probe.Name
}

// GetProbeVersion returns the probe version
func (p *BaseProbeConfigProvider) GetProbeVersion() string {
	return p.Probe.Version
}

// GetStateDir returns the state directory
func (p *BaseProbeConfigProvider) GetStateDir() string {
	return p.State.Dir
}

// GetCleanupInterval returns the cleanup interval in minutes
func (p *BaseProbeConfigProvider) GetCleanupInterval() int {
	return p.State.CleanupInterval
}

// GetEntities returns the entity configurations
func (p *BaseProbeConfigProvider) GetEntities() []EntityConfig {
	return p.Data.Entities
}

// Probe represents the base probe with common lifecycle management
type Probe struct {
	config         ProbeConfigProvider
	client         Client
	stateManager   StateManager
	entityRegistry *EntityRegistry
	wg             sync.WaitGroup
	entityProcs    map[string]chan struct{}      // Map of entity name to its stop channel
	entityContexts map[string]context.CancelFunc // Map of entity name to its context cancel function
	entityCtxMu    sync.RWMutex                  // Mutex to protect entityContexts
}

// NewProbe creates a new base probe
func NewProbe(config ProbeConfigProvider, client Client, stateManager StateManager) *Probe {
	return &Probe{
		config:         config,
		client:         client,
		stateManager:   stateManager,
		entityRegistry: NewEntityRegistry(),
		entityProcs:    make(map[string]chan struct{}),
		entityContexts: make(map[string]context.CancelFunc),
	}
}

// RegisterEntity registers an entity with the probe
func (a *Probe) RegisterEntity(entity Entity) {
	a.entityRegistry.Register(entity)
}

// Start starts the probe and all entity processes
func (a *Probe) Start() error {
	log.Printf("Starting probe %s v%s", a.config.GetProbeName(), a.config.GetProbeVersion())
	log.Printf("State directory: %s", a.config.GetStateDir())
	log.Printf("State cleanup interval: %d minutes", a.config.GetCleanupInterval())
	return a.startEntityProcesses()
}

// startEntityProcesses starts one goroutine per entity from the current config.
// Caller must ensure config is set and valid.
func (a *Probe) startEntityProcesses() error {
	for _, entityConfig := range a.config.GetEntities() {
		entityName := entityConfig.Name
		if entityName == "" {
			log.Printf("Warning: entity name is empty, skipping")
			continue
		}

		interval, err := entityConfig.GetRefreshInterval()
		if err != nil {
			log.Printf("Error: refresh_interval is required for entity %s: %v", entityName, err)
			return fmt.Errorf("refresh_interval is required for entity %s: %w", entityName, err)
		}

		entityStopChan := make(chan struct{})
		a.entityProcs[entityName] = entityStopChan

		entityCtx, entityCancel := context.WithCancel(context.Background())
		a.entityCtxMu.Lock()
		a.entityContexts[entityName] = entityCancel
		a.entityCtxMu.Unlock()

		log.Printf("Starting process for entity '%s' with refresh interval: %v", entityName, interval)

		a.wg.Add(1)
		go a.entityProcess(entityName, interval, entityStopChan, entityCtx, entityCancel)
	}
	return nil
}

// ReloadConfig stops all entity processes and restarts them using the current config.
// Use after updating a MergedConfigProvider with SetRemote (e.g. on configuration_changed from CP).
func (a *Probe) ReloadConfig() error {
	log.Println("Reloading configuration...")

	// Stop all entity processes (same logic as Stop but without stopping state manager)
	for entityName, stopChan := range a.entityProcs {
		log.Printf("Stopping process for entity: %s", entityName)
		a.entityCtxMu.RLock()
		if cancel, exists := a.entityContexts[entityName]; exists {
			cancel()
		}
		a.entityCtxMu.RUnlock()
		select {
		case <-stopChan:
		default:
			close(stopChan)
		}
	}

	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		log.Println("Timeout waiting for entity processes to stop during reload")
	}

	// Clear maps so startEntityProcesses can repopulate
	a.entityProcs = make(map[string]chan struct{})
	a.entityCtxMu.Lock()
	a.entityContexts = make(map[string]context.CancelFunc)
	a.entityCtxMu.Unlock()

	return a.startEntityProcesses()
}

// Stop stops the probe and all entity processes gracefully
func (a *Probe) Stop() {
	log.Println("Stopping probe...")

	// Cancel all entity contexts and stop all entity processes
	for entityName, stopChan := range a.entityProcs {
		log.Printf("Stopping process for entity: %s", entityName)

		// Cancel the context for this entity
		a.entityCtxMu.RLock()
		if cancel, exists := a.entityContexts[entityName]; exists {
			cancel()
		}
		a.entityCtxMu.RUnlock()

		select {
		case <-stopChan:
			// Channel already closed
		default:
			close(stopChan)
		}
	}

	// Wait for all processes to finish with a timeout
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("All entity processes stopped gracefully.")
	case <-time.After(5 * time.Second):
		log.Println("Timeout waiting for processes to stop. Some processes may still be running.")
	}

	// Stop state manager cleanup goroutine
	a.stateManager.Stop()

	log.Println("Probe stopped.")
}

// entityProcess runs the refresh loop for a single entity
func (a *Probe) entityProcess(entityName string, interval time.Duration, stopChan chan struct{}, entityCtx context.Context, entityCancel context.CancelFunc) {
	defer a.wg.Done()
	defer entityCancel()

	log.Printf("Entity process '%s' started with interval %v", entityName, interval)

	// First execution immediately
	if err := a.refreshEntity(entityName, entityCtx); err != nil {
		log.Printf("Error during first refresh for entity %s: %v", entityName, err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := a.refreshEntity(entityName, entityCtx); err != nil {
				log.Printf("Error refreshing entity %s: %v", entityName, err)
			}
		case <-stopChan:
			log.Printf("Entity process '%s' stopped", entityName)
			return
		case <-entityCtx.Done():
			log.Printf("Entity process '%s' context cancelled", entityName)
			return
		}
	}
}

// refreshEntity refreshes a single entity
// The context can be used by entity implementations to cancel long-running operations
func (a *Probe) refreshEntity(entityName string, ctx context.Context) error {
	log.Printf("Refreshing entity: %s", entityName)

	// Get entity from registry
	entity, exists := a.entityRegistry.Get(entityName)
	if !exists {
		return ErrUnknownEntity(entityName)
	}

	// Set context on client if it supports it (for cancellation)
	// This allows AWS client to use the cancellable context
	if contextSetter, ok := a.client.(interface{ SetContext(context.Context) }); ok {
		contextSetter.SetContext(ctx)
	}

	// Refresh entity data
	// Note: The context is passed via the client interface, but entity implementations
	// should handle it appropriately (e.g., AWS client methods accept context)
	data, err := entity.Refresh(a.client)
	if err != nil {
		// Check if error is due to context cancellation
		if ctx.Err() != nil {
			return fmt.Errorf("entity %s refresh cancelled: %w", entityName, ctx.Err())
		}
		return fmt.Errorf("error refreshing entity %s: %w", entityName, err)
	}

	// Save entity data
	if err := entity.Save(a.stateManager, data); err != nil {
		return fmt.Errorf("error saving entity %s: %w", entityName, err)
	}

	return nil
}

// GetStatus returns the current status of the probe
func (a *Probe) GetStatus() map[string]interface{} {
	entityStatuses := make(map[string]interface{})
	for entityName := range a.entityProcs {
		entityStatuses[entityName] = map[string]interface{}{
			"running": true,
		}
	}

	return map[string]interface{}{
		"name":            a.config.GetProbeName(),
		"version":         a.config.GetProbeVersion(),
		"state_directory": a.config.GetStateDir(),
		"running":         true,
		"entities":        entityStatuses,
	}
}
