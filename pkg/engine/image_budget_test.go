package engine

import (
	"context"
	"testing"
	"time"

	"github.com/darkliquid/localrpg/pkg/core"
)

func TestBudgetExhausted(t *testing.T) {
	if !(ImageBudget{MaxImages: 2, Used: 2}).Exhausted() {
		t.Fatal("at the image limit should be exhausted")
	}
	if (ImageBudget{MaxImages: 2, Used: 1}).Exhausted() {
		t.Fatal("under the image limit is not exhausted")
	}
	if (ImageBudget{MaxMicros: 100, Spent: 99}).Exhausted() {
		t.Fatal("under the spend limit is not exhausted")
	}
	if !(ImageBudget{MaxMicros: 100, Spent: 100}).Exhausted() {
		t.Fatal("at the spend limit should be exhausted")
	}
	if (ImageBudget{}).Exhausted() {
		t.Fatal("unlimited is never exhausted")
	}
	if !(ImageBudget{}).Unlimited() {
		t.Fatal("an empty budget is unlimited")
	}
}

func TestBudgetCharge(t *testing.T) {
	budget := ImageBudget{MaxImages: 3}
	budget.Charge(40)
	if budget.Used != 1 || budget.Spent != 40 {
		t.Fatalf("budget = %+v", budget)
	}
	if budget.Remaining() != 2 {
		t.Fatalf("remaining = %d", budget.Remaining())
	}
	// An unpriced generation still counts as an image.
	budget.Charge(0)
	if budget.Used != 2 || budget.Spent != 40 {
		t.Fatalf("an unpriced charge should count the image only: %+v", budget)
	}
	if (ImageBudget{}).Remaining() != -1 {
		t.Fatal("an unlimited budget has no remaining count")
	}
}

func TestBudgetRoundTripsInTheManifest(t *testing.T) {
	manifest := &core.GameManifest{ID: "campaign"}
	SetImageBudget(manifest, ImageBudget{MaxImages: 200, MaxMicros: 5_000_000, Used: 3, Spent: 120_000})
	SetImageApprovalMode(manifest, ImageApprovalAsk)

	got := ImageBudgetFromManifest(manifest)
	if got.MaxImages != 200 || got.MaxMicros != 5_000_000 || got.Used != 3 || got.Spent != 120_000 {
		t.Fatalf("budget = %+v", got)
	}
	if ImageApprovalMode(manifest) != ImageApprovalAsk {
		t.Fatalf("approval = %q", ImageApprovalMode(manifest))
	}
}

func TestBudgetDefaultsAreUnlimited(t *testing.T) {
	if !ImageBudgetFromManifest(nil).Unlimited() {
		t.Fatal("a nil manifest should be unlimited")
	}
	if !ImageBudgetFromManifest(&core.GameManifest{}).Unlimited() {
		t.Fatal("a manifest with no budget should be unlimited")
	}
	if ImageApprovalMode(nil) != ImageApprovalAuto {
		t.Fatal("a nil manifest should default to auto approval")
	}
	broken := &core.GameManifest{Settings: map[string]interface{}{ImageBudgetSetting: "not a budget"}}
	if !ImageBudgetFromManifest(broken).Unlimited() {
		t.Fatal("a malformed budget should degrade to unlimited")
	}
}

// budgetGen counts what it was asked to draw.
type budgetGen struct {
	id    string
	calls int
}

func (g *budgetGen) GenerateImage(context.Context, string) ([]byte, error) {
	g.calls++
	return []byte(g.id), nil
}

// TestBudgetBlocksGenerationAndFallsBack guards the enforcement: a spent budget
// does not call the provider, and the procedural fallback draws instead.
func TestBudgetBlocksGenerationAndFallsBack(t *testing.T) {
	dir := t.TempDir()
	provider := &budgetGen{id: "provider"}
	fallback := &budgetGen{id: "fallback"}
	worker := NewSceneWorker(core.NewPathResolver(dir), provider)
	worker.SetProceduralFallback(fallback)
	worker.SetBudget(ImageBudget{MaxImages: 1, Used: 1}, nil)

	written := waitForScene(t, worker, "campaign", 5, SceneJob{Prompt: "a hall"})

	if provider.calls != 0 {
		t.Fatalf("the provider was called %d times with the budget spent", provider.calls)
	}
	if fallback.calls != 1 {
		t.Fatalf("the fallback was called %d times", fallback.calls)
	}
	if written == "" {
		t.Fatal("a fallback image should still be written")
	}
	if worker.Budget().Used != 1 {
		t.Fatalf("the fallback is free, so the budget should not grow: %+v", worker.Budget())
	}
}

func TestBudgetChargesOnSuccess(t *testing.T) {
	dir := t.TempDir()
	provider := &budgetGen{id: "provider"}
	worker := NewSceneWorker(core.NewPathResolver(dir), provider)
	worker.SetPrice(func() (int64, bool) { return 250, true })

	var saved []ImageBudget
	worker.SetBudget(ImageBudget{MaxImages: 2}, func(b ImageBudget) { saved = append(saved, b) })

	waitForScene(t, worker, "campaign", 6, SceneJob{Prompt: "a hall"})

	if provider.calls != 1 {
		t.Fatalf("provider calls = %d", provider.calls)
	}
	got := worker.Budget()
	if got.Used != 1 || got.Spent != 250 {
		t.Fatalf("budget = %+v, want one image charged at 250", got)
	}
	if len(saved) != 1 || saved[0].Used != 1 {
		t.Fatalf("the charge was not persisted: %+v", saved)
	}
}

// TestUnlimitedBudgetIsUnchanged guards that a campaign with no budget generates
// exactly as IMG-2 did.
func TestUnlimitedBudgetIsUnchanged(t *testing.T) {
	dir := t.TempDir()
	provider := &budgetGen{id: "provider"}
	fallback := &budgetGen{id: "fallback"}
	worker := NewSceneWorker(core.NewPathResolver(dir), provider)
	worker.SetProceduralFallback(fallback)

	var saved int
	worker.SetBudget(ImageBudget{}, func(ImageBudget) { saved++ })

	waitForScene(t, worker, "campaign", 7, SceneJob{Prompt: "a hall"})

	if provider.calls != 1 || fallback.calls != 0 {
		t.Fatalf("provider = %d fallback = %d, want the provider alone", provider.calls, fallback.calls)
	}
	if saved != 1 {
		t.Fatalf("an unlimited budget still counts images, saved = %d", saved)
	}
	if worker.Budget().Used != 1 {
		t.Fatalf("budget = %+v", worker.Budget())
	}
}

func TestApprovalAskSkipsAMeteredProvider(t *testing.T) {
	dir := t.TempDir()
	provider := &budgetGen{id: "provider"}
	worker := NewSceneWorker(core.NewPathResolver(dir), provider)
	worker.SetApproval(ImageApprovalAsk, true)

	var pending int
	worker.SetOnPending(func(string, int, SceneJob) { pending++ })

	// The job is not generated, so wait for the pending callback instead.
	worker.EnqueueScene("campaign", 8, SceneJob{Prompt: "a hall"})
	waitFor(t, func() bool { return pending > 0 })

	if provider.calls != 0 {
		t.Fatalf("an unapproved generation called the provider %d times", provider.calls)
	}
}

func TestApprovalAskDoesNotWaitOnALocalProvider(t *testing.T) {
	dir := t.TempDir()
	provider := &budgetGen{id: "provider"}
	worker := NewSceneWorker(core.NewPathResolver(dir), provider)
	worker.SetApproval(ImageApprovalAsk, false)

	waitForScene(t, worker, "campaign", 9, SceneJob{Prompt: "a hall"})

	if provider.calls != 1 {
		t.Fatalf("a local provider needs no approval, calls = %d", provider.calls)
	}
}

// waitFor waits for a condition, so a test can assert an asynchronous effect
// without a fixed sleep.
func waitFor(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if done() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the condition was never met")
}
