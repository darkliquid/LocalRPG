package gui

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/telemetry"
	"github.com/darkliquid/localrpg/pkg/trace"
)

func TestRecordImageEmitsEventsAndDuration(t *testing.T) {
	recorder, _, err := telemetry.NewInMemory()
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	defer telemetry.ResetGlobalForTest()

	mem := trace.NewMemory(trace.LevelSummary)
	service := &Service{logger: mem}
	ctx, span := startImageSpan(context.Background(), service.logger, "banner")
	service.recordImage(ctx, span, "banner", "gemini", 1024, time.Now(), nil)
	span.End()

	var sawBytes bool
	for _, attr := range recorder.Spans()[0].Attributes() {
		if attr.Key == attribute.Key("localrpg.image.bytes") && attr.Value.AsInt64() == 1024 {
			sawBytes = true
		}
	}
	if !sawBytes {
		t.Fatal("localrpg.image.bytes was not set")
	}
	if _, ok := mem.Find("generate.complete"); !ok {
		t.Fatalf("events = %v, want generate.complete", mem.Names())
	}
}

func TestRecordImageFailureEmitsGenerateError(t *testing.T) {
	mem := trace.NewMemory(trace.LevelSummary)
	service := &Service{logger: mem}
	ctx, span := startImageSpan(context.Background(), service.logger, "icon")
	service.recordImage(ctx, span, "icon", "", 0, time.Now(), &harness.GenerationFailure{Code: harness.FailureProviderError, Message: "no data"})
	span.End()
	if _, ok := mem.Find("generate.error"); !ok {
		t.Fatalf("events = %v, want generate.error", mem.Names())
	}
}

type stubImageClient struct {
	data []byte
	err  error
}

func (c stubImageClient) GenerateImage(context.Context, string) ([]byte, error) {
	return c.data, c.err
}

func TestGenerateImageRejectsEmptyBytes(t *testing.T) {
	_, svc := setupTestGame(t)
	original := imageClientFactory
	defer func() { imageClientFactory = original }()
	imageClientFactory = func(config.ImageConfig, string) (media.ImageClient, error) {
		return stubImageClient{data: nil}, nil
	}

	_, failure := svc.generateImage(context.Background(), "banner", "a banner", "")
	if failure == nil || failure.Code != harness.FailureProviderError {
		t.Fatalf("generateImage failure = %v, want provider_error", failure)
	}
}
