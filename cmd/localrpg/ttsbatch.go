package main

import (
	"context"
	"flag"
	"fmt"
	"os"
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

// handleTTSBatchCommand backfills a campaign's speech through a provider's batch
// API, which is cheaper than the interactive path and does not spend interactive
// rate-limit quota.
func handleTTSBatchCommand(args []string) {
	fs := flag.NewFlagSet("tts batch", flag.ContinueOnError)
	wait := fs.Bool("wait", false, "wait for the job to finish")
	showStatus := fs.Bool("status", false, "show the campaign's batch jobs")
	resume := fs.String("resume", "", "finish a submitted job by id")
	cancel := fs.String("cancel", "", "cancel a job by id")
	if err := fs.Parse(args); err != nil {
		return
	}
	gameID := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if gameID == "" {
		fmt.Fprintln(os.Stderr, "Usage: localrpg tts batch [--wait] [--status] [--resume <job-id>] [--cancel <job-id>] <game-id>")
		os.Exit(1)
	}

	cfgMgr := config.NewConfigManager()
	cfg, err := cfgMgr.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	dirs := paths.Resolve(paths.System(), cfg.Paths, "")
	resolver := core.NewCustomPathResolver(dirs.Systems, dirs.Worlds, dirs.Games, dirs.Cache)

	store, err := storage.OpenGameStore(resolver, gameID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening campaign %q: %v\n", gameID, err)
		os.Exit(1)
	}

	if *showStatus {
		jobs, err := store.ListTTSJobs(gameID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing jobs: %v\n", err)
			os.Exit(1)
		}
		printTTSJobs(jobs)
		return
	}

	client, err := media.NewTTSClientWithSharedKey(cfg.Media.TTS, cfg.Providers.Gemini.APIKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building TTS client: %v\n", err)
		os.Exit(1)
	}
	batchClient, ok := client.(media.BatchTTSClient)
	if !ok {
		fmt.Fprintln(os.Stderr, "The configured TTS provider has no batch API; choose one that does.")
		os.Exit(1)
	}

	ctx := context.Background()
	providerKey := ""
	if key, ok := media.TTSKeyFor(cfg.Media.TTS); ok {
		providerKey = string(key)
	}
	batchEngine := ttsbatch.New(batchClient, media.NewContentCache(resolver.CacheDir()), store)
	batchEngine.SetOpusBitrate(cfg.OpusBitrate())
	opts := ttsbatch.Options{GameID: gameID, Provider: providerKey, Model: cfg.Media.TTS.Model}

	if *cancel != "" {
		if err := batchClient.CancelBatch(ctx, media.BatchJobHandle{ID: *cancel}); err != nil {
			fmt.Fprintf(os.Stderr, "Error cancelling %s: %v\n", *cancel, err)
			os.Exit(1)
		}
		_ = store.UpdateTTSJobStatus(*cancel, "cancelled", 0, nil)
		fmt.Printf("Cancelled %s\n", *cancel)
		return
	}

	if *resume != "" {
		job, err := batchEngine.Resume(ctx, opts, *resume)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error finishing %s: %v\n", *resume, err)
			os.Exit(1)
		}
		fmt.Printf("Job %s: %d rendered, %d failed.\n", job.ID, job.Completed, len(job.FailedKeys))
		return
	}

	groups, err := planCampaignGroups(resolver, store, cfg, gameID, client)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error planning speech: %v\n", err)
		os.Exit(1)
	}

	if !*wait {
		job, err := batchEngine.Submit(ctx, opts, groups)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error submitting batch: %v\n", err)
			os.Exit(1)
		}
		if job == nil {
			fmt.Println("Every clip is already cached; nothing to backfill.")
			return
		}
		fmt.Printf("Submitted job %s (%d requests). Finish it with --resume %s.\n", job.ID, job.RequestCount, job.ID)
		return
	}

	job, err := batchEngine.Run(ctx, opts, groups)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running batch: %v\n", err)
		os.Exit(1)
	}
	if job == nil {
		fmt.Println("Every clip is already cached; nothing to backfill.")
		return
	}
	fmt.Printf("Job %s: %d rendered, %d failed.\n", job.ID, job.Completed, len(job.FailedKeys))
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
	pipeline.SetGroupCaps(media.ResolveGroupCaps(cfg.Media.TTS, client))

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
func printTTSJobs(jobs []storage.TTSJob) {
	if len(jobs) == 0 {
		fmt.Println("No batch jobs.")
		return
	}
	for _, job := range jobs {
		line := fmt.Sprintf("%s  %s  %s  %d/%d", job.ID, job.Provider, job.Status, job.Completed, job.RequestCount)
		if len(job.FailedKeys) > 0 {
			line += fmt.Sprintf("  failed: %s", strings.Join(job.FailedKeys, ","))
		}
		fmt.Println(line)
	}
}
