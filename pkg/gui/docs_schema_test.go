package gui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
)

const configurationReferencePath = "docs/13-configuration-reference.md"

// renderConfigurationReference reflects over the config structs so the key
// list in the documentation cannot drift from the keys the loader accepts.
func renderConfigurationReference() string {
	var b strings.Builder

	b.WriteString("---\n")
	b.WriteString("id: 13-configuration-reference\n")
	b.WriteString("title: Configuration Reference\n")
	b.WriteString("category: Configuration & Providers\n")
	b.WriteString("order: 7\n")
	b.WriteString("description: Every key LocalRPG reads from config.yaml, with the accepted values for the enumerated ones.\n")
	b.WriteString("---\n\n")

	b.WriteString("# Configuration Reference\n\n")
	b.WriteString("This page is generated from the configuration structs, so it always lists the\n")
	b.WriteString("keys the loader accepts. The file is `config.yaml` in the user config\n")
	b.WriteString("directory, merged with an optional `./localrpg.yaml` in the project root.\n\n")

	b.WriteString("## Keys\n\n")
	b.WriteString("A `[]` suffix marks a list of tables. `<key>` and `<role>` are names you\n")
	b.WriteString("choose; the ledger and provider identifiers are listed in the\n")
	b.WriteString("[Provider & Model Catalogue](12-provider-catalogue).\n\n")

	root := reflect.TypeOf(config.Config{})
	for i := 0; i < root.NumField(); i++ {
		field := root.Field(i)
		if field.PkgPath != "" {
			continue
		}
		name := yamlFieldName(field)
		if name == "" || name == "-" {
			continue
		}
		entries := walkSchema(name, field.Type)
		if len(entries) == 0 {
			continue
		}
		fmt.Fprintf(&b, "### `%s`\n\n", name)
		b.WriteString("| Key | Type |\n")
		b.WriteString("| --- | --- |\n")
		for _, entry := range entries {
			fmt.Fprintf(&b, "| `%s` | %s |\n", entry.key, entry.typ)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Value sets\n\n")
	b.WriteString("Keys typed `string` above accept the following values where noted. An\n")
	b.WriteString("unlisted value is either rejected or falls back to the documented default.\n\n")
	b.WriteString("| Key | Accepted values |\n")
	b.WriteString("| --- | --- |\n")
	b.WriteString("| `agents.default_role` | `gm`, `narrator`, `extractor`, `completion`, or any role defined in `agents.roles` |\n")
	b.WriteString("| `agents.roles.<role>.type` | `builtin`, `http`, `cli`, `gemini`, `inherit`, `disabled` |\n")
	b.WriteString("| `agents.roles.<role>.supports_tools` | `auto`, `yes`, `no` |\n")
	b.WriteString("| `agents.completion.mode` | `auto`, `continue`, `trim`, `off` |\n")
	b.WriteString("| `media.tts.type` | `builtin`, `http`, `cli`, `gemini`, `disabled` |\n")
	b.WriteString("| `media.tts.markdown` | `auto`, `strip`, `keep` |\n")
	b.WriteString("| `media.stt.type` | `builtin`, `http`, `cli`, `web-speech`, `disabled` |\n")
	b.WriteString("| `media.image.type` | `builtin`, `http`, `cli`, `comfyui`, `gemini`, `disabled` |\n")
	b.WriteString("| `embeddings.provider` | `builtin-local`, `openai`, `gemini`, `disabled` |\n")
	b.WriteString("| `embeddings.providers.<id>.type` | `builtin`, `http`, `gemini`, `disabled` |\n")
	b.WriteString("| `preferences.font_scale` | `small`, `medium`, `large` |\n")
	b.WriteString("| `preferences.trace_level` | `off`, `summary`, `full` |\n")
	b.WriteString("| `mechanics.engagement` | `off`, `auto`, `ask` |\n")

	return b.String()
}

func TestConfigurationReferenceIsCurrent(t *testing.T) {
	assertGeneratedDoc(t, configurationReferencePath, renderConfigurationReference())
}
