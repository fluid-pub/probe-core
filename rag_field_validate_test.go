package core

import (
	"testing"
)

func TestParseRAGFieldSetFromSchemaYAML(t *testing.T) {
	y := `
entities:
  pages:
    fields:
      title:
        usable_in_rag: true
      body:
        usable_in_rag: true
      status:
        type: string
`
	set, err := ParseRAGFieldSetFromSchemaYAML([]byte(y))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := set["pages"]["title"]; !ok {
		t.Fatal("expected pages.title")
	}
	if _, ok := set["pages"]["body"]; !ok {
		t.Fatal("expected pages.body")
	}
	if _, ok := set["pages"]["status"]; ok {
		t.Fatal("did not expect pages.status")
	}
}

func TestValidateRAGEntityFields_ok(t *testing.T) {
	allowed := RAGFieldSet{
		"pages": {"body": {}, "title": {}},
	}
	entities := []EntityConfig{
		{
			Name:            "pages",
			RefreshInterval: "5m",
			Fields: map[string]EntityFieldConfig{
				"body": {RAG: true},
			},
		},
	}
	if err := ValidateRAGEntityFields(entities, allowed); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRAGEntityFields_unknownField(t *testing.T) {
	allowed := RAGFieldSet{"pages": {"body": {}}}
	entities := []EntityConfig{
		{
			Name:            "pages",
			RefreshInterval: "5m",
			Fields: map[string]EntityFieldConfig{
				"foo": {RAG: true},
			},
		},
	}
	if err := ValidateRAGEntityFields(entities, allowed); err == nil {
		t.Fatal("expected error")
	}
}
