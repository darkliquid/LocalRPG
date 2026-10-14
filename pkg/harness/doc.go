// Package harness manages LLM prompt assembly, context budgeting, model routing,
// and turn extraction.
//
// The harness packages working-set memories, entity summaries, recent turns, and
// tool definitions into structured prompt contexts within bounded token limits
// (AssembleContext, ContextPlan in context.go). It abstracts model invocation across
// providers (router.go, factory.go), handling fallbacks, retry policies, and token
// usage telemetry. After generation, the Extractor (extractor.go) parses structured
// entity updates, state mutations, and summary notes from model output.
//
// What harness is NOT:
//   - It is NOT the campaign orchestrator; turn flow, session state, and dice check
//     sequencing are controlled by pkg/engine.
//   - It does NOT implement concrete provider protocols (OpenAI, Gemini, Ollama, etc.);
//     underlying API adapters live in pkg/provider/*.
//   - It does NOT execute game system rules or JavaScript mechanics hooks; rules logic
//     belongs to pkg/rules.
package harness
