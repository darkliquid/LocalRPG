package gui

// The split of Service into one file per domain is a promise about where code
// lives, and a promise nothing else checks: the compiler is equally happy with 170
// methods in one file. This test is the guard, and it is deliberately a table
// rather than a rule, because which domain a method belongs to is a judgement and
// the judgement should be legible. Moving a method fails this until the table moves
// with it.
//
// The same instinct as the generated-doc guards: an invariant that is written down
// is one that can be kept.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// serviceMethodFiles is the domain each Service method is declared in.
var serviceMethodFiles = map[string][]string{
	"service.go": {
		"ApplyOfflinePreset", "CheckOffline", "ClearTrace", "Close", "Config", "GetResolver",
		"GetSettings", "OpenURL", "SaveSettings", "SetLogger", "SetURLOpener", "SetVersion",
		"TraceEvents", "appVersion", "applyResolvedPaths", "goBackground", "imageRegistry",
		"invalidateRegistries", "setStyleWarnings", "store", "storeOrNil", "styleWarnings",
		"tracePath", "ttsRegistry",
	},
	"audio.go": {
		"AllTTSBatchJobs", "AudioAvailable", "AudioPlaying", "CancelTTSBatch", "ClearTTSBatch",
		"ClipPath", "CountUncachedBeats", "DeleteTTSBatch", "GetCharacterPortrait",
		"GetSegmentClips", "PlaySegmentAudio", "PlayTurnAudio", "RegenerateCharacterPortrait",
		"ResumePendingBatches", "ResumeTTSBatch", "StartTTSBatch", "StopAudio",
		"SubscribeAudioStatus", "TTSBatchJobs", "addPortraitListener", "audioPipeline",
		"audioPlayer", "broadcastAudioFailure", "broadcastAudioStatus", "broadcastPortraitReady",
		"cancelBatchJobs", "clipPath", "clipPlanFor", "emitTurnClips", "groupingEnabled",
		"ledgerHolds", "liveGrouping", "narratorVoiceFor", "recordTTSUsage", "setAudioCurrent",
		"streamerRuns", "synthesizeSegment", "synthesizeTurnGroups", "turnClipStream", "voiceFor",
	},
	"entities.go": {
		"CreateFolder", "DeleteFolder", "DeleteGameEntity", "GetEntity", "GetGraph", "ListEntities",
		"ListEntityMemories", "ListFolders", "MergeEntities", "MoveFolder", "SaveEntity",
		"SaveEntityInFolder", "WorldFolders", "assertIDIsFree", "findEntityNote",
		"hasCustomPortrait", "rewriteInboundLinks", "scanAndEnrichCharacters",
	},
	"games.go": {
		"CreateGame", "DeleteGame", "DeleteGameAsset", "GameFolders", "GenerateGameAsset",
		"GetChronicle", "GetGameAsset", "GetGameState", "GetLocationArt", "GetRecap",
		"GetTurnContext", "GetWorkingSet", "ListGames", "RestartGame", "SaveGameAsset",
		"UpdateGameSettings", "assignPlayerVoice", "campaignArtPrompt", "entityState", "forgetGame",
		"gameAssetSource", "gameAssetURL", "gameCorpus", "gameLock", "protagonistArtSummary",
		"worldArtStyle",
	},
	"models.go": {
		"DownloadModel", "GetModelsStatus", "SubscribeModelEvents", "TestProvider",
		"TranscribeAudio", "UnsubscribeModelEvents", "builtinVoicePackMissing", "testImageProvider",
		"testLLMProvider", "testSTTProvider", "testTTSProvider",
	},
	"registry.go": {
		"AddRegistrySource", "ExportContent", "GenerateAssetPreview", "HandleRegistryInstall",
		"HandleRegistrySearch", "HandleRegistrySources", "HandleRegistryUpdates", "ImportContent",
		"ImportContentWithOptions", "RegistrySources", "RemoveRegistrySource", "contentTargetDir",
		"freeContentID", "registryClient", "resolveContentConflict", "verifyPackage",
	},
	"scene.go": {
		"GenerateTurnSceneImage", "GetTurnSceneImage", "broadcastSceneImagePending",
		"broadcastSceneImageReady", "configureSceneBudget", "saveImageBudget", "sceneArtResolver",
	},
	"systems.go": {
		"DeleteSystem", "DeleteSystemScenario", "GetSystem", "ListReferenceSystems", "ListSystems",
		"SaveSystem", "SaveSystemScenario", "SystemScenarios", "TestSystem", "systemDraftsDir",
	},
	"turns.go": {
		"BeginResolveCheck", "BeginTurn", "GetEntityTurns", "ResolveCheck", "SummaryPending",
		"chronicler", "configureTurnImagery", "configureTurnSpeech", "configureTurnTools",
		"engagementFor", "ensureEmbeddingWorker", "ensureIndexed", "findTurn", "loadTurnManifests",
		"logParityMismatch", "newTurnRuleEngine", "newTurnTimeline", "pendingCheckDTO", "prepareTurn",
		"resolveTurnPlayerID", "summariseBehind", "turnDTO",
	},
	"worlds.go": {
		"CreateWorld", "DeleteWorld", "DeleteWorldEntity", "GenerateWorldAsset", "GetWorld",
		"GetWorldAsset", "GetWorldEntity", "ListWorlds", "SaveWorldAsset", "SaveWorldEntity",
		"UpdateWorld", "writeWorld",
	},
}

// TestServiceMethodsStayInTheirDomain parses the package and checks every method
// on *Service against that table.
func TestServiceMethodsStayInTheirDomain(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	fset := token.NewFileSet()
	found := map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		// Only the files this split created. The package has other Service
		// methods in its feature files (worldgen.go, export.go and the rest), and
		// which file those live in is not this table's business.
		if name != "service.go" {
			if _, ok := serviceMethodFiles[name]; !ok {
				continue
			}
		}
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
				continue
			}
			recv := fn.Recv.List[0].Type
			if star, ok := recv.(*ast.StarExpr); ok {
				recv = star.X
			}
			ident, ok := recv.(*ast.Ident)
			if !ok || ident.Name != "Service" {
				continue
			}
			found[fn.Name.Name] = name
		}
	}

	want := map[string]string{}
	for file, names := range serviceMethodFiles {
		for _, n := range names {
			want[n] = file
		}
	}

	var missing, wrong []string
	for name, file := range want {
		got, ok := found[name]
		switch {
		case !ok:
			missing = append(missing, name)
		case got != file:
			wrong = append(wrong, name+" is in "+got+", want "+file)
		}
	}
	var extra []string
	for name := range found {
		if _, ok := want[name]; !ok {
			extra = append(extra, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(wrong)
	sort.Strings(extra)
	if len(missing) > 0 {
		t.Errorf("methods no longer declared anywhere: %v", missing)
	}
	if len(wrong) > 0 {
		t.Errorf("methods in the wrong file:\n  %s", strings.Join(wrong, "\n  "))
	}
	if len(extra) > 0 {
		t.Errorf("Service methods in these files with no domain in the table: %v", extra)
	}
}
