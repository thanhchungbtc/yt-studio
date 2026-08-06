package ninerouter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// jsonSchemaOf renders a Go type as a self-contained JSON Schema, for a prompt
// to hand a model as its output contract.
//
// The point is that the shape asked for and the shape parsed are one
// declaration. A field added to the struct appears in the prompt on the next
// call; a hand-written block would have to be remembered, and would not be.
//
// huma is already the dependency that turns this project's structs into the
// OpenAPI document, so the same reflection serves both.
func jsonSchemaOf(v any) (string, error) {
	// $defs rather than huma's OpenAPI components path: the result has to stand
	// on its own inside a prompt, with nothing to resolve references against.
	registry := huma.NewMapRegistry("#/$defs/", huma.DefaultSchemaNamer)
	schema := huma.SchemaFromType(registry, reflect.TypeOf(v))

	assembled := map[string]any{"root": schema}
	if defs := registry.Map(); len(defs) > 0 {
		assembled["defs"] = defs
	}

	// Round-tripped through plain maps before the final encode. huma.Schema has
	// its own MarshalJSON, and bytes a custom marshaller produced are copied
	// through verbatim — so the escaping it applies cannot be switched off from
	// outside. Decoding first replaces those types with maps the encoder below
	// actually formats.
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
