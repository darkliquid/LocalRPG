package provider_test

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/provider"
)

func TestRegisterLookupAndDuplicatePanics(t *testing.T) {
	provider.Reset()
	t.Cleanup(provider.Reset)

	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{ID: "probe", Family: provider.FamilyLLM, Label: "Probe"},
		Build:      func(context.Context, []byte) (any, error) { return "built", nil },
	})

	reg, ok := provider.Lookup("probe")
	if !ok || reg.Descriptor.Family != provider.FamilyLLM {
		t.Fatalf("lookup failed: %+v ok=%v", reg, ok)
	}
	if got := len(provider.List()); got != 1 {
		t.Fatalf("List = %d, want 1", got)
	}
	if got := len(provider.List(provider.FamilyTTS)); got != 0 {
		t.Fatalf("family filter = %d, want 0", got)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("expected a duplicate registration to panic")
		}
	}()
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{ID: "probe", Family: provider.FamilyLLM},
		Build:      func(context.Context, []byte) (any, error) { return nil, nil },
	})
}

func TestValidateRejectsMissingFamily(t *testing.T) {
	provider.Reset()
	t.Cleanup(provider.Reset)
	provider.Register(provider.Registration{
		Descriptor: provider.Descriptor{ID: "bad"},
	})
	if err := provider.Validate(); err == nil {
		t.Fatal("expected Validate to reject a registration with no family")
	}
}
