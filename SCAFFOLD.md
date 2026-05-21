# Probe Scaffold Generator

The core package includes a CLI tool to generate a complete probe scaffold with all necessary boilerplate code and structure.

## Building the CLI

```bash
cd code/agents/core
make build
```

This will create the `core` binary in the `build/` directory.

## Usage

Generate a new probe scaffold:

```bash
./build/core scaffold -name <probe-name>
```

**Example:**

```bash
./build/core scaffold -name aws-probe
```

This will create a new directory `../aws-probe/` (relative to the core directory) with the complete probe structure.

## Generated Structure

The scaffold generates the following structure:

```text
<probe-name>/
├── cmd/
│   └── main.go                    # Application entry point
├── internal/
│   ├── probe/
│   │   ├── probe.go              # Probe wrapper around core
│   │   └── entities/
│   │       └── example_entity.go # Example entity implementation
│   ├── config/
│   │   └── config.go             # Configuration loading and validation
│   ├── manager/
│   │   └── manager.go            # State manager wrapper
│   ├── models/
│   │   └── models.go             # Data models
│   └── <service-name>/
│       └── client.go             # Service API client
├── config/
│   ├── probe.yml                 # Main configuration file
│   └── probe.example.yml        # Example configuration
├── go.mod                        # Go module definition
├── Makefile                      # Build automation
├── README.md                     # Probe documentation
├── .gitignore                    # Git ignore rules
├── .tool-versions                # Go version (1.23)
└── env.secrets                   # Environment variables (gitignored)
```

## Next Steps After Generation

1. **Navigate to the generated probe directory:**

   ```bash
   cd ../<probe-name>
   ```

2. **Configure credentials:**
   - Edit `env.secrets` with your service API credentials
   - This file is automatically gitignored and contains sensitive data
   - Source it with: `source env.secrets` before running the probe
   - Use environment variables in `config/probe.yml` (e.g., `${SERVICE_TOKEN}`)

3. **Update the configuration:**
   - Edit `config/probe.yml` with your service-specific configuration
   - Reference environment variables from `env.secrets` using `${VARIABLE_NAME}` syntax

4. **Implement your entities:**
   - Edit or create new entity files in `internal/probe/entities/`
   - Each entity must implement the `core.Entity` interface:
     - `Name() string`
     - `Refresh(client core.Client) (interface{}, error)`
     - `Save(stateManager core.StateManager, data interface{}) error`

5. **Implement your service client:**
   - Edit `internal/<service-name>/client.go`
   - Implement methods to interact with your service API
   - The client will be passed to entities via the `Refresh()` method

6. **Update data models:**
   - Edit `internal/models/models.go` to define your data structures
   - Models should include YAML and JSON tags for serialization

7. **Register entities:**
   - In `internal/probe/probe.go`, register your entities:

     ```go
     coreAgent.RegisterEntity(entities.NewYourEntity())
     ```

8. **Start developing:**

   ```bash
   make dev
   ```

## Configuration Template

The generated `config/probe.yml` follows this structure:

```yaml
probe:
  name: "<probe-name>"
  version: "1.0.0"

<service-name>:
  api_url: "https://api.example.com"
  token: "${FLUID_SERVICE_TOKEN}"

data:
  entities:
    - name: example
      refresh_interval: "1h"
      retention_frequencies:
        days: 30
        months: 12

state:
  dir: "state"
  format: "yaml"
  cleanup_interval: 1
```

## Entity Configuration

Each entity in the configuration can specify:

- **`name`** (required): Unique name for the entity
- **`refresh_interval`** (required): How often to refresh this entity (e.g., `"5m"`, `"1h"`, `"8h"`)
- **`retention_frequencies`** (optional): Which historical state files to keep:
  - `seconds`, `minutes`, `hours`, `days`, `weeks`, `months`, `years`
  - Each frequency defines how many states to keep (0 = unlimited)

## Example: Creating an AWS Probe

```bash
cd code/agents/core
make build
./build/core scaffold -name aws-probe
cd ../aws-probe

# Edit config/probe.yml with AWS credentials
# Implement entities in internal/probe/entities/
# Implement AWS client in internal/aws/client.go
# Register entities in internal/probe/probe.go

make dev
```

## Customization

After generation, you can customize:

- **Service client**: Implement your API client in `internal/<service-name>/client.go`
- **Entities**: Create multiple entities in `internal/probe/entities/`
- **Models**: Define your data structures in `internal/models/models.go`
- **Configuration**: Add service-specific configuration fields in `internal/config/config.go`

## Environment Variables (env.secrets)

The scaffold generates an `env.secrets` file that is automatically gitignored. This file contains sensitive credentials and should never be committed to version control.

**Format:**

```bash
# Service Probe Credentials
# Source this file with: source env.secrets
# This file is ignored by git

# Service API Token
export FLUID_SERVICE_TOKEN="your_token_here"
```

**Usage:**

1. Edit `env.secrets` with your actual credentials
2. Source it before running the probe:

   ```bash
   source env.secrets
   make dev
   ```

3. Reference variables in `config/probe.yml` using `${VARIABLE_NAME}` syntax

## Notes

- The scaffold generates code that uses the core framework
- All generated code follows Go best practices and conventions
- The probe structure is ready to use with minimal modifications
- Environment variables are supported in configuration files using `${VAR_NAME}` syntax
- The `env.secrets` file is automatically gitignored and should contain all sensitive credentials