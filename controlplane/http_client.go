package controlplane

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// HTTPClient implements Client using the control plane /probes HTTP API.
type HTTPClient struct {
	baseURL          string
	organizationUUID string
	token            string
	probeName        string
	probeVersion     string
	httpClient       *http.Client
	mu               sync.RWMutex
	registered       bool
}

// NewHTTPClient creates a probe HTTP transport client.
func NewHTTPClient(baseURL, organizationUUID, token, probeName, probeVersion string) (*HTTPClient, error) {
	resolved, err := ParseBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	if organizationUUID == "" {
		return nil, fmt.Errorf("organization UUID is required")
	}
	if token == "" {
		return nil, fmt.Errorf("token is required")
	}
	return &HTTPClient{
		baseURL:          resolved,
		organizationUUID: organizationUUID,
		token:            token,
		probeName:        probeName,
		probeVersion:     probeVersion,
		httpClient:       &http.Client{Timeout: 60 * time.Second},
	}, nil
}

func (c *HTTPClient) path(parts ...string) string {
	escaped := make([]string, 0, len(parts))
	for _, p := range parts {
		escaped = append(escaped, url.PathEscape(p))
	}
	return c.baseURL + "/" + strings.Join(escaped, "/")
}

func (c *HTTPClient) postJSON(url string, body interface{}) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(http.MethodPost, url, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.httpClient.Do(req)
}

func (c *HTTPClient) Register(probeName, probeVersion string) error {
	_ = probeName
	_ = probeVersion
	url := c.path("probes", "register", c.organizationUUID, c.token)
	resp, err := c.postJSON(url, nil)
	if err != nil {
		return fmt.Errorf("register request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("register failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	c.mu.Lock()
	c.registered = true
	c.mu.Unlock()
	return nil
}

func (c *HTTPClient) PushSchema(schemaData map[string]interface{}) error {
	schemaPayload := map[string]interface{}{
		"probe":    c.probeName,
		"version":  c.probeVersion,
		"entities": schemaData,
	}
	url := c.path("probes", "v1", "schema", c.organizationUUID, c.token)
	resp, err := c.postJSON(url, map[string]interface{}{"schema": schemaPayload})
	if err != nil {
		return fmt.Errorf("push schema request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("push schema failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	log.Printf("Successfully pushed schema to controlplane (HTTP)")
	return nil
}

func (c *HTTPClient) PushState(entityName string, stateData interface{}) error {
	statePayload := map[string]interface{}{
		"probe":     c.probeName,
		"timestamp": time.Now().Format(time.RFC3339),
		"version":   c.probeVersion,
		"data": map[string]interface{}{
			"entities": map[string]interface{}{
				entityName: stateData,
			},
		},
	}
	body := map[string]interface{}{
		"state": statePayload,
	}
	url := c.path("probes", "v1", "ingest", c.organizationUUID, c.token)
	resp, err := c.postJSON(url, body)
	if err != nil {
		return fmt.Errorf("push state request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("push state failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func (c *HTTPClient) IsRegistered() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.registered
}

func (c *HTTPClient) Ping() error {
	_, err := c.PingWithVersion("")
	return err
}

func (c *HTTPClient) PingWithVersion(configVersion string) (string, error) {
	payload := map[string]interface{}{}
	if configVersion != "" {
		payload["config_version"] = configVersion
	}
	url := c.path("probes", "ping", c.organizationUUID, c.token)
	resp, err := c.postJSON(url, payload)
	if err != nil {
		return "", fmt.Errorf("ping request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ping failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return PingStatusPong, nil
	}
	if parsed.Status == "" {
		return PingStatusPong, nil
	}
	return parsed.Status, nil
}

func (c *HTTPClient) FetchConfig() (runtimeConfigJSON []byte, configVersion string, err error) {
	url := c.path("probes", "config", c.organizationUUID, c.token)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create config request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch config: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("config endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read config response: %w", err)
	}
	var parsed struct {
		ConfigVersion string `json:"config_version"`
	}
	_ = json.Unmarshal(body, &parsed)
	return body, parsed.ConfigVersion, nil
}
