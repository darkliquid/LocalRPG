package media

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// defaultCatalogTTL is how long a fetched catalog is trusted before another fetch.
const defaultCatalogTTL = 24 * time.Hour

// catalogFetchTimeout bounds one provider catalog call.
const catalogFetchTimeout = 20 * time.Second

// CatalogSnapshot is a provider's voices as of a moment, with Stale set when the
// provider could not be reached and this is the last answer kept on disk.
type CatalogSnapshot struct {
	Provider  string          `json:"provider"`
	FetchedAt time.Time       `json:"fetched_at"`
	Stale     bool            `json:"stale"`
	Voices    []ProviderVoice `json:"voices"`
}

// CachedVoiceCatalog fetches a provider's voices at most once per TTL and keeps
// the last successful answer on disk, so an outage does not empty the picker.
type CachedVoiceCatalog struct {
	cacheDir string
	ttl      time.Duration
	now      func() time.Time
}

// NewCachedVoiceCatalog stores snapshots under <cacheDir>/voices.
func NewCachedVoiceCatalog(cacheDir string) *CachedVoiceCatalog {
	return &CachedVoiceCatalog{cacheDir: cacheDir, ttl: defaultCatalogTTL, now: time.Now}
}

// Load returns a provider's catalog, from disk when it is fresh, from the
// provider otherwise. A provider that cannot enumerate voices yields an empty
// snapshot and no error. A fetch failure returns the last snapshot marked stale;
// with nothing cached it returns the error and an empty snapshot.
func (c *CachedVoiceCatalog) Load(ctx context.Context, providerID string, client TTSClient, refresh bool) (CatalogSnapshot, error) {
	path := c.snapshotPath(providerID)
	last, haveLast := readCatalogSnapshot(path)

	catalog, ok := client.(VoiceCatalog)
	if !ok {
		return CatalogSnapshot{Provider: providerID, Voices: []ProviderVoice{}}, nil
	}

	if !refresh && haveLast && c.now().Sub(last.FetchedAt) < c.ttl {
		last.Stale = false
		return last, nil
	}

	fetchCtx, cancel := context.WithTimeout(ctx, catalogFetchTimeout)
	defer cancel()

	voices, err := catalog.ListVoices(fetchCtx)
	if err != nil {
		if haveLast {
			last.Stale = true
			return last, nil
		}
		return CatalogSnapshot{Provider: providerID, Voices: []ProviderVoice{}}, err
	}

	snapshot := CatalogSnapshot{Provider: providerID, FetchedAt: c.now().UTC(), Voices: voices}
	writeCatalogSnapshot(path, snapshot)
	return snapshot, nil
}

func (c *CachedVoiceCatalog) snapshotPath(providerID string) string {
	safe := strings.NewReplacer(":", "-", "/", "-", "\\", "-").Replace(providerID)
	return filepath.Join(c.cacheDir, "voices", safe+".json")
}

func readCatalogSnapshot(path string) (CatalogSnapshot, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CatalogSnapshot{}, false
	}
	var snapshot CatalogSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return CatalogSnapshot{}, false
	}
	return snapshot, true
}

func writeCatalogSnapshot(path string, snapshot CatalogSnapshot) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0644)
}
