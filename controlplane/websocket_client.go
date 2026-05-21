package controlplane

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Must match ControlplaneWeb.ProbeSocket ("agent:lobby" → ProbeChannel).
	channelTopic = "agent:lobby"
	pingInterval = 30 * time.Second
)

type Message struct {
	Topic   string                 `json:"topic"`
	Event   string                 `json:"event"`
	Payload map[string]interface{} `json:"payload"`
	Ref     string                 `json:"ref"`
}

type Reply struct {
	Status   string                 `json:"status"`
	Response map[string]interface{} `json:"response"`
}

type WebSocketClient struct {
	websocketURL     string
	configURL        string // HTTP URL for GET config (derived from websocket URL)
	conn             *websocket.Conn
	connMu           sync.RWMutex
	writeMu          sync.Mutex
	connected        bool
	joined           bool
	refCounter       int64
	refMu            sync.Mutex
	stopChan         chan struct{}
	wg               sync.WaitGroup
	probeName        string
	probeVersion     string
	organizationUUID string
	token            string
	replyChans       map[string]chan Reply
	replyMu          sync.RWMutex
}

func NewWebSocketClient(websocketURL, organizationUUID, token, probeName, probeVersion string) (*WebSocketClient, error) {
	if websocketURL == "" {
		return nil, fmt.Errorf("websocket URL is required")
	}

	if organizationUUID == "" {
		return nil, fmt.Errorf("organization UUID is required")
	}

	if token == "" {
		return nil, fmt.Errorf("token is required")
	}

	parsedURL, err := url.Parse(websocketURL)
	if err != nil {
		return nil, fmt.Errorf("invalid websocket URL: %w", err)
	}

	if parsedURL.Scheme != "ws" && parsedURL.Scheme != "wss" {
		return nil, fmt.Errorf("invalid websocket scheme: %s (expected ws or wss)", parsedURL.Scheme)
	}

	query := parsedURL.Query()
	query.Set("organization_uuid", organizationUUID)
	query.Set("token", token)
	parsedURL.RawQuery = query.Encode()

	completeURL := parsedURL.String()

	// Derive HTTP config URL: same host, https, path /agents/config/:org_uuid/:token
	configURL := configURLFromWebSocket(websocketURL, organizationUUID, token)

	return &WebSocketClient{
		websocketURL:     completeURL,
		configURL:        configURL,
		connected:        false,
		joined:           false,
		stopChan:         make(chan struct{}),
		probeName:        probeName,
		probeVersion:     probeVersion,
		organizationUUID: organizationUUID,
		token:            token,
		replyChans:       make(map[string]chan Reply),
	}, nil
}

// configURLFromWebSocket builds the HTTP URL for the config API from the websocket URL.
func configURLFromWebSocket(wsURL, orgUUID, token string) string {
	u, err := url.Parse(wsURL)
	if err != nil {
		return ""
	}
	scheme := "https"
	if u.Scheme == "ws" {
		scheme = "http"
	}
	// Path like /agents/config/:organization_uuid/:token
	path := fmt.Sprintf("/agents/config/%s/%s", url.PathEscape(orgUUID), url.PathEscape(token))
	return fmt.Sprintf("%s://%s%s", scheme, u.Host, path)
}

func (c *WebSocketClient) Connect() error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.connected && c.conn != nil {
		return nil
	}

	dialer := websocket.Dialer{
		HandshakeTimeout:  10 * time.Second,
		EnableCompression: true, // Enable WebSocket compression (permessage-deflate)
	}

	conn, resp, err := dialer.Dial(c.websocketURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect WebSocket: %w", err)
	}

	// Log compression status
	if resp != nil {
		compressionHeader := resp.Header.Get("Sec-WebSocket-Extensions")
		if compressionHeader != "" {
			log.Printf("WebSocket compression negotiated: %s", compressionHeader)
		} else {
			log.Printf("WebSocket compression not negotiated (server may not support it)")
		}
	}

	c.conn = conn
	c.connected = true

	c.wg.Add(1)
	go c.readLoop()

	return nil
}

func (c *WebSocketClient) Join() error {
	if !c.connected {
		return fmt.Errorf("not connected to WebSocket")
	}

	if c.joined {
		return nil
	}

	ref := c.nextRef()
	replyChan := make(chan Reply, 1)
	c.replyMu.Lock()
	c.replyChans[ref] = replyChan
	c.replyMu.Unlock()

	defer func() {
		c.replyMu.Lock()
		delete(c.replyChans, ref)
		c.replyMu.Unlock()
	}()

	joinMsg := Message{
		Topic:   channelTopic,
		Event:   "phx_join",
		Payload: make(map[string]interface{}),
		Ref:     ref,
	}

	if err := c.sendMessage(joinMsg); err != nil {
		return fmt.Errorf("failed to send join message: %w", err)
	}

	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()

	select {
	case <-timeout.C:
		return fmt.Errorf("timeout waiting for join response")
	case reply := <-replyChan:
		if reply.Status == "ok" {
			c.joined = true
			return nil
		}
		reason, _ := reply.Response["reason"].(string)
		return fmt.Errorf("join failed: %s", reason)
	}
}

func (c *WebSocketClient) Disconnect() error {
	close(c.stopChan)

	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn != nil {
		if err := c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")); err != nil {
			log.Printf("Error sending close message: %v", err)
		}
		if err := c.conn.Close(); err != nil {
			return fmt.Errorf("error closing WebSocket: %w", err)
		}
		c.conn = nil
	}

	c.connected = false
	c.joined = false

	c.wg.Wait()

	return nil
}

func (c *WebSocketClient) IsConnected() bool {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.connected && c.joined
}

func (c *WebSocketClient) sendMessage(msg Message) error {
	c.connMu.RLock()
	conn := c.conn
	connected := c.connected
	c.connMu.RUnlock()

	if conn == nil || !connected {
		return fmt.Errorf("WebSocket connection is closed")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Log message size for debugging large payloads
	// Note: Actual compressed size depends on WebSocket compression negotiation
	// Compression is enabled via EnableCompression in the dialer
	if msg.Event == "push_state" {
		log.Printf("Preparing to send push_state message (compression enabled), uncompressed size: %d bytes (%.2f KB)", len(data), float64(len(data))/1024)
	}

	// Use writeMu to serialize all writes to the WebSocket connection
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	// Re-check connection after acquiring write lock
	c.connMu.RLock()
	conn = c.conn
	connected = c.connected
	c.connMu.RUnlock()

	if conn == nil || !connected {
		return fmt.Errorf("WebSocket connection is closed")
	}

	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		c.connMu.Lock()
		c.connected = false
		c.joined = false
		c.connMu.Unlock()
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}

func (c *WebSocketClient) readLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.stopChan:
			return
		default:
			c.connMu.RLock()
			conn := c.conn
			c.connMu.RUnlock()

			if conn == nil {
				return
			}

			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket read error: %v", err)
				}
				c.connMu.Lock()
				c.connected = false
				c.joined = false
				c.connMu.Unlock()
				return
			}

			var msg Message
			if err := json.Unmarshal(message, &msg); err != nil {
				log.Printf("Error unmarshaling message: %v", err)
				continue
			}

			c.handleMessage(msg)
		}
	}
}

func (c *WebSocketClient) handleMessage(msg Message) {
	if msg.Event == "phx_reply" {
		status, _ := msg.Payload["status"].(string)
		response, _ := msg.Payload["response"].(map[string]interface{})

		reply := Reply{
			Status:   status,
			Response: response,
		}

		c.replyMu.RLock()
		replyChan, exists := c.replyChans[msg.Ref]
		c.replyMu.RUnlock()

		if exists {
			select {
			case replyChan <- reply:
			default:
			}
		} else {
			if status == "ok" {
				log.Printf("Received phx_reply OK for ref %s (no waiting channel)", msg.Ref)
			} else if status == "error" {
				reason, _ := response["reason"].(string)
				log.Printf("Received phx_reply ERROR for ref %s: %s", msg.Ref, reason)
			}
		}
	}
}

func (c *WebSocketClient) nextRef() string {
	c.refMu.Lock()
	defer c.refMu.Unlock()
	c.refCounter++
	return fmt.Sprintf("%d", c.refCounter)
}

func (c *WebSocketClient) Register(probeName, probeVersion string) error {
	if !c.connected {
		if err := c.Connect(); err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
	}
	return c.Join()
}

func (c *WebSocketClient) PushSchema(schemaData map[string]interface{}) error {
	if !c.joined {
		return fmt.Errorf("not joined to channel")
	}

	schemaPayload := map[string]interface{}{
		"probe":    c.probeName,
		"version":  c.probeVersion,
		"entities": schemaData,
	}

	ref := c.nextRef()
	pushMsg := Message{
		Topic: channelTopic,
		Event: "push_schema",
		Payload: map[string]interface{}{
			"schema": schemaPayload,
		},
		Ref: ref,
	}

	return c.sendMessage(pushMsg)
}

func (c *WebSocketClient) PushState(entityName string, stateData interface{}) error {
	if !c.connected {
		return fmt.Errorf("not connected to WebSocket")
	}
	if !c.joined {
		return fmt.Errorf("not joined to channel")
	}

	// Count items if stateData is a slice
	var itemCount interface{}
	if slice, ok := stateData.([]interface{}); ok {
		itemCount = len(slice)
	} else if reflect.TypeOf(stateData).Kind() == reflect.Slice {
		itemCount = reflect.ValueOf(stateData).Len()
	} else {
		itemCount = "unknown"
	}

	log.Printf("Pushing state for entity %s to controlplane (items: %v)", entityName, itemCount)

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

	ref := c.nextRef()
	pushMsg := Message{
		Topic: channelTopic,
		Event: "push_state",
		Payload: map[string]interface{}{
			"state": statePayload,
		},
		Ref: ref,
	}

	err := c.sendMessage(pushMsg)
	if err != nil {
		log.Printf("Error sending push_state message for entity %s: %v", entityName, err)
		return err
	}

	log.Printf("Successfully sent push_state message for entity %s (ref: %s)", entityName, ref)
	return nil
}

func (c *WebSocketClient) IsRegistered() bool {
	return c.IsConnected()
}

func (c *WebSocketClient) Ping() error {
	_, err := c.PingWithVersion("")
	return err
}

// PingWithVersion sends a ping with optional config_version and returns the response status
// (pong or configuration_changed). Waits for phx_reply with a timeout.
func (c *WebSocketClient) PingWithVersion(configVersion string) (status string, err error) {
	if !c.joined {
		return "", fmt.Errorf("not joined to channel")
	}

	payload := make(map[string]interface{})
	if configVersion != "" {
		payload["config_version"] = configVersion
	}

	ref := c.nextRef()
	replyChan := make(chan Reply, 1)
	c.replyMu.Lock()
	c.replyChans[ref] = replyChan
	c.replyMu.Unlock()

	defer func() {
		c.replyMu.Lock()
		delete(c.replyChans, ref)
		c.replyMu.Unlock()
	}()

	pingMsg := Message{
		Topic:   channelTopic,
		Event:   "ping",
		Payload: payload,
		Ref:     ref,
	}
	if err := c.sendMessage(pingMsg); err != nil {
		return "", err
	}

	timeout := time.NewTimer(15 * time.Second)
	defer timeout.Stop()
	select {
	case reply := <-replyChan:
		if reply.Status != "ok" {
			return "", fmt.Errorf("ping failed: %s", reply.Status)
		}
		s, _ := reply.Response["status"].(string)
		if s == "" {
			s = PingStatusPong
		}
		return s, nil
	case <-timeout.C:
		return "", fmt.Errorf("ping timeout waiting for reply")
	}
}

// FetchConfig fetches runtime config from the dedicated config endpoint.
func (c *WebSocketClient) FetchConfig() (runtimeConfigJSON []byte, configVersion string, err error) {
	if c.configURL == "" {
		return nil, "", fmt.Errorf("config URL not configured")
	}
	req, err := http.NewRequest(http.MethodGet, c.configURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create config request: %w", err)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
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

func (c *WebSocketClient) StartPingLoop() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if c.joined {
					if err := c.Ping(); err != nil {
						log.Printf("Error sending ping: %v", err)
					}
				}
			case <-c.stopChan:
				return
			}
		}
	}()
}

func (c *WebSocketClient) StartReconnectLoop() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()

		backoff := 1 * time.Second
		maxBackoff := 60 * time.Second

		for {
			select {
			case <-c.stopChan:
				return
			case <-time.After(backoff):
				if !c.IsConnected() {
					log.Printf("Attempting to reconnect to controlplane (backoff: %v)", backoff)

					if err := c.Connect(); err != nil {
						log.Printf("Reconnection failed: %v", err)
						backoff = min(backoff*2, maxBackoff)
						continue
					}

					if err := c.Join(); err != nil {
						log.Printf("Rejoin failed: %v", err)
						c.connMu.Lock()
						if c.conn != nil {
							c.conn.Close()
							c.conn = nil
						}
						c.connected = false
						c.connMu.Unlock()
						backoff = min(backoff*2, maxBackoff)
						continue
					}

					log.Printf("Successfully reconnected to controlplane")
					backoff = 1 * time.Second
				} else {
					backoff = 1 * time.Second
				}
			}
		}
	}()
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
