package controlplane

import (
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
	"time"
)

// StateUpdate represents a state update to be pushed
type StateUpdate struct {
	EntityName string
	Data       interface{}
	Timestamp  time.Time
	Retries    int
}

// PushConfig contains configuration for the push manager
type PushConfig struct {
	QueueSize         int
	MaxRetries        int
	RetryBackoff      time.Duration
	HeartbeatInterval time.Duration
}

// ConfigVersionFunc returns the probe's current config version (for heartbeat payload).
type ConfigVersionFunc func() string

// ConfigChangedCallback is called when the controlplane reports configuration_changed.
// The probe should merge runtimeConfigJSON into its config and call ReloadConfig.
type ConfigChangedCallback func(runtimeConfigJSON []byte, configVersion string)

// PushManager manages asynchronous state pushing to the controlplane
type PushManager struct {
	queue            chan StateUpdate
	client           Client
	config           PushConfig
	stopChan         chan struct{}
	reconnectChan    chan struct{}
	wg               sync.WaitGroup
	mu               sync.RWMutex
	connected        bool
	lastError        error
	probeName        string
	probeVersion     string
	getConfigVersion ConfigVersionFunc
	onConfigChanged  ConfigChangedCallback
}

// NewPushManager creates a new push manager
func NewPushManager(client Client, config PushConfig, probeName, probeVersion string) *PushManager {
	if config.QueueSize <= 0 {
		config.QueueSize = 100
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.RetryBackoff <= 0 {
		config.RetryBackoff = 1 * time.Second
	}
	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = 30 * time.Second
	}

	return &PushManager{
		queue:         make(chan StateUpdate, config.QueueSize),
		client:        client,
		config:        config,
		stopChan:      make(chan struct{}),
		reconnectChan: make(chan struct{}, 1),
		connected:     false,
		probeName:     probeName,
		probeVersion:  probeVersion,
	}
}

// SetConfigCallbacks sets optional callbacks for config-aware heartbeat.
// When set, heartbeat uses PingWithVersion(getConfigVersion()) and on
// configuration_changed fetches config and calls onConfigChanged.
func (pm *PushManager) SetConfigCallbacks(getVersion ConfigVersionFunc, onChanged ConfigChangedCallback) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.getConfigVersion = getVersion
	pm.onConfigChanged = onChanged
}

// Start starts the push manager goroutines
func (pm *PushManager) Start() error {
	log.Printf("Starting push manager (queue size: %d, max retries: %d)", pm.config.QueueSize, pm.config.MaxRetries)

	// Initial registration
	if err := pm.register(); err != nil {
		log.Printf("Warning: initial registration failed: %v", err)
		log.Printf("Push manager will continue and retry registration")
	}

	// Start push consumer goroutine
	pm.wg.Add(1)
	go pm.pushLoop()

	// Start heartbeat/reconnection goroutine
	pm.wg.Add(1)
	go pm.heartbeatLoop()

	return nil
}

// Stop stops the push manager gracefully
func (pm *PushManager) Stop() {
	log.Printf("Stopping push manager...")
	close(pm.stopChan)
	pm.wg.Wait()
	log.Printf("Push manager stopped")
}

// Enqueue adds a state update to the queue
func (pm *PushManager) Enqueue(entityName string, data interface{}) {
	// Count items for logging
	var itemCount interface{}
	if slice, ok := data.([]interface{}); ok {
		itemCount = len(slice)
	} else if reflect.TypeOf(data).Kind() == reflect.Slice {
		itemCount = reflect.ValueOf(data).Len()
	} else {
		itemCount = "unknown"
	}

	log.Printf("Enqueueing state update for entity %s to controlplane (items: %v)", entityName, itemCount)

	update := StateUpdate{
		EntityName: entityName,
		Data:       data,
		Timestamp:  time.Now(),
		Retries:    0,
	}

	select {
	case pm.queue <- update:
		log.Printf("Successfully enqueued state update for entity %s", entityName)
	default:
		log.Printf("Warning: controlplane push queue is full, dropping state update for entity %s", entityName)
	}
}

// register attempts to register with the controlplane
func (pm *PushManager) register() error {
	if pm.client.IsRegistered() {
		log.Printf("Probe already registered with controlplane")
		pm.mu.Lock()
		pm.connected = true
		pm.lastError = nil
		pm.mu.Unlock()
		return nil
	}

	log.Printf("Attempting to register probe %s v%s with controlplane...", pm.probeName, pm.probeVersion)
	err := pm.client.Register(pm.probeName, pm.probeVersion)
	if err != nil {
		pm.mu.Lock()
		pm.connected = false
		pm.lastError = err
		pm.mu.Unlock()
		log.Printf("Registration failed: %v", err)
		return fmt.Errorf("registration failed: %w", err)
	}

	// Update connected flag based on actual client status
	pm.mu.Lock()
	pm.connected = pm.client.IsRegistered()
	if pm.connected {
		pm.lastError = nil
	}
	pm.mu.Unlock()

	if pm.connected {
		log.Printf("Successfully registered probe %s v%s with controlplane", pm.probeName, pm.probeVersion)
	}

	return nil
}

// pushLoop consumes state updates from the queue and pushes them
func (pm *PushManager) pushLoop() {
	defer pm.wg.Done()

	for {
		select {
		case <-pm.stopChan:
			return
		case update := <-pm.queue:
			pm.handleUpdate(update)
		}
	}
}

// handleUpdate handles a single state update with retry logic
func (pm *PushManager) handleUpdate(update StateUpdate) {
	// Check actual connection status from client instead of cached flag
	if !pm.client.IsRegistered() {
		log.Printf("Not connected to controlplane, skipping push for entity %s", update.EntityName)
		pm.triggerReconnect()
		return
	}

	err := pm.client.PushState(update.EntityName, update.Data)
	if err != nil {
		log.Printf("Error pushing state for entity %s: %v", update.EntityName, err)

		// Update connected flag based on actual client status
		pm.mu.Lock()
		pm.connected = pm.client.IsRegistered()
		if !pm.connected {
			pm.lastError = err
		}
		pm.mu.Unlock()

		// Mark as disconnected on network errors
		if isNetworkError(err) {
			pm.triggerReconnect()
		}

		// Retry if we haven't exceeded max retries
		if update.Retries < pm.config.MaxRetries {
			update.Retries++
			backoff := pm.calculateBackoff(update.Retries)

			log.Printf("Retrying push for entity %s (attempt %d/%d) after %v", update.EntityName, update.Retries, pm.config.MaxRetries, backoff)
			time.Sleep(backoff)

			// Re-queue for retry
			select {
			case pm.queue <- update:
			default:
				log.Printf("Warning: queue full, dropping retry for entity %s", update.EntityName)
			}
		} else {
			log.Printf("Max retries exceeded for entity %s, dropping update", update.EntityName)
		}
	} else {
		log.Printf("Successfully pushed state for entity %s to controlplane", update.EntityName)
	}
}

// heartbeatLoop sends periodic heartbeats and handles reconnection
func (pm *PushManager) heartbeatLoop() {
	defer pm.wg.Done()

	ticker := time.NewTicker(pm.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pm.stopChan:
			return
		case <-ticker.C:
			pm.checkConnection()
		case <-pm.reconnectChan:
			pm.attemptReconnect()
		}
	}
}

// checkConnection checks the connection status via ping and handles configuration_changed
func (pm *PushManager) checkConnection() {
	if !pm.client.IsRegistered() {
		return
	}

	pm.mu.RLock()
	getVersion := pm.getConfigVersion
	onChanged := pm.onConfigChanged
	pm.mu.RUnlock()

	var err error
	status := PingStatusPong
	if getVersion != nil {
		status, err = pm.client.PingWithVersion(getVersion())
	} else {
		err = pm.client.Ping()
	}
	if err != nil {
		log.Printf("Heartbeat failed: %v", err)
		pm.mu.Lock()
		pm.connected = false
		pm.lastError = err
		pm.mu.Unlock()
		pm.triggerReconnect()
		return
	}

	pm.mu.Lock()
	pm.connected = true
	pm.lastError = nil
	pm.mu.Unlock()

	if status == PingStatusConfigurationChanged && onChanged != nil {
		runtimeJSON, configVersion, fetchErr := pm.client.FetchConfig()
		if fetchErr != nil {
			log.Printf("Fetch config after configuration_changed failed: %v", fetchErr)
			return
		}
		log.Printf("Configuration changed, applying new config (version=%q)", configVersion)
		onChanged(runtimeJSON, configVersion)
	}
}

// attemptReconnect attempts to reconnect to the controlplane
func (pm *PushManager) attemptReconnect() {
	log.Printf("Attempting to reconnect to controlplane...")

	backoff := pm.config.RetryBackoff
	maxBackoff := 5 * time.Minute

	for {
		select {
		case <-pm.stopChan:
			return
		default:
		}

		err := pm.register()
		if err == nil {
			log.Printf("Successfully reconnected to controlplane")
			return
		}

		log.Printf("Reconnection failed: %v, retrying in %v", err, backoff)
		time.Sleep(backoff)

		// Exponential backoff with max limit
		backoff = backoff * 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// triggerReconnect triggers a reconnection attempt
func (pm *PushManager) triggerReconnect() {
	select {
	case pm.reconnectChan <- struct{}{}:
	default:
		// Reconnection already triggered
	}
}

// calculateBackoff calculates exponential backoff duration
func (pm *PushManager) calculateBackoff(retries int) time.Duration {
	backoff := pm.config.RetryBackoff
	for i := 1; i < retries; i++ {
		backoff = backoff * 2
	}
	maxBackoff := 30 * time.Second
	if backoff > maxBackoff {
		backoff = maxBackoff
	}
	return backoff
}

// isNetworkError checks if an error is a network-related error
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	networkErrors := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"no such host",
		"network is unreachable",
		"i/o timeout",
	}
	for _, netErr := range networkErrors {
		if contains(errStr, netErr) {
			return true
		}
	}
	return false
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
