package ninerouter

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// The schema exists so the shape asked for and the shape parsed are one
// declaration. These tests hold that: a field added to the struct must appear
// in the prompt, and nothing may reach a model as an escape sequence.

func TestSchemaCoversEveryField(t *testing.T) {
	t.Parallel()
	schema, err := jsonSchemaOf(blueprintDoc{})
	if err != nil {
		t.Fatalf("jsonSchemaOf: %v", err)
	}
	for _, name := range jsonFieldNames(t, blueprintDoc{}, chapterDoc{}) {
		if !strings.Contains(schema, `"`+name+`"`) {
			t.Errorf("the schema is missing %q, so the model is never asked for it", name)
		}
	}
}

// The identity fields are the caller's to stamp. Asking a model for a video id
// gets you an invented one, so they live on storedBlueprint and must not reach
// the prompt.
func TestSchemaOmitsTheStampedFields(t *testing.T) {
	t.Parallel()
	schema, err := jsonSchemaOf(blueprintDoc{})
	if err != nil {
		t.Fatalf("jsonSchemaOf: %v", err)
	}
	for _, name := range []string{"video", "ref"} {
		if strings.Contains(schema, `"`+name+`"`) {
			t.Errorf("the schema asks the model for %q, which the caller stamps", name)
		}
	}
	// But the stored document still carries them, or the asset loses its identity.
	for _, name := range []string{"video", "ref"} {
		if !hasJSONField(storedBlueprint{}, name) {
			t.Errorf("storedBlueprint dropped %q", name)
		}
	}
}

// Descriptions are the prompt's field hints; a schema without them is a shape
// with no instructions attached.
func TestSchemaCarriesTheFieldDescriptions(t *testing.T) {
	t.Parallel()
	schema, err := jsonSchemaOf(blueprintDoc{})
	if err != nil {
		t.Fatalf("jsonSchemaOf: %v", err)
	}
	for _, want := range []string{
		"1-based position in the outline",
		"letters and spaces only",
		"the first is the opening directive",
		"light | curious | unsettling | dark",
	} {
		if !strings.Contains(schema, want) {
			t.Errorf("the schema lost the hint %q", want)
		}
	}
}

// A model reading <style> where the hint says <style> follows the
// escape, not the instruction.
func TestSchemaIsNotHTMLEscaped(t *testing.T) {
	t.Parallel()
	schema, err := jsonSchemaOf(blueprintDoc{})
	if err != nil {
		t.Fatalf("jsonSchemaOf: %v", err)
	}
	// Written as concatenations: a Go literal would decode the escape and this
	// would assert the opposite of what it means to.
	for _, escaped := range []string{`\u003` + "c", `\u003` + "e", `\u002` + "6"} {
		if strings.Contains(schema, escaped) {
			t.Errorf("%s reached the prompt as an escape sequence:\n%s", escaped, schema)
		}
	}
	if !strings.Contains(schema, "<style>") {
		t.Error("the opening-directive hint lost its placeholder")
	}
}

// It has to be valid JSON and stand on its own: a prompt has nothing to resolve
// a dangling $ref against.
func TestSchemaIsSelfContained(t *testing.T) {
	t.Parallel()
	schema, err := jsonSchemaOf(blueprintDoc{})
	if err != nil {
		t.Fatalf("jsonSchemaOf: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(schema), &doc); err != nil {
		t.Fatalf("the schema is not valid JSON: %v", err)
	}
	defs, ok := doc["$defs"].(map[string]any)
	if !ok || len(defs) == 0 {
		t.Fatal("the nested chapter type was not inlined under $defs")
	}
	for _, ref := range refsIn(doc) {
		if !strings.HasPrefix(ref, "#/$defs/") {
			t.Errorf("$ref %q points outside the document", ref)
		}
		if _, found := defs[strings.TrimPrefix(ref, "#/$defs/")]; !found {
			t.Errorf("$ref %q resolves to nothing", ref)
		}
	}
}

// jsonFieldNames reads the json tags off the given structs.
func jsonFieldNames(t *testing.T, values ...any) []string {
	t.Helper()
	var out []string
	for _, v := range values {
		typ := reflect.TypeOf(v)
		for i := range typ.NumField() {
			if name := jsonName(typ.Field(i)); name != "" {
				out = append(out, name)
			}
		}
	}
	return out
}

func hasJSONField(v any, name string) bool {
	typ := reflect.TypeOf(v)
	for i := range typ.NumField() {
		if jsonName(typ.Field(i)) == name {
			return true
		}
	}
	return false
}

func jsonName(f reflect.StructField) string {
	tag, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	if tag == "-" {
		return ""
	}
	return tag
}

// refsIn collects every $ref anywhere in the document.
func refsIn(v any) []string {
	var out []string
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if k == "$ref" {
				if ref, ok := child.(string); ok {
					out = append(out, ref)
				}
				continue
			}
			out = append(out, refsIn(child)...)
		}
	case []any:
		for _, child := range t {
			out = append(out, refsIn(child)...)
		}
	}
	return out
}
