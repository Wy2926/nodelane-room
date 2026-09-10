package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRegenerationPreservesContract(t *testing.T) {
	source, err := os.ReadFile("../../docs/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	if err := os.Mkdir("docs", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("docs/openapi.yaml", source, 0644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := run(); err != nil {
			t.Fatal(err)
		}
		generated, err := os.ReadFile("docs/openapi.yaml")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(source, generated) {
			t.Fatal("contract is stale or regeneration is unstable; run go run ./scripts/openapi")
		}
	}
	var doc M
	if err := yaml.Unmarshal(source, &doc); err != nil {
		t.Fatal(err)
	}
	var check func(any)
	check = func(value any) {
		switch v := value.(type) {
		case M:
			if ref, ok := v["$ref"].(string); ok {
				if !strings.HasPrefix(ref, "#/") {
					t.Fatalf("unsupported reference: %s", ref)
				}
				var target any = doc
				for _, key := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
					m, ok := target.(M)
					if !ok || m[key] == nil {
						t.Fatalf("unresolved reference: %s", ref)
					}
					target = m[key]
				}
			}
			for _, child := range v {
				check(child)
			}
		case []any:
			for _, child := range v {
				check(child)
			}
		}
	}
	check(doc)
}
