package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/paths"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/ttsbatch"
)

// runTTSBatchCommand backfills a campaign's speech through a provider's batch
// API, which is cheaper than the interactive path and does not spend interactive
// rate-limit quota.
func runTTSBatchCommand(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tts batch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	wait := fs.Bool("wait", false, "wait for the job to finish")
	showStatus := fs.Bool("status", false, "show the campaign's batch jobs")
	resume := fs.String("resume", "", "finish a submitted job by id")
	cancel := fs.String("cancel", "", "cancel a job by id")
	force := fs.Bool("force", false, "re-render every clip, overwriting the cache, instead of only missing ones")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	gameID := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if gameID == "" {
		fmt.Fprintln(stderr, "Usage: localrpg tts batch [--wait] [--force] [--status] [--resume <job-id>] [--cancel <job-id>] <game-id>")
		return 1
	}

	cfgMgr := config.NewConfigManager()
	cfg, err := cfgMgr.Load()
	if err != nil {
		fmt.Fprintf(stderr, "Error loading config: %v\n", err)
		return 1
	}

	dirs := paths.Resolve(paths.System(), cfg.Paths, "")
	resolver := core.NewCustomPathResolver(dirs.Systems, dirs.Worlds, dirs.Games, dirs.Cache)

	store, err := storage.OpenGameStore(resolver, gameID)
	if err != nil {
		fmt.Fprintf(stderr, "Error opening campaign %q: %v\n", gameID, err)
		return 1
	}

	if *showStatus {
		jobs, err := store.ListTTSJobs(gameID)
		if err != nil {
			fmt.Fprintf(stderr, "Error listing jobs: %v\n", err)
			return 1
		}
		printTTSJobs(stdout, jobs)
		return 0
	}

	client, err := media.NewTTSClientWithSharedKey(cfg.Media.TTS, cfg.Providers.Gemini.APIKey)
	if err != nil {
		fmt.Fprintf(stderr, "Error building TTS client: %v\n", err)
		return 1
	}
	batchClient, ok := client.(media.BatchTTSClient)
	if !ok {
		fmt.Fprintln(stderr, "The configured TTS provider has no batch API; choose one that does.")
		return 1
	}

	ctx := context.Background()
	providerKey := ""
	if key, ok := media.TTSKeyFor(cfg.Media.TTS); ok {
		providerKey = string(key)
	}
	batchEngine := ttsbatch.New(batchClient, media.NewContentCache(resolver.CacheDir()), store)
	batchEngine.SetOpusBitrate(cfg.OpusBitrate())
	opts := ttsbatch.Options{GameID: gameID, Provider: providerKey, Model: cfg.Media.TTS.Model, Force: *force}

	if *cancel != "" {
		if err := batchClient.CancelBatch(ctx, media.BatchJobHandle{ID: *cancel}); err != nil {
			fmt.Fprintf(stderr, "Error cancelling %s: %v\n", *cancel, err)
			return 1
		}
		_ = store.UpdateTTSJobStatus(*cancel, "cancelled", 0, nil)
		fmt.Fprintf(stdout, "Cancelled %s\n", *cancel)
		return 0
	}

	if *resume != "" {
		job, err := batchEngine.Resume(ctx, opts, *resume)
		if err != nil {
			fmt.Fprintf(stderr, "Error finishing %s: %v\n", *resume, err)
			return 1
		}
		fmt.Fprintf(stdout, "Job %s: %d rendered, %d failed.\n", job.ID, job.Completed, len(job.FailedKeys))
		return 0
	}

	groups, err := planCampaignGroups(resolver, store, cfg, gameID, client)
	if err != nil {
		fmt.Fprintf(stderr, "Error planning speech: %v\n", err)
		return 1
	}

	if !*wait {
		job, err := batchEngine.Submit(ctx, opts, groups)
		if err != nil {
			fmt.Fprintf(stderr, "Error submitting batch: %v\n", err)
			return 1
		}
		if job == nil {
			fmt.Fprintln(stdout, "Every clip is already cached; nothing to backfill.")
			return 0
		}
		fmt.Fprintf(stdout, "Submitted job %s (%d requests). Finish it with --resume %s.\n", job.ID, job.RequestCount, job.ID)
		return 0
	}

	job, err := batchEngine.Run(ctx, opts, groups)
	if err != nil {
		fmt.Fprintf(stderr, "Error running batch: %v\n", err)
		return 1
	}
	if job == nil {
		fmt.Fprintln(stdout, "Every clip is already cached; nothing to backfill.")
		return 0
	}
	fmt.Fprintf(stdout, "Job %s: %d rendered, %d failed.\n", job.ID, job.Completed, len(job.FailedKeys))
	return 0
}

// planCampaignGroups plans every uncached group the campaign's turns need, using
// the same pipeline and keys the app uses so the backfilled clips are found.
func planCampaignGroups(resolver *core.PathResolver, store *storage.Store, cfg *config.Config, gameID string, client media.TTSClient) ([]media.ClipGroup, error) {
	turns, err := engine.NewHistoryLogger(filepath.Join(resolver.GameDir(gameID), "history.jsonl")).LoadHistory()
	if err != nil {
		return nil, err
	}

	pipeline := media.NewTTSPipeline(client, media.NewContentCache(resolver.CacheDir()))
	pipeline.SetTextPolicy(media.TextPolicyFromConfig(cfg.Media.TTS))
	pipeline.SetOpusBitrate(cfg.OpusBitrate())
	pipeline.SetGroupCaps(media.TurnGroupCaps(cfg, media.ResolveGroupCaps(cfg.Media.TTS, client)))

	narrator := cliNarratorVoice(resolver, cfg, gameID)
	voiceFor := func(speakerID string) *entity.VoiceConfig {
		return harness.ResolveSpeakerVoice(store, speakerID)
	}

	groups := make([]media.ClipGroup, 0)
	for _, turn := range turns {
		if len(turn.Segments) == 0 {
			continue
		}
		groups = append(groups, pipeline.GroupClipKeys(turn.Segments, narrator, voiceFor)...)
	}
	return groups, nil
}

// cliNarratorVoice resolves the narrator voice the app would use: the campaign's
// own setting when it has one, otherwise the configuration's default.
func cliNarratorVoice(resolver *core.PathResolver, cfg *config.Config, gameID string) *entity.VoiceConfig {
	voiceID := cfg.Media.TTS.DefaultVoice
	if manifest, err := core.LoadGameManifest(filepath.Join(resolver.GameDir(gameID), "game.yaml")); err == nil && manifest != nil && manifest.Settings != nil {
		if nv, ok := manifest.Settings["narrator_voice"].(string); ok && strings.TrimSpace(nv) != "" {
			voiceID = strings.TrimSpace(nv)
		}
	}
	return &entity.VoiceConfig{
		VoiceID:    voiceID,
		Pitch:      cfg.Media.TTS.Pitch,
		SpeechRate: cfg.Media.TTS.SpeechRate,
		Options:    cfg.Media.TTS.Options,
	}
}

// printTTSJobs lists a campaign's batch jobs.
func printTTSJobs(stdout io.Writer, jobs []storage.TTSJob) {
	if len(jobs) == 0 {
		fmt.Fprintln(stdout, "No batch jobs.")
		return
	}
	for _, job := range jobs {
		line := fmt.Sprintf("%s  %s  %s  %d/%d", job.ID, job.Provider, job.Status, job.Completed, job.RequestCount)
		if len(job.FailedKeys) > 0 {
			line += fmt.Sprintf("  failed: %s", strings.Join(job.FailedKeys, ","))
		}
		fmt.Fprintln(stdout, line)
	}
}
