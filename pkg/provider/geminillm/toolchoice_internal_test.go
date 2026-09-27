package geminillm

import (
	"testing"

	"google.golang.org/genai"

	"github.com/darkliquid/localrpg/pkg/harness"
)

func TestBuildGenerateConfigMapsToolChoice(t *testing.T) {
	g := &GeminiProvider{model: "m"}

	required := g.buildGenerateConfig(harness.GenerateRequest{ToolChoice: "required"})
	if required.ToolConfig == nil || required.ToolConfig.FunctionCallingConfig == nil ||
		required.ToolConfig.FunctionCallingConfig.Mode != genai.FunctionCallingConfigModeAny {
		t.Fatalf("required tool config = %+v", required.ToolConfig)
	}

	none := g.buildGenerateConfig(harness.GenerateRequest{ToolChoice: "none"})
	if none.ToolConfig == nil || none.ToolConfig.FunctionCallingConfig == nil ||
		none.ToolConfig.FunctionCallingConfig.Mode != genai.FunctionCallingConfigModeNone {
		t.Fatalf("none tool config = %+v", none.ToolConfig)
	}

	auto := g.buildGenerateConfig(harness.GenerateRequest{})
	if auto.ToolConfig != nil {
		t.Fatalf("default should set no tool config, got %+v", auto.ToolConfig)
	}
}
