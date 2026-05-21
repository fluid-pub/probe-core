package core

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Schema represents the probe's data schema
type Schema struct {
	Entities map[string]EntitySchema `yaml:"entities"`
}

// EntitySchema represents the schema for a single entity
type EntitySchema struct {
	Description string                 `yaml:"description"`
	Fields      map[string]FieldSchema `yaml:"fields"`
}

// FieldSchema represents the schema for a single field
type FieldSchema struct {
	Type        string `yaml:"type"`
	Description string `yaml:"description"`
	Format      string `yaml:"format,omitempty"`
	Primary     bool   `yaml:"primary,omitempty"`
	Frequency   string `yaml:"frequency,omitempty"` // For cost type fields: frequency of the cost (default: "monthly")
}

// LoadSchema loads the schema from a YAML file
// It searches for the file in the following order:
// 1. At the specified path (if provided)
// 2. In the same directory as the config file (if configPath is provided)
// 3. In the executable's directory
// 4. In the current working directory
func LoadSchema(schemaPath ...string) (*Schema, error) {
	var schemaFile string

	// If a path is provided, use it
	if len(schemaPath) > 0 && schemaPath[0] != "" {
		schemaFile = schemaPath[0]
	} else {
		// Try to find schema.yml relative to the executable
		execPath, err := os.Executable()
		if err == nil {
			execDir := filepath.Dir(execPath)
			schemaFile = filepath.Join(execDir, "config", "schema.yml")
		}

		// If not found in executable directory, try current directory
		if schemaFile == "" || !fileExists(schemaFile) {
			schemaFile = filepath.Join("config", "schema.yml")
		}

		// If still not found, try parent directory (for development)
		if !fileExists(schemaFile) {
			schemaFile = filepath.Join("..", "config", "schema.yml")
		}
	}

	// Check if file exists
	if !fileExists(schemaFile) {
		return nil, fmt.Errorf("schema file not found at %s", schemaFile)
	}

	data, err := os.ReadFile(schemaFile)
	if err != nil {
		return nil, fmt.Errorf("error reading schema file: %w", err)
	}

	var schema Schema
	if err := yaml.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("error parsing schema file: %w", err)
	}

	return &schema, nil
}

// LoadSchemaFromConfigDir loads the schema from the same directory as the config file
func LoadSchemaFromConfigDir(configPath string) (*Schema, error) {
	configDir := filepath.Dir(configPath)
	schemaFile := filepath.Join(configDir, "schema.yml")
	return LoadSchema(schemaFile)
}

// ToMap converts the schema to a map[string]interface{} for JSON serialization
func (s *Schema) ToMap() map[string]interface{} {
	entities := make(map[string]interface{})
	for entityName, entitySchema := range s.Entities {
		entityMap := map[string]interface{}{
			"description": entitySchema.Description,
			"fields":      make(map[string]interface{}),
		}
		fields := entityMap["fields"].(map[string]interface{})
		for fieldName, fieldSchema := range entitySchema.Fields {
			fieldMap := map[string]interface{}{
				"type":        fieldSchema.Type,
				"description": fieldSchema.Description,
			}
			if fieldSchema.Format != "" {
				fieldMap["format"] = fieldSchema.Format
			}
			if fieldSchema.Primary {
				fieldMap["primary"] = true
			}
			if fieldSchema.Frequency != "" {
				fieldMap["frequency"] = fieldSchema.Frequency
			}
			fields[fieldName] = fieldMap
		}
		entities[entityName] = entityMap
	}
	return entities
}
