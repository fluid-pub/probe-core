package state

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fluid/probes/core"
	"fluid/probes/core/controlplane"

	"gopkg.in/yaml.v3"
)

// resolveSchemaPath locates schema.yml for PushSchema (image path, config sibling, or dev layout).
func resolveSchemaPath(stateDir string) string {
	candidates := []string{"/etc/fluid/config/schema.yml"}
	if stateDir != "" {
		candidates = append([]string{
			filepath.Join(filepath.Dir(stateDir), "config", "schema.yml"),
		}, candidates...)
	}
	candidates = append(candidates, "config/schema.yml")
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// Manager handles state persistence and cleanup
type Manager struct {
	config      ConfigProvider
	pushManager *controlplane.PushManager
	stopCleanup chan struct{}
	cleanupWg   sync.WaitGroup
	mu          sync.Mutex
}

// NewManager creates a new state manager. If the configuration requests a control
// plane connection (websocket URL + org + token), that connection must succeed
// or this function returns a non-nil error.
func NewManager(cfg ConfigProvider) (*Manager, error) {
	m := &Manager{
		config:      cfg,
		stopCleanup: make(chan struct{}),
	}

	// Initialize controlplane client when a full connection is configured
	if core.ControlplaneConnectionRequested(cfg.GetControlplane()) {
		log.Printf("Controlplane configuration found, initializing HTTP connection...")
		// Try to find schema.yml in the same directory as the state directory
		// (assuming config is typically in a sibling directory)
		schemaPath := resolveSchemaPath(cfg.GetStateDir())
		cpConfig := cfg.GetControlplane()
		client, err := controlplane.NewClientFromConfig(cpConfig, cfg.GetProbeName(), cfg.GetProbeVersion(), schemaPath)
		if err != nil {
			return nil, fmt.Errorf("control plane connection required but failed: %w", err)
		}
		if err := m.SetControlplaneClient(client); err != nil {
			return nil, fmt.Errorf("control plane client initialization failed: %w", err)
		}
		if merger, ok := cfg.(controlplane.RuntimeConfigMerger); ok {
			controlplane.FetchAndMergeConfig(merger, client)
		}
	} else if cfg.GetControlplane() != nil {
		log.Printf("Controlplane configuration incomplete, skipping initialization (offline mode)")
	}

	// Start cleanup goroutine if at least one entity has retention frequencies configured
	hasRetention := false
	entitiesWithRetention := []string{}
	for _, entity := range cfg.GetEntities() {
		if entity.Retention != nil {
			hasRetention = true
			entitiesWithRetention = append(entitiesWithRetention, entity.Name)
		}
	}
	if hasRetention {
		log.Printf("Initializing state cleanup process for entities: %v (cleanup interval: %d minutes)", entitiesWithRetention, cfg.GetCleanupInterval())
		m.cleanupWg.Add(1)
		go m.cleanupLoop()
	} else {
		log.Printf("No retention configuration found for any entity, state cleanup process will not start")
	}

	return m, nil
}

// Stop stops the cleanup goroutine and push manager
func (m *Manager) Stop() {
	if m.stopCleanup != nil {
		close(m.stopCleanup)
		m.cleanupWg.Wait()
	}
	if m.pushManager != nil {
		m.pushManager.Stop()
	}
}

// GetPushManager returns the push manager if available
func (m *Manager) GetPushManager() *controlplane.PushManager {
	return m.pushManager
}

// SetConfigCallbacks forwards config callbacks to the push manager so heartbeat
// can report config_version and trigger reload on configuration_changed.
func (m *Manager) SetConfigCallbacks(getVersion controlplane.ConfigVersionFunc, onChanged controlplane.ConfigChangedCallback) {
	if m.pushManager != nil {
		m.pushManager.SetConfigCallbacks(getVersion, onChanged)
	}
}

// SetControlplaneClient sets the controlplane client and initializes the push manager
func (m *Manager) SetControlplaneClient(client controlplane.Client) error {
	if m.config.GetControlplane() == nil {
		return fmt.Errorf("controlplane configuration not found")
	}

	log.Printf("Setting controlplane client and initializing push manager...")
	cpConfig := m.config.GetControlplane()
	log.Printf("Controlplane config: queue_size=%d, max_retries=%d, retry_backoff=%s, heartbeat_interval=%s",
		cpConfig.QueueSize, cpConfig.MaxRetries, cpConfig.RetryBackoff, cpConfig.HeartbeatInterval)
	pushConfig := controlplane.PushConfig{
		QueueSize:         cpConfig.QueueSize,
		MaxRetries:        cpConfig.MaxRetries,
		HeartbeatInterval: 30 * time.Second,
	}

	// Parse retry backoff
	if cpConfig.RetryBackoff != "" {
		backoff, err := time.ParseDuration(cpConfig.RetryBackoff)
		if err != nil {
			log.Printf("Warning: invalid retry_backoff '%s', using default 1s: %v", cpConfig.RetryBackoff, err)
			backoff = 1 * time.Second
		}
		pushConfig.RetryBackoff = backoff
	} else {
		pushConfig.RetryBackoff = 1 * time.Second
	}

	// Parse heartbeat interval
	if cpConfig.HeartbeatInterval != "" {
		interval, err := time.ParseDuration(cpConfig.HeartbeatInterval)
		if err != nil {
			log.Printf("Warning: invalid heartbeat_interval '%s', using default 30s: %v", cpConfig.HeartbeatInterval, err)
			interval = 30 * time.Second
		}
		pushConfig.HeartbeatInterval = interval
	} else {
		pushConfig.HeartbeatInterval = 30 * time.Second
	}

	m.pushManager = controlplane.NewPushManager(
		client,
		pushConfig,
		m.config.GetProbeName(),
		m.config.GetProbeVersion(),
	)

	if err := m.pushManager.Start(); err != nil {
		return fmt.Errorf("failed to start push manager: %w", err)
	}

	return nil
}

// SaveEntity saves entity data with timestamped and latest versions
func (m *Manager) SaveEntity(entityName string, data interface{}) error {
	dir := m.config.GetStateDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating state directory: %w", err)
	}

	timestamp := time.Now().Format("2006-01-02_15-04-05")

	// Save timestamped entity file
	timestampedPath := filepath.Join(dir, fmt.Sprintf("%s_%s.yaml", entityName, timestamp))
	if err := m.saveToFile(timestampedPath, data); err != nil {
		return fmt.Errorf("error saving timestamped %s: %w", entityName, err)
	}

	// Save latest entity file
	latestPath := filepath.Join(dir, fmt.Sprintf("%s.yaml", entityName))
	if err := m.saveToFile(latestPath, data); err != nil {
		return fmt.Errorf("error saving latest %s: %w", entityName, err)
	}

	// Push to controlplane asynchronously if push manager is available
	if m.pushManager != nil {
		log.Printf("Push manager available: pushing state for entity %s to controlplane", entityName)
		m.pushManager.Enqueue(entityName, data)
	} else {
		log.Printf("Warning: push manager is nil, skipping push for entity %s", entityName)
	}

	return nil
}

// saveToFile saves data to a YAML file
func (m *Manager) saveToFile(filePath string, data interface{}) error {
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("error creating file %s: %w", filePath, err)
	}
	defer file.Close()

	encoder := yaml.NewEncoder(file)
	defer encoder.Close()

	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("error encoding data to file %s: %w", filePath, err)
	}

	return nil
}

// cleanupLoop runs the cleanup process at configured intervals
func (m *Manager) cleanupLoop() {
	defer m.cleanupWg.Done()

	cleanupInterval := time.Duration(m.config.GetCleanupInterval()) * time.Minute
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	log.Printf("State cleanup process started (interval: %v)", cleanupInterval)

	for {
		select {
		case <-m.stopCleanup:
			log.Printf("State cleanup process stopped")
			return
		case <-ticker.C:
			log.Printf("Running state cleanup...")
			if err := m.performCleanup(); err != nil {
				log.Printf("Error during cleanup: %v", err)
			}
		}
	}
}

// performCleanup performs the actual cleanup of old state files
func (m *Manager) performCleanup() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if any entity has retention configured
	hasRetention := false
	for _, entity := range m.config.GetEntities() {
		if entity.Retention != nil {
			hasRetention = true
			break
		}
	}
	if !hasRetention {
		return nil
	}

	// Get all timestamped files
	allFiles, err := m.getAllTimestampedFiles()
	if err != nil {
		return fmt.Errorf("error getting timestamped files: %w", err)
	}

	if len(allFiles) == 0 {
		log.Printf("No timestamped state files found, skipping cleanup")
		return nil
	}

	log.Printf("Found %d timestamped state files", len(allFiles))

	// Determine which files to keep based on retention frequencies
	filesToKeep := m.getFilesToKeep(allFiles)

	// Count files to remove
	filesToRemove := make([]string, 0)
	for _, file := range allFiles {
		if !filesToKeep[file] {
			filesToRemove = append(filesToRemove, file)
		}
	}

	if len(filesToRemove) == 0 {
		log.Printf("No files to remove (all files satisfy retention requirements)")
		return nil
	}

	log.Printf("Removing %d state file(s) that do not satisfy retention requirements", len(filesToRemove))

	// Remove files that are not in the keep list
	for _, file := range filesToRemove {
		if err := os.Remove(file); err != nil {
			log.Printf("Warning: error removing file %s: %v", file, err)
		} else {
			log.Printf("Removed state file: %s", filepath.Base(file))
		}
	}

	log.Printf("Cleanup completed: %d file(s) removed, %d file(s) kept", len(filesToRemove), len(allFiles)-len(filesToRemove))

	return nil
}

// getAllTimestampedFiles returns all timestamped state files in the state directory
func (m *Manager) getAllTimestampedFiles() ([]string, error) {
	var allFiles []string
	dir := m.config.GetStateDir()

	// Get all files matching timestamp patterns (entity_*.yaml and state_*.yaml)
	patterns := []string{
		"state_*.yaml",
		"*_*.yaml", // Matches any entity_*.yaml pattern
	}

	for _, pattern := range patterns {
		files, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			return nil, fmt.Errorf("error globbing pattern %s: %w", pattern, err)
		}
		for _, file := range files {
			// Only include files with timestamp pattern (entity_YYYY-MM-DD_HH-MM-SS.yaml)
			if m.isTimestampedFile(file) {
				allFiles = append(allFiles, file)
			}
		}
	}

	return allFiles, nil
}

// isTimestampedFile checks if a file matches the timestamp pattern
func (m *Manager) isTimestampedFile(filePath string) bool {
	filename := filepath.Base(filePath)
	// Pattern: entity_YYYY-MM-DD_HH-MM-SS.yaml or state_YYYY-MM-DD_HH-MM-SS.yaml
	parts := strings.Split(filename, "_")
	if len(parts) < 4 { // Need at least entity, year, month-day, hour-min-sec
		return false
	}
	// Check if the last part ends with .yaml
	lastPart := parts[len(parts)-1]
	if !strings.HasSuffix(lastPart, ".yaml") {
		return false
	}
	return true
}

// getFilesToKeep determines which files to keep based on retention configuration
func (m *Manager) getFilesToKeep(files []string) map[string]bool {
	keepMap := make(map[string]bool)
	now := time.Now()

	// Parse file timestamps and group by entity type
	fileInfos := make([]fileInfo, 0, len(files))
	for _, file := range files {
		info := m.parseFileInfo(file)
		if info.timestamp.IsZero() {
			continue
		}
		fileInfos = append(fileInfos, info)
	}

	// Group files by entity
	entityFiles := make(map[string][]fileInfo)
	for _, info := range fileInfos {
		entityFiles[info.entity] = append(entityFiles[info.entity], info)
	}

	// For each entity, apply its retention configuration
	for _, entityConfig := range m.config.GetEntities() {
		if entityConfig.Retention == nil {
			continue
		}

		entityFileInfos, exists := entityFiles[entityConfig.Name]
		if !exists {
			continue
		}

		retention := entityConfig.Retention

		// Apply retention frequencies from least frequent to most frequent
		if retention.Years != nil {
			keepMap = m.keepFilesForFrequency(entityFileInfos, keepMap, now, "year", *retention.Years)
		}
		if retention.Months != nil {
			keepMap = m.keepFilesForFrequency(entityFileInfos, keepMap, now, "month", *retention.Months)
		}
		if retention.Weeks != nil {
			keepMap = m.keepFilesForFrequency(entityFileInfos, keepMap, now, "week", *retention.Weeks)
		}
		if retention.Days != nil {
			keepMap = m.keepFilesForFrequency(entityFileInfos, keepMap, now, "day", *retention.Days)
		}
		if retention.Hours != nil {
			keepMap = m.keepFilesForFrequency(entityFileInfos, keepMap, now, "hour", *retention.Hours)
		}
		if retention.Minutes != nil {
			keepMap = m.keepFilesForFrequency(entityFileInfos, keepMap, now, "minute", *retention.Minutes)
		}
		if retention.Seconds != nil {
			keepMap = m.keepFilesForFrequency(entityFileInfos, keepMap, now, "second", *retention.Seconds)
		}
	}

	return keepMap
}

type fileInfo struct {
	path      string
	timestamp time.Time
	entity    string
}

// parseFileInfo extracts entity name and timestamp from a file path
func (m *Manager) parseFileInfo(filePath string) fileInfo {
	filename := filepath.Base(filePath)

	// Extract timestamp from filename (format: entity_YYYY-MM-DD_HH-MM-SS.yaml)
	parts := strings.Split(filename, "_")
	if len(parts) < 3 {
		return fileInfo{path: filePath}
	}

	// Reconstruct timestamp string
	timestampStr := strings.Join(parts[1:len(parts)-1], "_") + "_" + strings.TrimSuffix(parts[len(parts)-1], ".yaml")

	timestamp, err := time.Parse("2006-01-02_15-04-05", timestampStr)
	if err != nil {
		return fileInfo{path: filePath}
	}

	entity := parts[0]

	return fileInfo{
		path:      filePath,
		timestamp: timestamp,
		entity:    entity,
	}
}

// keepFilesForFrequency keeps files based on a specific frequency period
func (m *Manager) keepFilesForFrequency(fileInfos []fileInfo, existingKeep map[string]bool, now time.Time, periodType string, maxCount int) map[string]bool {
	if maxCount == 0 {
		// Unlimited: keep all files
		for _, info := range fileInfos {
			existingKeep[info.path] = true
		}
		return existingKeep
	}

	// Group files by period (year, month, day, etc.) and keep only the most recent file per period
	periodGroups := make(map[string][]fileInfo)

	for _, info := range fileInfos {
		periodKey := m.getPeriodKey(info.timestamp, periodType, now, maxCount)
		if periodKey != "" {
			periodGroups[periodKey] = append(periodGroups[periodKey], info)
		}
	}

	// For each period, keep only the most recent file
	for _, group := range periodGroups {
		if len(group) == 0 {
			continue
		}

		// Find the most recent file in this period
		mostRecent := group[0]
		for _, info := range group[1:] {
			if info.timestamp.After(mostRecent.timestamp) {
				mostRecent = info
			}
		}

		existingKeep[mostRecent.path] = true
	}

	return existingKeep
}

// getPeriodKey returns a unique key for the period containing the given timestamp
func (m *Manager) getPeriodKey(timestamp time.Time, periodType string, now time.Time, maxCount int) string {
	switch periodType {
	case "year":
		cutoffYear := now.Year() - maxCount + 1
		if timestamp.Year() < cutoffYear {
			return ""
		}
		return fmt.Sprintf("year_%d", timestamp.Year())

	case "month":
		cutoffDate := now.AddDate(0, -maxCount+1, 0)
		cutoffYear, cutoffMonth, _ := cutoffDate.Date()
		timestampYear, timestampMonth, _ := timestamp.Date()
		if timestampYear < cutoffYear || (timestampYear == cutoffYear && timestampMonth < cutoffMonth) {
			return ""
		}
		return fmt.Sprintf("month_%d_%02d", timestampYear, timestampMonth)

	case "week":
		cutoffDate := now.AddDate(0, 0, -(maxCount-1)*7)
		cutoffYear, cutoffWeek := cutoffDate.ISOWeek()
		timestampYear, timestampWeek := timestamp.ISOWeek()
		if timestampYear < cutoffYear || (timestampYear == cutoffYear && timestampWeek < cutoffWeek) {
			return ""
		}
		return fmt.Sprintf("week_%d_%02d", timestampYear, timestampWeek)

	case "day":
		cutoffDate := now.AddDate(0, 0, -maxCount+1)
		cutoffYear, cutoffMonth, cutoffDay := cutoffDate.Date()
		timestampYear, timestampMonth, timestampDay := timestamp.Date()
		if timestampYear < cutoffYear || (timestampYear == cutoffYear && timestampMonth < cutoffMonth) || (timestampYear == cutoffYear && timestampMonth == cutoffMonth && timestampDay < cutoffDay) {
			return ""
		}
		return fmt.Sprintf("day_%d_%02d_%02d", timestampYear, timestampMonth, timestampDay)

	case "hour":
		cutoffTime := now.Add(-time.Duration(maxCount) * time.Hour)
		if timestamp.Before(cutoffTime) {
			return ""
		}
		return fmt.Sprintf("hour_%d_%02d_%02d_%02d", timestamp.Year(), timestamp.Month(), timestamp.Day(), timestamp.Hour())

	case "minute":
		cutoffTime := now.Add(-time.Duration(maxCount) * time.Minute)
		if timestamp.Before(cutoffTime) {
			return ""
		}
		return fmt.Sprintf("minute_%d_%02d_%02d_%02d_%02d", timestamp.Year(), timestamp.Month(), timestamp.Day(), timestamp.Hour(), timestamp.Minute())

	case "second":
		cutoffTime := now.Add(-time.Duration(maxCount) * time.Second)
		if timestamp.Before(cutoffTime) {
			return ""
		}
		return fmt.Sprintf("second_%d_%02d_%02d_%02d_%02d_%02d", timestamp.Year(), timestamp.Month(), timestamp.Day(), timestamp.Hour(), timestamp.Minute(), timestamp.Second())

	default:
		return ""
	}
}
