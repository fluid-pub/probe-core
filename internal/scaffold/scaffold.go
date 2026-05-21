package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

const mainTemplate = `package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"fluid/probes/core"
	"{{.ModulePath}}/internal/probe"
	"{{.ModulePath}}/internal/config"
)

func main() {
	configPath := flag.String("config", "config/probe.yml", "Path to configuration file")
	flag.Parse()

	// Load environment variables from env.secrets using core function
	if err := core.LoadEnvSecrets(); err != nil {
		log.Printf("Warning: failed to load env.secrets: %v", err)
	}

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	{{.ServiceName}}Probe, err := probe.NewProbe(cfg)
	if err != nil {
		log.Fatalf("failed to initialize probe: %v", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	if err := {{.ServiceName}}Probe.Start(); err != nil {
		log.Fatalf("Error starting probe: %v", err)
	}

	log.Printf("{{.ServiceTitle}} probe started successfully. Press Ctrl+C to stop.")

	sig := <-sigChan
	log.Printf("Received signal: %v, shutting down...", sig)

	{{.ServiceName}}Probe.Stop()
}
`

const probeTemplate = `package probe

import (
	"fmt"
	"log"

	"fluid/probes/core"
	"{{.ModulePath}}/internal/probe/entities"
	"{{.ModulePath}}/internal/config"
	"{{.ModulePath}}/internal/manager"
	"{{.ModulePath}}/internal/{{.ServiceName}}"
)

type Probe struct {
	*core.Probe
	config *config.Config
	client *{{.ServiceName}}.Client
}

func NewProbe(cfg *config.Config) (*Probe, error) {
	client, err := {{.ServiceName}}.NewClient(&cfg.{{.ServiceTitle}})
	if err != nil {
		return nil, fmt.Errorf("{{.ServiceName}} client: %w", err)
	}

	stateManager, err := manager.NewManager(cfg)
	if err != nil {
		return nil, err
	}

	coreProbe := core.NewProbe(cfg, client, stateManager)

	{{.ServiceName}}Probe := &Probe{
		Probe:  coreProbe,
		config: cfg,
		client: client,
	}

	coreProbe.RegisterEntity(entities.NewExampleEntity())

	return {{.ServiceName}}Probe, nil
}

func (a *Probe) Start() error {
	log.Printf("{{.ServiceTitle}} probe starting...")
	return a.Probe.Start()
}

func (a *Probe) GetStatus() map[string]interface{} {
	status := a.Probe.GetStatus()
	status["{{.ServiceName}}_configured"] = true
	return status
}
`

const entityTemplate = `package entities

import (
	"fmt"
	"log"

	"fluid/probes/core"
	"{{.ModulePath}}/internal/models"
	"{{.ModulePath}}/internal/{{.ServiceName}}"
)

type ExampleEntity struct{}

func NewExampleEntity() *ExampleEntity {
	return &ExampleEntity{}
}

func (e *ExampleEntity) Name() string {
	return "{{.EntityName}}"
}

func (e *ExampleEntity) Refresh(client core.Client) (interface{}, error) {
	{{.ServiceName}}Client, ok := client.(*{{.ServiceName}}.Client)
	if !ok {
		return nil, fmt.Errorf("invalid client type for {{.EntityName}} entity, expected *{{.ServiceName}}.Client")
	}

	log.Printf("Retrieving {{.ServiceTitle}} {{.EntityName}}...")

	data, err := {{.ServiceName}}Client.GetExample()
	if err != nil {
		return nil, fmt.Errorf("error retrieving {{.EntityName}}: %w", err)
	}

	log.Printf("Retrieved %d {{.EntityName}}", len(data))
	return data, nil
}

func (e *ExampleEntity) Save(stateManager core.StateManager, data interface{}) error {
	{{.EntityName}}Data, ok := data.([]models.Example)
	if !ok {
		return fmt.Errorf("invalid data type for {{.EntityName}} entity")
	}

	if err := stateManager.SaveEntity(e.Name(), {{.EntityName}}Data); err != nil {
		return fmt.Errorf("error saving {{.EntityName}}: %w", err)
	}

	log.Printf("Example state saved successfully")
	return nil
}
`

const configTemplate = `package config

import (
	"fmt"
	"os"
	"strings"

	"fluid/probes/core"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Probe      core.ProbeConfig ` + "`yaml:\"probe\"`" + `
	{{.ServiceTitle}} {{.ServiceTitle}}Config ` + "`yaml:\"{{.ServiceName}}\"`" + `
	Data       core.DataConfig  ` + "`yaml:\"data\"`" + `
	State      core.StateConfig ` + "`yaml:\"state\"`" + `
}

type {{.ServiceTitle}}Config struct {
	APIURL string ` + "`yaml:\"api_url\"`" + `
	Token  string ` + "`yaml:\"token\"`" + `
}

func LoadConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("error reading configuration file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("error parsing configuration: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	if err := config.resolveEnvironmentVariables(); err != nil {
		return nil, fmt.Errorf("error resolving environment variables: %w", err)
	}

	return &config, nil
}

func (c *Config) Validate() error {
	if c.Probe.Name == "" {
		return fmt.Errorf("probe name is missing")
	}

	if c.{{.ServiceTitle}}.APIURL == "" {
		return fmt.Errorf("{{.ServiceName}} API URL is missing")
	}

	if len(c.Data.Entities) == 0 {
		return fmt.Errorf("at least one entity must be configured")
	}

	if c.State.Dir == "" {
		return fmt.Errorf("state directory is missing")
	}

	if c.State.CleanupInterval <= 0 {
		c.State.CleanupInterval = 1
	}

	return nil
}

func (c *Config) resolveEnvironmentVariables() error {
	if c.{{.ServiceTitle}}.Token != "" && c.{{.ServiceTitle}}.Token[0] == '$' {
		envVar := os.Getenv(strings.Trim(c.{{.ServiceTitle}}.Token, "${}"))
		if envVar == "" {
			return fmt.Errorf("environment variable %s is not defined", c.{{.ServiceTitle}}.Token)
		}
		c.{{.ServiceTitle}}.Token = envVar
	}

	return nil
}

func (c *Config) GetProbeName() string {
	return c.Probe.Name
}

func (c *Config) GetProbeVersion() string {
	return c.Probe.Version
}

func (c *Config) GetStateDir() string {
	return c.State.Dir
}

func (c *Config) GetCleanupInterval() int {
	return c.State.CleanupInterval
}

func (c *Config) GetEntities() []core.EntityConfig {
	return c.Data.Entities
}
`

const managerTemplate = `package manager

import (
	"fluid/probes/core/state"
	"{{.ModulePath}}/internal/config"
)

type Manager struct {
	coreManager *state.Manager
	config      *config.Config
}

func NewManager(cfg *config.Config) (*Manager, error) {
	coreManager, err := state.NewManager(cfg)
	if err != nil {
		return nil, err
	}

	return &Manager{
		coreManager: coreManager,
		config:      cfg,
	}, nil
}

func (m *Manager) Stop() {
	m.coreManager.Stop()
}

func (m *Manager) SaveEntity(entityName string, data interface{}) error {
	return m.coreManager.SaveEntity(entityName, data)
}
`

const modelsTemplate = `package models

import "time"

type Example struct {
	ID          string    ` + "`yaml:\"id\" json:\"id\"`" + `
	Name        string    ` + "`yaml:\"name\" json:\"name\"`" + `
	CreatedAt   time.Time ` + "`yaml:\"created_at\" json:\"created_at\"`" + `
	UpdatedAt   time.Time ` + "`yaml:\"updated_at\" json:\"updated_at\"`" + `
}
`

const clientTemplate = `package {{.ServiceName}}

import (
	"fmt"
	"{{.ModulePath}}/internal/config"
)

type Client struct {
	APIURL string
	Token  string
}

func NewClient(cfg *config.{{.ServiceTitle}}Config) (*Client, error) {
	if cfg.APIURL == "" {
		return nil, fmt.Errorf("API URL is required")
	}

	if cfg.Token == "" {
		return nil, fmt.Errorf("token is required")
	}

	return &Client{
		APIURL: cfg.APIURL,
		Token:  cfg.Token,
	}, nil
}

func (c *Client) GetExample() ([]interface{}, error) {
	return nil, fmt.Errorf("GetExample not implemented")
}
`

const configYamlTemplate = `# {{.ServiceTitle}} Probe Configuration
probe:
  name: "{{.Name}}"
  version: "1.0.0"

# {{.ServiceTitle}} Configuration
{{.ServiceName}}:
  api_url: "https://api.example.com"
  token: "${FLUID_{{.ServiceTitleUpper}}_TOKEN}"

# Data entities to retrieve
data:
  entities:
    - name: {{.EntityName}}
      refresh_interval: "1h"
      retention_frequencies:
        days: 30
        months: 12

# State configuration
state:
  dir: "state"
  format: "yaml"
  cleanup_interval: 1
`

const configExampleYamlTemplate = `# {{.ServiceTitle}} Probe Configuration Example
probe:
  name: "{{.Name}}"
  version: "1.0.0"

# {{.ServiceTitle}} Configuration
{{.ServiceName}}:
  api_url: "https://api.example.com"
  token: "${FLUID_{{.ServiceTitleUpper}}_TOKEN}"

# Data entities to retrieve
data:
  entities:
    - name: {{.EntityName}}
      refresh_interval: "1h"
      retention_frequencies:
        days: 30
        months: 12

# State configuration
state:
  dir: "state"
  format: "yaml"
  cleanup_interval: 1
`

const goModTemplate = `module {{.ModulePath}}

go 1.23

require (
	fluid/probes/core v0.1.0
	gopkg.in/yaml.v3 v3.0.1
)

replace fluid/probes/core => ../core
`

const makefileTemplate = `# Makefile for Fluid {{.ServiceTitle}} Probe

BINARY_NAME={{.BinaryName}}
BUILD_DIR=build
CONFIG_DIR=config
STATE_DIR=state

GO=go
GOOS?=$(shell go env GOOS)
GOARCH?=$(shell go env GOARCH)

VERSION?=1.0.0
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.GitCommit=${GIT_COMMIT}"

.PHONY: all build clean run test deps help

all: clean build

deps:
	@echo "Installing dependencies..."
	$(GO) mod download
	@$(GO) mod tidy

build: deps
	@echo "Building {{.ServiceTitle}} probe..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) cmd/main.go
	@echo "Probe built in $(BUILD_DIR)/$(BINARY_NAME)"

clean:
	@echo "Cleaning build files..."
	@rm -rf $(BUILD_DIR)
	@rm -rf $(STATE_DIR)

run: build
	@echo "Starting {{.ServiceTitle}} probe..."
	@cd $(BUILD_DIR) && ./$(BINARY_NAME)

dev: deps
	@echo "Starting in development mode..."
	$(GO) run ./cmd

test: deps
	@echo "Running tests..."
	$(GO) test -v ./...

fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...

help:
	@echo "Available commands:"
	@echo "  deps          - Install dependencies"
	@echo "  build         - Build the probe"
	@echo "  clean         - Clean build files"
	@echo "  run           - Run the probe"
	@echo "  dev           - Development mode"
	@echo "  test          - Run tests"
	@echo "  fmt           - Format code"
	@echo "  help          - Show this help"
`

const readmeTemplate = `# Fluid {{.ServiceTitle}} Probe

A Go-based probe for retrieving and managing {{.ServiceTitle}} data as part of the Fluid project.

## Features

- **{{.ServiceTitle}} Integration**: Connects to {{.ServiceTitle}} API
- **State Management**: Automatically manages historical states with configurable retention
- **YAML Output**: Saves state data in YAML format for easy processing
- **Configurable**: Flexible configuration via YAML files

## Configuration

### Probe Configuration (` + "`config/probe.yml`" + `)

See ` + "`config/probe.example.yml`" + ` for a complete example.

### Environment Variables

- ` + "`FLUID_{{.ServiceTitleUpper}}_TOKEN`" + `: Your {{.ServiceTitle}} API token

## Building and Running

### Prerequisites

- Go 1.23 or later
- {{.ServiceTitle}} API credentials

### Build

` + "```bash" + `
make build
` + "```" + `

### Run

` + "```bash" + `
make dev
` + "```" + `

## Architecture

` + "```" + `
cmd/main.go              # Application entry point
├── internal/
│   ├── probe/           # Probe lifecycle management
│   ├── config/          # Configuration loading and validation
│   ├── {{.ServiceName}}/      # {{.ServiceTitle}} API client
│   ├── models/          # Data structures
│   └── manager/         # State persistence
├── config/              # Configuration files
└── state/               # Output directory for states
` + "```" + `
`

const gitignoreTemplate = `# Secrets
*.secrets
env.secrets

# Build artifacts
build/
{{.BinaryName}}
*.exe

# State files
state/

# IDE
.idea/
.vscode/
*.swp
*.swo
*~
`

const toolVersionsTemplate = `golang 1.23.0
`

const envSecretsTemplate = `# {{.ServiceTitle}} Probe Credentials
# Source this file with: source env.secrets
# This file is ignored by git

# {{.ServiceTitle}} API Token
export FLUID_{{.ServiceTitleUpper}}_TOKEN="your_token_here"
`

type ProbeInfo struct {
	Name              string
	PackageName       string
	ModulePath        string
	ServiceName       string
	ServiceTitle      string
	ServiceTitleUpper string
	EntityName        string
	EntityTitle       string
	BinaryName        string
}

func Generate(probeName string) error {
	serviceTitle := toTitleCase(probeName)
	info := &ProbeInfo{
		Name:              probeName,
		PackageName:       strings.ToLower(strings.ReplaceAll(probeName, "-", "")),
		ModulePath:        fmt.Sprintf("fluid/probes/%s", probeName),
		ServiceName:       strings.ToLower(strings.ReplaceAll(probeName, "-", "")),
		ServiceTitle:      serviceTitle,
		ServiceTitleUpper: strings.ToUpper(strings.ReplaceAll(serviceTitle, "-", "_")),
		EntityName:        "example",
		EntityTitle:       "Example",
		BinaryName:        fmt.Sprintf("%s-probe", probeName),
	}

	baseDir := filepath.Join("..", probeName)
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return fmt.Errorf("error creating base directory: %w", err)
	}

	files := []struct {
		path     string
		template string
	}{
		{"cmd/main.go", mainTemplate},
		{"internal/probe/probe.go", probeTemplate},
		{"internal/probe/entities/example_entity.go", entityTemplate},
		{"internal/config/config.go", configTemplate},
		{"internal/manager/manager.go", managerTemplate},
		{"internal/models/models.go", modelsTemplate},
		{"internal/" + info.ServiceName + "/client.go", clientTemplate},
		{"config/probe.yml", configYamlTemplate},
		{"config/probe.example.yml", configExampleYamlTemplate},
		{"go.mod", goModTemplate},
		{"Makefile", makefileTemplate},
		{"README.md", readmeTemplate},
		{".gitignore", gitignoreTemplate},
		{".tool-versions", toolVersionsTemplate},
		{"env.secrets", envSecretsTemplate},
	}

	for _, f := range files {
		if err := generateFile(baseDir, f.path, f.template, info); err != nil {
			return fmt.Errorf("error generating %s: %w", f.path, err)
		}
	}

	return nil
}

func generateFile(baseDir, filePath, tmpl string, info *ProbeInfo) error {
	fullPath := filepath.Join(baseDir, filePath)
	dir := filepath.Dir(fullPath)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating directory %s: %w", dir, err)
	}

	t, err := template.New("file").Funcs(template.FuncMap{
		"upper": strings.ToUpper,
	}).Parse(tmpl)
	if err != nil {
		return fmt.Errorf("error parsing template: %w", err)
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("error creating file: %w", err)
	}
	defer file.Close()

	if err := t.Execute(file, info); err != nil {
		return fmt.Errorf("error executing template: %w", err)
	}

	return nil
}

func toTitleCase(s string) string {
	parts := strings.Split(s, "-")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
		}
	}
	return strings.Join(parts, "")
}
