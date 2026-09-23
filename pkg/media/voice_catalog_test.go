package media

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/entity"
)

// catalogClient is a TTSClient that also enumerates voices, with a fail switch.
type catalogClient struct {
	voices []ProviderVoice
	calls  int
	fail   bool
}

func (c *catalogClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	return nil, nil
}

func (c *catalogClient) ListVoices(ctx context.Context) ([]ProviderVoice, error) {
	c.calls++
	if c.fail {
		return nil, errors.New("catalog unavailable")
	}
	return c.voices, nil
}

// plainClient only synthesises, so it has no catalog to offer.
type plainClient struct{}

func (p *plainClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	return nil, nil
}

func TestCachedVoiceCatalogFetchesThenServesFromDisk(t *testing.T) {
	client := &catalogClient{voices: []ProviderVoice{{ID: "v1", Name: "Voice One"}}}
	catalog := NewCachedVoiceCatalog(t.TempDir())

	first, err := catalog.Load(context.Background(), "builtin:test", client, false)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(first.Voices) != 1 || first.Stale || first.Provider != "builtin:test" {
		t.Errorf("first snapshot = %+v", first)
	}
	if client.calls != 1 {
		t.Fatalf("expected one fetch, got %d", client.calls)
	}

	second, err := catalog.Load(context.Background(), "builtin:test", client, false)
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if client.calls != 1 {
		t.Errorf("a fresh snapshot should not refetch, calls = %d", client.calls)
	}
	if len(second.Voices) != 1 {
		t.Errorf("second snapshot = %+v", second)
	}

	refreshed, err := catalog.Load(context.Background(), "builtin:test", client, true)
	if err != nil {
		t.Fatalf("refresh Load: %v", err)
	}
	if client.calls != 2 {
		t.Errorf("refresh should refetch, calls = %d", client.calls)
	}
	if refreshed.Stale {
		t.Errorf("a successful refresh is not stale")
	}
}

func TestCachedVoiceCatalogServesStaleOnError(t *testing.T) {
	client := &catalogClient{voices: []ProviderVoice{{ID: "v1"}}}
	catalog := NewCachedVoiceCatalog(t.TempDir())
	if _, err := catalog.Load(context.Background(), "builtin:test", client, false); err != nil {
		t.Fatalf("Load: %v", err)
	}

	client.fail = true
	catalog.ttl = 0 // force a refetch
	stale, err := catalog.Load(context.Background(), "builtin:test", client, true)
	if err != nil {
		t.Fatalf("expected the last snapshot, got %v", err)
	}
	if !stale.Stale || len(stale.Voices) != 1 {
		t.Errorf("stale snapshot = %+v", stale)
	}
}

func TestCachedVoiceCatalogErrorWithoutSnapshot(t *testing.T) {
	client := &catalogClient{fail: true}
	catalog := NewCachedVoiceCatalog(t.TempDir())

	snapshot, err := catalog.Load(context.Background(), "builtin:test", client, false)
	if err == nil {
		t.Fatalf("expected an error when nothing is cached")
	}
	if len(snapshot.Voices) != 0 || snapshot.Stale {
		t.Errorf("snapshot = %+v", snapshot)
	}
}

func TestCachedVoiceCatalogWithoutCatalogSupport(t *testing.T) {
	catalog := NewCachedVoiceCatalog(t.TempDir())
	snapshot, err := catalog.Load(context.Background(), "builtin:test", &plainClient{}, false)
	if err != nil {
		t.Fatalf("a provider without a catalog is not an error: %v", err)
	}
	if len(snapshot.Voices) != 0 {
		t.Errorf("snapshot = %+v", snapshot)
	}
}

func TestCachedVoiceCatalogHonoursTTL(t *testing.T) {
	client := &catalogClient{voices: []ProviderVoice{{ID: "v1"}}}
	catalog := NewCachedVoiceCatalog(t.TempDir())
	catalog.ttl = time.Hour
	now := time.Now()
	catalog.now = func() time.Time { return now }

	if _, err := catalog.Load(context.Background(), "builtin:test", client, false); err != nil {
		t.Fatalf("Load: %v", err)
	}

	now = now.Add(2 * time.Hour)
	if _, err := catalog.Load(context.Background(), "builtin:test", client, false); err != nil {
		t.Fatalf("Load after TTL: %v", err)
	}
	if client.calls != 2 {
		t.Errorf("an expired snapshot should refetch, calls = %d", client.calls)
	}
}
