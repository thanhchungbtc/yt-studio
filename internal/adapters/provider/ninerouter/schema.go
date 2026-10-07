package ninerouter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// jsonSchemaOf renders a Go type as a self-contained JSON Schema for a prompt
// to hand a model, so the shape asked for and the shape parsed are one
// declaration. huma already reflects this project's structs for OpenAPI.
func jsonSchemaOf(v any) (string, error) {
	// $defs rather than huma's OpenAPI components path: the result must stand on
	// its own inside a prompt, with nothing to resolve references against.
	registry := huma.NewMapRegistry("#/$defs/", huma.DefaultSchemaNamer)
	schema := huma.SchemaFromType(registry, reflect.TypeOf(v))

	assembled := map[string]any{"root": schema}
	if defs := registry.Map(); len(defs) > 0 {
		assembled["defs"] = defs
	}

	// Round-tripped through plain maps first: huma.Schema has its own
	// MarshalJSON, whose output is copied through verbatim, so its escaping
	// cannot be switched off from outside.
	raw, err := json.Marshal(assembled)
	if err != nil {
		return "", fmt.Errorf("encode schema: %w", err)
	}
	var plain struct {
		Root map[string]any `json:"root"`
		Defs map[string]any `json:"defs"`
	}
	if err := json.Unmarshal(raw, &plain); err != nil {
		return "", fmt.Errorf("decode schema: %w", err)
	}
	doc := plain.Root
	if len(plain.Defs) > 0 {
		doc["$defs"] = plain.Defs
	}

	// HTML escaping would show a model an escape sequence where its instructions
	// say <style>. This is a prompt, not a web page.
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return "", fmt.Errorf("render schema: %w", err)
	}
	return strings.TrimRight(out.String(), "\n"), nil
}
