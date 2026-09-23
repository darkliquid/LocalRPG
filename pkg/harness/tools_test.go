package harness

import (
	"strings"
	"testing"
)

func TestToolSpecsDescribeEveryTool(t *testing.T) {
	specs := ToolSpecs()
	names := make(map[string]ToolSpec, len(specs))
	for _, spec := range specs {
		names[spec.Name] = spec
		if spec.Description == "" {
			t.Errorf("%s has no description", spec.Name)
		}
		if spec.Parameters["type"] != "object" {
			t.Errorf("%s parameters are not a JSON object schema", spec.Name)
		}
	}

	for _, want := range []string{"search_entities", "get_entity", "graph_neighbours", "search_timeline"} {
		if _, ok := names[want]; !ok {
			t.Errorf("tool table is missing %q", want)
		}
	}

	search := names["search_entities"]
	required, _ := search.Parameters["required"].([]string)
	if len(required) == 0 || required[0] != "query" {
		t.Errorf("search_entities required = %v, want query", required)
	}
}

func TestUnknownToolMessageListsTheSurface(t *testing.T) {
	message := UnknownToolMessage("teleport")
	if !strings.Contains(message, "teleport") {
		t.Errorf("message = %q, want the offending name", message)
	}
	for _, want := range ToolNames() {
		if !strings.Contains(message, want) {
			t.Errorf("message is missing the available tool %q: %s", want, message)
		}
	}
}
