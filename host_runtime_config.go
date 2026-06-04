package core

import "time"

// Host collection entity names used by host-native probes (e.g. Debian) for interval mapping.
const (
	EntityDebianSystemMetrics      = "debian_system_metrics"
	EntityDebianFilesystem         = "debian_filesystem"
	EntityDebianFileChecks         = "debian_file_checks"
	EntityDebianPackageUpdates     = "debian_package_updates"
	EntityDebianInstalledPackages  = "debian_installed_packages"
	EntityDebianSystemdServices    = "debian_systemd_services"
)

// HostCollectionOverlay is optional collection tuning from control plane runtime_config.
type HostCollectionOverlay struct {
	SystemInterval            string `json:"system_interval,omitempty"`
	FilesInterval             string `json:"files_interval,omitempty"`
	APTInterval               string `json:"apt_interval,omitempty"`
	InstalledPackagesInterval string `json:"installed_packages_interval,omitempty"`
	ServicesInterval          string `json:"services_interval,omitempty"`
	RequestTimeout            string `json:"request_timeout,omitempty"`
	MaxPackageItems           *int   `json:"max_package_items,omitempty"`
	MaxHashFileSizeBytes      *int64 `json:"max_hash_file_size_bytes,omitempty"`
}

// HostFileRule is a file path to integrity-check from runtime_config.
type HostFileRule struct {
	Path string `json:"path"`
}

// HostDirectoryRule is a directory to scan from runtime_config.
type HostDirectoryRule struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
}

// ApplyEntityIntervalsToCollection maps data.entities refresh_interval values onto
// collection interval fields when the runtime payload does not set them explicitly.
func ApplyEntityIntervalsToCollection(overlay *HostCollectionOverlay, entities []EntityConfig) {
	if overlay == nil {
		return
	}
	for _, e := range entities {
		if e.Name == "" || e.RefreshInterval == "" {
			continue
		}
		switch e.Name {
		case EntityDebianSystemMetrics, EntityDebianFilesystem:
			if overlay.SystemInterval == "" {
				overlay.SystemInterval = e.RefreshInterval
			}
		case EntityDebianFileChecks:
			if overlay.FilesInterval == "" {
				overlay.FilesInterval = e.RefreshInterval
			}
		case EntityDebianPackageUpdates:
			if overlay.APTInterval == "" {
				overlay.APTInterval = e.RefreshInterval
			}
		case EntityDebianInstalledPackages:
			if overlay.InstalledPackagesInterval == "" {
				overlay.InstalledPackagesInterval = e.RefreshInterval
			}
		case EntityDebianSystemdServices:
			if overlay.ServicesInterval == "" {
				overlay.ServicesInterval = e.RefreshInterval
			}
		}
	}
}

// MergeHostCollectionOverlay applies non-empty remote fields onto local duration strings and limits.
func MergeHostCollectionOverlay(local *HostCollectionOverlay, remote *HostCollectionOverlay) *HostCollectionOverlay {
	if remote == nil {
		return local
	}
	out := *local
	if remote.SystemInterval != "" {
		out.SystemInterval = remote.SystemInterval
	}
	if remote.FilesInterval != "" {
		out.FilesInterval = remote.FilesInterval
	}
	if remote.APTInterval != "" {
		out.APTInterval = remote.APTInterval
	}
	if remote.InstalledPackagesInterval != "" {
		out.InstalledPackagesInterval = remote.InstalledPackagesInterval
	}
	if remote.ServicesInterval != "" {
		out.ServicesInterval = remote.ServicesInterval
	}
	if remote.RequestTimeout != "" {
		out.RequestTimeout = remote.RequestTimeout
	}
	if remote.MaxPackageItems != nil {
		out.MaxPackageItems = remote.MaxPackageItems
	}
	if remote.MaxHashFileSizeBytes != nil {
		out.MaxHashFileSizeBytes = remote.MaxHashFileSizeBytes
	}
	return &out
}

// ParseDurationFields validates and parses host collection interval strings.
func ParseDurationFields(c HostCollectionOverlay) (system, files, apt, installed, services, request time.Duration, err error) {
	system, err = time.ParseDuration(c.SystemInterval)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}
	files, err = time.ParseDuration(c.FilesInterval)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}
	apt, err = time.ParseDuration(c.APTInterval)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}
	installed, err = time.ParseDuration(c.InstalledPackagesInterval)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}
	services, err = time.ParseDuration(c.ServicesInterval)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}
	request, err = time.ParseDuration(c.RequestTimeout)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}
	return system, files, apt, installed, services, request, nil
}
