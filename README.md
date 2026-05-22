# Fluid Agents Core

Release notes: [CHANGELOG.md](CHANGELOG.md).

Core framework for building Fluid agents. Provides common functionality for probe lifecycle management, entity handling, state persistence, and automatic cleanup.

## Overview

The core package provides:

- **Probe lifecycle management**: Start, stop, and orchestrate entity processes
- **Entity system**: Interface and registry for managing data entities
- **State management**: Automatic state persistence with timestamped files and retention policies
- **Configuration**: Standardized configuration structure for all agents
- **CLI scaffold generator**: Tool to generate new probe scaffolds

## Architecture

### Components

- **Probe** (`probe.go`): Base probe with lifecycle management
- **Entity** (`entity.go`): Interface for entities that can be refreshed
- **StateManager** (`state/manager.go`): State persistence and cleanup
- **EntityRegistry** (`registry.go`): Centralized entity registry
- **Config** (`config.go`): Standardized configuration structures

## Usage

### As a Library

Import the core package in your probe:

```go
import "fluid/probes/core"
```

Create an probe:

```go
coreAgent := core.NewAgent(config, client, stateManager)
coreAgent.RegisterEntity(entities.NewMyEntity())
coreAgent.Start()
```

### Generating a New Probe

Use the CLI scaffold generator to create a new probe:

```bash
make build
./build/core scaffold -name <probe-name>
```

See [SCAFFOLD.md](./SCAFFOLD.md) for detailed documentation on generating new agents.

## Building

```bash
make build
```

This builds the CLI tool in `build/core`.

## Documentation

- **[SCAFFOLD.md](./SCAFFOLD.md)**: Guide for generating new probe scaffolds
- **[Technical Documentation](../../../design/technique/agents.md)**: Complete architecture documentation

## Requirements

- Go 1.23 or later
