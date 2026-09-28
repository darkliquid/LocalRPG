package all_test

import (
	"strings"
	"testing"

	_ "github.com/darkliquid/localrpg/pkg/provider/all"

	"github.com/darkliquid/localrpg/pkg/provider"
)

// These live beside the other registry-wide tests because pkg/provider's own
// test file resets the registry, so a test inside that package cannot see the
// descriptors every provider registers at init.

func TestEveryRegisteredIDIsACanonicalKey(t *testing.T) {
	if err := provider.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	for _, id := range provider.IDs() {
		k, err := provider.ParseKey(id)
		if err != nil {
			t.Errorf("registered ID %q is not a canonical key: %v", id, err)
			continue
		}
		if _, ok := k.Instance(); ok {
			t.Errorf("registered ID %q must be an adapter key, not an instance key", id)
		}
	}
}

func TestDescriptorKeyIsUniquePerFamily(t *testing.T) {
	seen := map[string]string{}
	for _, id := range provider.IDs() {
		k, err := provider.ParseKey(id)
		if err != nil {
			continue
		}
		pair := strings.Join([]string{string(k.Family()), k.Adapter()}, "/")
		if other, dup := seen[pair]; dup {
			t.Errorf("%q and %q share family/adapter %s", other, id, pair)
		}
		seen[pair] = id
	}
}

func TestAllKeysAreRegisteredOrReserved(t *testing.T) {
	// embedding:builtin is built directly by pkg/embeddings rather than through
	// the registry, so it is a reserved key with no descriptor.
	reserved := map[provider.Key]bool{provider.KeyEmbeddingBuiltin: true}

	registered := map[string]bool{}
	for _, id := range provider.IDs() {
		registered[id] = true
	}
	for _, k := range provider.AllKeys() {
		if _, err := provider.ParseKey(string(k)); err != nil {
			t.Errorf("constant %q is malformed: %v", k, err)
		}
		if !registered[string(k)] && !reserved[k] {
			t.Errorf("key %q has no registered descriptor and is not reserved", k)
		}
	}
}
