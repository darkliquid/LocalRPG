package export

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
)

type ScriptCompiler struct {
	rootDir  string
	resolver *core.PathResolver
}

func NewScriptCompiler(rootDir string) *ScriptCompiler {
	return &ScriptCompiler{
		rootDir:  rootDir,
		resolver: core.NewPathResolver(rootDir),
	}
}

func (s *ScriptCompiler) Compile(ctx context.Context, gameID string) (*ReplayScript, error) {
	gameDir := s.resolver.GameDir(gameID)
	manifestPath := filepath.Join(gameDir, "game.yaml")
	manifest, err := core.LoadGameManifest(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("load manifest: %w", err)
	}

	historyFile := filepath.Join(gameDir, "history.jsonl")
	logger := engine.NewHistoryLogger(historyFile)
	turns, err := logger.LoadHistory()
	if err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}

	beats := make([]SceneBeat, len(turns))
	totalDuration := 0.0

	for i, turn := range turns {
		segments := turn.Segments
		if len(segments) == 0 {
			segments = media.LegacySegments(turn.Prose())
		}

		audioPath := ""
		if len(turn.AudioRefs) > 0 {
			audioPath = turn.AudioRefs[0]
		}

		// Estimate 3.5 seconds per narrated span and 4 per spoken line when audio is missing.
		duration := 0.0
		for _, segment := range segments {
			if segment.Kind == entity.SegmentSpeech {
				duration += 4.0
				continue
			}
			duration += 3.5
		}
		totalDuration += duration

		beats[i] = SceneBeat{
			TurnNumber:  turn.Number,
			Timestamp:   turn.Timestamp,
			Mode:        turn.Mode,
			PlayerInput: turn.Input,
			Prose:       turn.Prose(),
			Segments:    segments,
			AudioPath:   audioPath,
			DurationSec: duration,
		}
	}

	return &ReplayScript{
		GameID:        gameID,
		GameName:      manifest.Name,
		TotalDuration: totalDuration,
		Beats:         beats,
	}, nil
}
