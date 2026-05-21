package core

import (
	"testing"
)

func TestSchemaToMap_usable_in_rag(t *testing.T) {
	s := &Schema{
		Entities: map[string]EntitySchema{
			"pages": {
				Description: "Pages",
				Fields: map[string]FieldSchema{
					"title": {
						Type:        "string",
						Description: "Title",
						UsableInRAG: true,
					},
					"id": {
						Type:        "string",
						Description: "ID",
						Primary:     true,
					},
				},
			},
		},
	}

	m := s.ToMap()
	entities, ok := m["pages"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected pages entity, got %#v", m)
	}
	fields, ok := entities["fields"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected fields map, got %#v", entities)
	}
	title, ok := fields["title"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected title field, got %#v", fields)
	}
	if title["usable_in_rag"] != true {
		t.Fatalf("expected usable_in_rag on title, got %#v", title)
	}
	if _, has := title["nullable"]; has {
		t.Fatalf("nullable should be omitted when false")
	}
}
