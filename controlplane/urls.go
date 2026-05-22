package controlplane

import (
	"fmt"
	"net/url"
	"strings"
)

// ResolvedBaseURL returns the HTTP base URL for probe transport (no trailing slash).
// Prefers base_url; otherwise derives from websocket_url (legacy configs).
func ResolvedBaseURL(baseURL, websocketURL string) (string, error) {
	if b := strings.TrimSpace(baseURL); b != "" {
		return strings.TrimRight(b, "/"), nil
	}
	if w := strings.TrimSpace(websocketURL); w != "" {
		return BaseURLFromWebSocketURL(w)
	}
	return "", fmt.Errorf("controlplane base_url or websocket_url is required")
}

// BaseURLFromWebSocketURL maps a legacy probe WebSocket URL to the HTTP API base.
func BaseURLFromWebSocketURL(wsURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(wsURL))
	if err != nil {
		return "", fmt.Errorf("invalid websocket URL: %w", err)
	}
	switch u.Scheme {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	case "http", "https":
		// Already an HTTP base (operators sometimes store the API root).
	default:
		return "", fmt.Errorf("invalid websocket scheme: %s", u.Scheme)
	}
	u.Path = ""
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), nil
}
