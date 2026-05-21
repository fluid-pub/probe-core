package core

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// RAGFieldSet maps each entity type (e.g. pages) to schema field names marked usable_in_rag.
// Only those fields may have fields.<name>.rag: true in the probe config.
type RAGFieldSet map[string]map[string]struct{}

// schemaYAMLRoot is the minimal schema.yml shape (entities.*.fields.*.usable_in_rag).
type schemaYAMLRoot struct {
	Entities map[string]schemaYAMLEntity `yaml:"entities"`
}

type schemaYAMLEntity struct {
	Fields map[string]schemaYAMLField `yaml:"fields"`
}

type schemaYAMLField struct {
	UsableInRAG bool `yaml:"usable_in_rag"`
}

// ParseRAGFieldSetFromSchemaYAML extracts RAG-eligible fields from a schema.yml file.
func ParseRAGFieldSetFromSchemaYAML(data []byte) (RAGFieldSet, error) {
	var root schemaYAMLRoot
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse schema yaml: %w", err)
	}
	out := make(RAGFieldSet)
	for entName, ent := range root.Entities {
		for fieldName, fld := range ent.Fields {
			if fld.UsableInRAG {
				if out[entName] == nil {
					out[entName] = make(map[string]struct{})
				}
				out[entName][fieldName] = struct{}{}
			}
		}
	}
	return out, nil
}

// ValidateRAGEntityFields ensures every fields.<name>.rag: true matches a usable_in_rag field
// in the schema for that entity. If allowed is nil, validation is skipped.
func ValidateRAGEntityFields(entities []EntityConfig, allowed RAGFieldSet) error {
	if allowed == nil {
		return nil
	}
	for _, e := range entities {
		if e.Fields == nil {
			continue
		}
		for fieldName, fc := range e.Fields {
			if !fc.RAG {
				continue
			}
			entFields, ok := allowed[e.Name]
			if !ok {
				return fmt.Errorf("config: entity %q has fields.%s.rag=true but this entity has no RAG-eligible fields in schema", e.Name, fieldName)
			}
			if _, ok := entFields[fieldName]; !ok {
				return fmt.Errorf("config: entity %q has fields.%s.rag=true but schema does not mark %q as usable_in_rag", e.Name, fieldName, fieldName)
			}
		}
	}
	return nil
}
