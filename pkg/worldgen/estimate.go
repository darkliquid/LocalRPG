package worldgen

// Estimate is the planned cost of a generation, before it runs.
type Estimate struct {
	// Calls is how many model calls the pipeline will make.
	Calls int `json:"calls"`
	// Chunks is how many source chunks an ingestion will process.
	Chunks int `json:"chunks,omitempty"`
	// CostMicros is the estimated cost in millionths of a currency unit. It is
	// zero when the provider is unpriced.
	CostMicros int64 `json:"cost_micros,omitempty"`
	// Priced reports whether CostMicros is meaningful.
	Priced bool `json:"priced"`
}

// PriceTable prices a planned number of calls, or reports that the provider is
// unpriced. A nil table means unpriced, which is the honest default: the cost of
// a provider the ledger has no rate for is not zero, it is unknown.
type PriceTable func(calls int) (int64, bool)

// worldStepCalls is how many calls the whole-world pipeline makes before any
// count exceeds one batch: outline, places, characters, and link.
const worldStepCalls = 4

// ChunksPerCall is how many source chunks one ingestion call reads. It lives
// here because the estimator has to know it, and the ingestion reads it from
// here so the two cannot drift.
const ChunksPerCall = 4

// EstimatePlan reports the calls a generation of the given kind will make.
// kind is one of "world", "entities", "enhance", or "ingest".
//
// brief.Counts steers a from-scratch world and a batch of entities. An ingestion
// ignores them: the source decides how many entities exist, so only the chunk
// count moves its estimate.
func EstimatePlan(kind string, brief Brief, chunks int, prices PriceTable) Estimate {
	counts := brief.Counts
	if counts.Zero() {
		counts = DefaultCounts()
	}

	e := Estimate{Chunks: chunks}
	switch kind {
	case "entities":
		e.Calls = batchCalls(counts.Total())
	case "enhance":
		e.Calls = 1
	case "ingest":
		e.Calls = ChunkCalls(chunks)
	default:
		e.Calls = worldStepCalls + extraBatches(counts)
	}
	if e.Calls < 1 {
		e.Calls = 1
	}
	if prices != nil {
		if cost, ok := prices(e.Calls); ok {
			e.CostMicros = cost
			e.Priced = true
		}
	}
	return e
}

// Total is how many entities a count requests.
func (c Counts) Total() int {
	return c.Locations + c.Factions + c.Characters
}

// batchCalls is how many calls a batch of n entities takes at MaxBatch each.
func batchCalls(n int) int {
	if n <= 0 {
		return 1
	}
	return (n + MaxBatch - 1) / MaxBatch
}

// ChunkCalls is how many calls reading n chunks takes, one per batch.
func ChunkCalls(n int) int {
	if n <= 0 {
		return 1
	}
	return (n + ChunksPerCall - 1) / ChunksPerCall
}

// extraBatches counts the additional calls a count over one batch needs.
func extraBatches(c Counts) int {
	extra := 0
	for _, n := range []int{c.Locations, c.Factions, c.Characters} {
		if n > MaxBatch {
			extra += batchCalls(n) - 1
		}
	}
	return extra
}
