package controlplane

import (
	"fmt"
	"net/url"
	"strings"
)

// ParseBaseURL normalizes the control plane HTTP API root (no trailing slash).
func ParseBaseURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("invalid controlplane base_url: %w", err)
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return "", fmt.Errorf("controlplane base_url scheme must be http or https, got %q", u.Scheme)
	}
	u.Path = ""
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), nil
}
