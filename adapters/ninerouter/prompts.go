package ninerouter

import (
	"embed"
	"fmt"
	"strings"
	"text/template"
)

// promptFS holds the prompt templates. They are embedded rather than read from
// disk because they are part of the binary's behaviour: a backend whose prompts
// can be edited underneath it is a backend whose output nobody can reproduce.
//
//go:embed prompts/*.tmpl
var promptFS embed.FS

// prompts is parsed once at init. The templates are compile-time constants and
// a broken one is a build-time mistake, so failing loudly here beats returning
// an error every caller would have to invent a response to.
var prompts = template.Must(template.ParseFS(promptFS, "prompts/*.tmpl"))

// Template names, which text/template takes from the file's base name.
const (
	blueprintSystemPrompt = "blueprint.system.tmpl"
	blueprintUserPrompt   = "blueprint.user.tmpl"
	scriptSystemPrompt    = "script.system.tmpl"
	scriptUserPrompt      = "script.user.tmpl"

	imagePromptsSystemPrompt = "imageprompts.system.tmpl"
	imagePromptsUserPrompt   = "imageprompts.user.tmpl"
)

// render executes one template against its request.
func render(name string, data any) (string, error) {
	var buf strings.Builder
	if err := prompts.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("render %s: %w", name, err)
	}
	return strings.TrimSpace(buf.String()), nil
}
