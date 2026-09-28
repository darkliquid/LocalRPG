// Package tools implements the read-only tools the GM may call mid-turn. Every
// tool reads the campaign's own index and store; none of them can write.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/embeddings"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/storage"
)

// defaultLimit caps a modest tool result when the model does not ask for a limit.
const defaultLimit = 10

// EntityWriter saves an entity note to persistent storage.
type EntityWriter interface {
	SaveEntity(ent *entity.Entity) error
}

// Executor runs tool calls against one campaign's index.
type Executor struct {
	store              *storage.Store
	writer             EntityWriter
	maxChars           int
	embeddingsProvider embeddings.Provider
	voiceProfiles      []config.VoiceProfile
	assignedVoices     map[string]config.VoiceProfile

	embeddingUsage EmbeddingUsageFunc
	embeddingKey   string
	embeddingModel string
}

// EmbeddingUsageFunc reports one embedding-backed search's usage to a sink, so
// semantic recall appears in the spend ledger like every other provider call.
type EmbeddingUsageFunc func(providerKey, model string, inputTokens, characters, requests int)

// SetEmbeddingUsage installs the sink embedding-backed searches report to.
func (e *Executor) SetEmbeddingUsage(fn EmbeddingUsageFunc, providerKey, model string) {
	e.embeddingUsage = fn
	e.embeddingKey = providerKey
	e.embeddingModel = model
}

// reportEmbeddingUsage files one embedding call, if a sink is installed. It is
// best effort: a missing sink or a provider that reports nothing never fails a
// tool call.
func (e *Executor) reportEmbeddingUsage() {
	if e.embeddingUsage == nil {
		return
	}
	inputTokens, characters, requests := 0, 0, 1
	if reporter, ok := e.embeddingsProvider.(interface{ LastUsage() harness.Usage }); ok {
		usage := reporter.LastUsage()
		inputTokens = usage.InputTokens
		characters = usage.Characters
		if usage.Requests > 0 {
			requests = usage.Requests
		}
	}
	e.embeddingUsage(e.embeddingKey, e.embeddingModel, inputTokens, characters, requests)
}

// NewExecutor builds an executor. maxChars is agents.tool_result_chars; a
// non-positive value falls back to 4000.
func NewExecutor(store *storage.Store, maxChars int) *Executor {
	if maxChars <= 0 {
		maxChars = 4000
	}
	return &Executor{
		store:          store,
		maxChars:       maxChars,
		assignedVoices: make(map[string]config.VoiceProfile),
	}
}

// SetEmbeddingsProvider configures the vector embedding provider for hybrid search.
func (e *Executor) SetEmbeddingsProvider(p embeddings.Provider) {
	e.embeddingsProvider = p
}

// SetVoiceProfiles sets the available NPC voice profiles.
func (e *Executor) SetVoiceProfiles(profiles []config.VoiceProfile) {
	e.voiceProfiles = profiles
}

// SetEntityWriter sets a custom entity writer such as Timeline.
func (e *Executor) SetEntityWriter(w EntityWriter) {
	e.writer = w
}

// AssignedVoices returns a copy of voice profiles assigned or staged during this executor's lifetime.
func (e *Executor) AssignedVoices() map[string]config.VoiceProfile {
	res := make(map[string]config.VoiceProfile, len(e.assignedVoices))
	for k, v := range e.assignedVoices {
		res[k] = v
	}
	return res
}

// Execute runs one call and returns the text the model will read. ok is false
// when the call failed, but the result is still a readable message: a tool error
// must never become a failed turn.
func (e *Executor) Execute(ctx context.Context, call harness.ToolCall) (string, bool) {
	arguments := map[string]interface{}{}
	if strings.TrimSpace(call.Arguments) != "" {
		if err := json.Unmarshal([]byte(call.Arguments), &arguments); err != nil {
			return e.cap(fmt.Sprintf("error: could not parse the arguments for %s: %v", call.Name, err)), false
		}
	}

	switch call.Name {
	case "search_entities":
		return e.searchEntities(ctx, arguments)
	case "get_entity":
		return e.getEntity(arguments)
	case "graph_neighbours":
		return e.graphNeighbours(arguments)
	case "search_timeline":
		return e.searchTimeline(ctx, arguments)
	case "search_memories":
		return e.searchMemories(ctx, arguments)
	case "get_entity_timeline":
		return e.getEntityTimeline(arguments)
	case "search_voice_profiles":
		return e.searchVoiceProfiles(arguments)
	case "assign_voice":
		return e.assignVoice(arguments)
	default:
		return e.cap(harness.UnknownToolMessage(call.Name)), false
	}
}

func (e *Executor) searchEntities(ctx context.Context, arguments map[string]interface{}) (string, bool) {
	rawQuery := stringArgument(arguments, "query")
	match := stringArgument(arguments, "match")
	if match == "" {
		match = BuildMatch(rawQuery)
	}
	if match == "" && rawQuery == "" {
		return "error: search_entities needs a query", false
	}
	limit := intArgument(arguments, "limit", defaultLimit)
	entityType := stringArgument(arguments, "type")

	var ftsHits []storage.SearchEntityHit
	if match != "" {
		hits, err := e.store.SearchEntities(match, entityType, limit*2)
		if err == nil {
			ftsHits = hits
		}
	}

	var vecIDs []string
	if e.embeddingsProvider != nil && rawQuery != "" {
		vecs, err := e.embeddingsProvider.Embed(ctx, []string{rawQuery})
		if err == nil {
			e.reportEmbeddingUsage()
		}
		if err == nil && len(vecs) > 0 {
			vHits, err := e.store.SearchSimilarVectors(ctx, []string{"entity"}, e.embeddingsProvider.ID(), vecs[0], limit*2)
			if err == nil {
				for _, vh := range vHits {
					if entityType != "" {
						ent, err := e.store.GetEntity(vh.TargetID)
						if err != nil || ent == nil || ent.Type != entityType {
							continue
						}
					}
					vecIDs = append(vecIDs, vh.TargetID)
				}
			}
		}
	}

	if len(ftsHits) == 0 && len(vecIDs) == 0 {
		return "No entities matched.", true
	}

	ftsIDs := make([]string, len(ftsHits))
	ftsMap := make(map[string]storage.SearchEntityHit, len(ftsHits))
	for i, hit := range ftsHits {
		ftsIDs[i] = hit.ID
		ftsMap[hit.ID] = hit
	}

	fusedIDs := FuseRankings(ftsIDs, vecIDs, limit)
	if len(fusedIDs) == 0 {
		return "No entities matched.", true
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%d matching entities:\n", len(fusedIDs))
	for _, id := range fusedIDs {
		if hit, ok := ftsMap[id]; ok {
			fmt.Fprintf(&sb, "- %s (%s, id %s): %s\n", hit.Name, hit.Type, hit.ID, hit.Snippet)
		} else if ent, err := e.store.GetEntity(id); err == nil && ent != nil {
			snippet := ent.Body
			if len([]rune(snippet)) > 120 {
				snippet = string([]rune(snippet)[:120]) + "..."
			}
			fmt.Fprintf(&sb, "- %s (%s, id %s): %s\n", ent.Name, ent.Type, ent.ID, snippet)
		}
	}
	return e.cap(sb.String()), true
}

func (e *Executor) getEntity(arguments map[string]interface{}) (string, bool) {
	ref := stringArgument(arguments, "id_or_name")
	if ref == "" {
		return "error: get_entity needs id_or_name", false
	}

	ent, err := e.findEntity(ref)
	if err != nil {
		return e.cap(fmt.Sprintf("error: %v", err)), false
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s (%s, id %s)\n", ent.Name, ent.Type, ent.ID)
	if ent.Location != "" {
		fmt.Fprintf(&sb, "location: %s\n", ent.Location)
	}
	if ent.Faction != "" {
		fmt.Fprintf(&sb, "faction: %s\n", ent.Faction)
	}
	if len(ent.Aliases) > 0 {
		fmt.Fprintf(&sb, "aliases: %s\n", strings.Join(ent.Aliases, ", "))
	}
	if ent.State != nil {
		if raw, err := json.Marshal(ent.State.Raw()); err == nil {
			fmt.Fprintf(&sb, "state: %s\n", raw)
		}
	}
	sb.WriteString("\n")
	sb.WriteString(ent.Body)
	return e.cap(sb.String()), true
}

// findEntity resolves an exact id first, then a case-insensitive name.
func (e *Executor) findEntity(ref string) (*entity.Entity, error) {
	if ent, err := e.store.GetEntity(ref); err == nil && ent != nil {
		return ent, nil
	}
	summaries, err := e.store.ListEntities()
	if err != nil {
		return nil, err
	}
	for _, summary := range summaries {
		if strings.EqualFold(summary.Name, ref) || strings.EqualFold(summary.ID, ref) {
			return e.store.GetEntity(summary.ID)
		}
	}
	return nil, fmt.Errorf("no entity matching %q", ref)
}

func (e *Executor) graphNeighbours(arguments map[string]interface{}) (string, bool) {
	id := stringArgument(arguments, "id")
	if id == "" {
		return "error: graph_neighbours needs an id", false
	}
	direction := strings.ToLower(stringArgument(arguments, "direction"))
	if direction == "" {
		direction = "both"
	}
	limit := intArgument(arguments, "limit", 20)

	lines := make([]string, 0)
	if direction == "from" || direction == "both" {
		edges, err := e.store.GetEdgesFrom(id)
		if err != nil {
			return e.cap(fmt.Sprintf("error: graph_neighbours failed: %v", err)), false
		}
		for _, edge := range edges {
			lines = append(lines, fmt.Sprintf("- %s -> %s (%s)", id, edge.TargetID, edge.Relation))
		}
	}
	if direction == "to" || direction == "both" {
		edges, err := e.store.GetEdgesTo(id)
		if err != nil {
			return e.cap(fmt.Sprintf("error: graph_neighbours failed: %v", err)), false
		}
		for _, edge := range edges {
			lines = append(lines, fmt.Sprintf("- %s <- %s (%s)", id, edge.SourceID, edge.Relation))
		}
	}

	if len(lines) == 0 {
		return fmt.Sprintf("No neighbours for %q.", id), true
	}
	sort.Strings(lines)
	if len(lines) > limit {
		lines = lines[:limit]
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d neighbours of %s:\n", len(lines), id)
	sb.WriteString(strings.Join(lines, "\n"))
	return e.cap(sb.String()), true
}

func (e *Executor) searchTimeline(ctx context.Context, arguments map[string]interface{}) (string, bool) {
	rawQuery := stringArgument(arguments, "query")
	match := stringArgument(arguments, "match")
	if match == "" {
		match = BuildMatch(rawQuery)
	}
	if match == "" && rawQuery == "" {
		return "error: search_timeline needs a query", false
	}
	limit := intArgument(arguments, "limit", defaultLimit)
	entityFilter := stringArgument(arguments, "entity")

	var ftsHits []storage.SearchTurnHit
	if match != "" {
		hits, err := e.store.SearchTurns(match, entityFilter, limit*2)
		if err == nil {
			ftsHits = hits
		}
	}

	var vecTurnNumbers []string
	if e.embeddingsProvider != nil && rawQuery != "" {
		vecs, err := e.embeddingsProvider.Embed(ctx, []string{rawQuery})
		if err == nil {
			e.reportEmbeddingUsage()
		}
		if err == nil && len(vecs) > 0 {
			vHits, err := e.store.SearchSimilarVectors(ctx, []string{"turn"}, e.embeddingsProvider.ID(), vecs[0], limit*2)
			if err == nil {
				for _, vh := range vHits {
					if entityFilter != "" {
						num, _ := strconv.Atoi(vh.TargetID)
						turnRec, err := e.store.GetTurn(num)
						if err != nil || turnRec == nil {
							continue
						}
						// Check if entity is mentioned in turn
						mentioned := false
						for _, m := range turnRec.Entities {
							if m.EntityID == entityFilter {
								mentioned = true
								break
							}
						}
						if !mentioned {
							continue
						}
					}
					vecTurnNumbers = append(vecTurnNumbers, vh.TargetID)
				}
			}
		}
	}

	if len(ftsHits) == 0 && len(vecTurnNumbers) == 0 {
		return "No turns matched.", true
	}

	ftsTurnNumbers := make([]string, len(ftsHits))
	ftsMap := make(map[string]storage.SearchTurnHit, len(ftsHits))
	for i, hit := range ftsHits {
		strNum := strconv.Itoa(hit.Number)
		ftsTurnNumbers[i] = strNum
		ftsMap[strNum] = hit
	}

	fusedNumbers := FuseRankings(ftsTurnNumbers, vecTurnNumbers, limit)
	if len(fusedNumbers) == 0 {
		return "No turns matched.", true
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%d matching turns:\n", len(fusedNumbers))
	for _, strNum := range fusedNumbers {
		num, _ := strconv.Atoi(strNum)
		if hit, ok := ftsMap[strNum]; ok {
			fmt.Fprintf(&sb, "- turn %d: %s\n", hit.Number, hit.Snippet)
		} else if turnRec, err := e.store.GetTurn(num); err == nil && turnRec != nil {
			snippet := turnRec.Narration
			if len([]rune(snippet)) > 120 {
				snippet = string([]rune(snippet)[:120]) + "..."
			}
			fmt.Fprintf(&sb, "- turn %d: %s\n", turnRec.Number, snippet)
		}
	}
	return e.cap(sb.String()), true
}

// cap truncates a result and says so, because a model that cannot tell a capped
// result from a small world will conclude the world is small.
func (e *Executor) cap(text string) string {
	runes := []rune(text)
	if len(runes) <= e.maxChars {
		return text
	}
	return string(runes[:e.maxChars]) + fmt.Sprintf("\n... (truncated at %d characters; narrow the query to see more)", e.maxChars)
}

func stringArgument(arguments map[string]interface{}, key string) string {
	value, _ := arguments[key].(string)
	return strings.TrimSpace(value)
}

func intArgument(arguments map[string]interface{}, key string, fallback int) int {
	switch value := arguments[key].(type) {
	case float64:
		if value > 0 {
			return int(value)
		}
	case int:
		if value > 0 {
			return value
		}
	}
	return fallback
}

func (e *Executor) searchVoiceProfiles(arguments map[string]interface{}) (string, bool) {
	query := strings.TrimSpace(stringArgument(arguments, "query"))
	if query == "" {
		return "error: search_voice_profiles needs a query describing desired traits", false
	}
	if len(e.voiceProfiles) == 0 {
		return "no voice profiles configured", true
	}
	limit := intArgument(arguments, "limit", 5)
	if limit <= 0 {
		limit = 5
	}

	terms := strings.Fields(strings.ToLower(query))

	type scoredProfile struct {
		profile config.VoiceProfile
		score   int
	}
	var scored []scoredProfile

	for _, p := range e.voiceProfiles {
		s := 0
		searchText := strings.ToLower(strings.Join([]string{
			p.ID,
			p.Name,
			p.Description,
			strings.Join(p.Tags, " "),
		}, " "))

		for _, term := range terms {
			if strings.Contains(searchText, term) {
				s += 2
			}
		}
		for _, tag := range p.Tags {
			for _, term := range terms {
				if strings.EqualFold(tag, term) {
					s += 3
				}
			}
		}
		if s > 0 {
			scored = append(scored, scoredProfile{profile: p, score: s})
		}
	}

	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	if len(scored) == 0 {
		var sb strings.Builder
		sb.WriteString("No voice profile directly matched traits; available profiles:\n")
		n := limit
		if n > len(e.voiceProfiles) {
			n = len(e.voiceProfiles)
		}
		for i := 0; i < n; i++ {
			p := e.voiceProfiles[i]
			sb.WriteString(fmt.Sprintf("- `%s`: %s [tags: %s]\n", p.ID, p.Description, strings.Join(p.Tags, ", ")))
		}
		return e.cap(sb.String()), true
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d matching voice profiles:\n", len(scored)))
	for i := 0; i < len(scored) && i < limit; i++ {
		p := scored[i].profile
		sb.WriteString(fmt.Sprintf("- `%s`: %s [tags: %s]\n", p.ID, p.Description, strings.Join(p.Tags, ", ")))
	}
	return e.cap(sb.String()), true
}

func (e *Executor) assignVoice(arguments map[string]interface{}) (string, bool) {
	idOrName := strings.TrimSpace(stringArgument(arguments, "entity"))
	profileID := strings.TrimSpace(stringArgument(arguments, "profile_id"))
	if idOrName == "" || profileID == "" {
		return "error: assign_voice requires 'entity' and 'profile_id'", false
	}

	var targetProfile *config.VoiceProfile
	for i := range e.voiceProfiles {
		if strings.EqualFold(e.voiceProfiles[i].ID, profileID) {
			targetProfile = &e.voiceProfiles[i]
			break
		}
	}
	if targetProfile == nil {
		available := make([]string, 0, len(e.voiceProfiles))
		for _, p := range e.voiceProfiles {
			available = append(available, p.ID)
		}
		return fmt.Sprintf("error: unknown voice profile %q. Available: %s", profileID, strings.Join(available, ", ")), false
	}

	if e.assignedVoices == nil {
		e.assignedVoices = make(map[string]config.VoiceProfile)
	}
	slugID := entity.Slugify(idOrName)
	e.assignedVoices[slugID] = *targetProfile

	var ent *entity.Entity
	if e.store != nil {
		if found, err := e.findEntity(idOrName); err == nil && found != nil {
			ent = found
		} else if found, err := e.findEntity(slugID); err == nil && found != nil {
			ent = found
		}
	}

	voiceCfg := &entity.VoiceConfig{
		Provider:   targetProfile.Provider,
		VoiceID:    targetProfile.VoiceID,
		Pitch:      targetProfile.Pitch,
		SpeechRate: targetProfile.SpeechRate,
		Options:    targetProfile.Options,
	}

	if ent != nil {
		ent.Voice = voiceCfg
		var saveErr error
		if e.writer != nil {
			saveErr = e.writer.SaveEntity(ent)
		} else if e.store != nil {
			saveErr = e.store.SaveEntity(ent)
		}
		if saveErr != nil {
			return fmt.Sprintf("failed to save voice to entity %s: %v", ent.Name, saveErr), false
		}
		return fmt.Sprintf("Voice profile %q (%s) assigned to %s.", targetProfile.ID, targetProfile.Description, ent.Name), true
	}

	return fmt.Sprintf("Voice profile %q (%s) staged for %q. It will be assigned when the character note is created.", targetProfile.ID, targetProfile.Description, idOrName), true
}

