package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/content"
	"github.com/darkliquid/localrpg/pkg/core"
)

// LockContent computes and writes content.lock.yaml for the given campaign.
func LockContent(paths *core.PathResolver, gameID string) (content.ContentLock, error) {
	gameManifest, err := core.LoadGameManifest(filepath.Join(paths.GameDir(gameID), "game.yaml"))
	if err != nil {
		return content.ContentLock{}, fmt.Errorf("load game manifest: %w", err)
	}

	sysDir := paths.SystemDir(gameManifest.SystemID)
	sysManifest, err := core.LoadSystemManifest(filepath.Join(sysDir, "system.yaml"))
	if err != nil {
		return content.ContentLock{}, fmt.Errorf("load system manifest: %w", err)
	}
	sysDigest, err := content.BehaviouralDigest(sysDir)
	if err != nil {
		return content.ContentLock{}, fmt.Errorf("system behavioural digest: %w", err)
	}

	worldDir := paths.WorldDir(gameManifest.WorldID)
	worldManifest, err := core.LoadWorldManifest(filepath.Join(worldDir, "world.yaml"))
	if err != nil {
		return content.ContentLock{}, fmt.Errorf("load world manifest: %w", err)
	}
	worldDigest, err := content.BehaviouralDigest(worldDir)
	if err != nil {
		return content.ContentLock{}, fmt.Errorf("world behavioural digest: %w", err)
	}

	lock := content.ContentLock{
		App: "localrpg",
		Entries: []content.LockEntry{
			{
				Type:    "system",
				ID:      gameManifest.SystemID,
				Version: sysManifest.Version,
				SHA256:  sysDigest,
			},
			{
				Type:    "world",
				ID:      gameManifest.WorldID,
				Version: worldManifest.Version,
				SHA256:  worldDigest,
			},
		},
	}

	lockPath := filepath.Join(paths.GameDir(gameID), "content.lock.yaml")
	if err := lock.Save(lockPath); err != nil {
		return content.ContentLock{}, fmt.Errorf("save lock: %w", err)
	}

	return lock, nil
}

// ResolveContentLock checks the campaign's dependencies and content lock.
// It verifies that:
// 1. World dependencies (Requires) on the system are satisfied (refusing if not).
// 2. The lockfile exists (creating one if missing).
// 3. Digest and version changes are surfaced as warnings.
func ResolveContentLock(paths *core.PathResolver, gameID string) (content.ContentLock, []string, error) {
	gameManifest, err := core.LoadGameManifest(filepath.Join(paths.GameDir(gameID), "game.yaml"))
	if err != nil {
		return content.ContentLock{}, nil, fmt.Errorf("load game manifest: %w", err)
	}

	sysDir := paths.SystemDir(gameManifest.SystemID)
	worldDir := paths.WorldDir(gameManifest.WorldID)

	sysManifest, worldManifest, warnings, err := loadCampaignSources(gameManifest, sysDir, worldDir)
	if err != nil {
		return content.ContentLock{}, nil, err
	}

	// 1. Check requirements
	if err := checkWorldRequires(gameManifest, worldManifest, sysManifest); err != nil {
		return content.ContentLock{}, nil, err
	}

	// 2. Check lockfile
	lockPath := filepath.Join(paths.GameDir(gameID), "content.lock.yaml")
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		// Campaign has no lockfile yet: lock on open if content is available
		if sysManifest != nil && worldManifest != nil {
			lock, err := LockContent(paths, gameID)
			if err != nil {
				return content.ContentLock{}, nil, fmt.Errorf("lock content on open: %w", err)
			}
			return lock, warnings, nil
		}
		return content.ContentLock{}, warnings, nil
	}

	lock, err := content.LoadLock(lockPath)
	if err != nil {
		return content.ContentLock{}, nil, fmt.Errorf("load lockfile: %w", err)
	}

	warnings = append(warnings, contentDriftWarnings(gameManifest, sysManifest, worldManifest, lock, sysDir, worldDir)...)

	return lock, warnings, nil
}

// loadCampaignSources loads the system and world a campaign is built from. One that is
// simply absent is a warning rather than a failure, because a campaign can be opened
// while its world or system is still being written; any other failure is an error.
func loadCampaignSources(gameManifest *core.GameManifest, sysDir, worldDir string) (*core.SystemManifest, *core.WorldManifest, []string, error) {
	var warnings []string

	sysManifest, sysErr := core.LoadSystemManifest(filepath.Join(sysDir, "system.yaml"))
	if sysErr != nil {
		if os.IsNotExist(sysErr) || strings.Contains(sysErr.Error(), "no such file or directory") {
			warnings = append(warnings, fmt.Sprintf("system %q is missing on disk", gameManifest.SystemID))
		} else {
			return nil, nil, nil, fmt.Errorf("load system %q: %w", gameManifest.SystemID, sysErr)
		}
	}

	worldManifest, worldErr := core.LoadWorldManifest(filepath.Join(worldDir, "world.yaml"))
	if worldErr != nil {
		if os.IsNotExist(worldErr) || strings.Contains(worldErr.Error(), "no such file or directory") {
			warnings = append(warnings, fmt.Sprintf("world %q is missing on disk", gameManifest.WorldID))
		} else {
			return nil, nil, nil, fmt.Errorf("load world %q: %w", gameManifest.WorldID, worldErr)
		}
	}

	return sysManifest, worldManifest, warnings, nil
}

// checkWorldRequires enforces the version constraints a world places on the system it
// runs on. A requirement naming another type, or a system other than this one, is not
// this campaign's to answer for, so it is passed over.
//
// Nothing is checked when either manifest is missing: the world's requirements cannot
// be read without them, and the absence has already been reported as a warning.
func checkWorldRequires(gameManifest *core.GameManifest, worldManifest *core.WorldManifest, sysManifest *core.SystemManifest) error {
	if sysManifest == nil || worldManifest == nil {
		return nil
	}
	return checkRequires(worldManifest.Requires, gameManifest.SystemID, gameManifest.WorldID, sysManifest.Version)
}

// checkRequires enforces the version constraints a world places on the system it runs
// on. A requirement naming another type, or a system other than this one, is not this
// campaign's to answer for, so it is passed over.
//
// It is shared because a campaign is checked twice against the same rule: once when it
// is created, where the ids come from the request, and once every time it is opened,
// where they come from its own manifest.
func checkRequires(requires []core.ContentRequirement, systemID, worldID, systemVersion string) error {
	for _, req := range requires {
		if req.Type != "" && req.Type != "system" {
			continue
		}
		if req.ID != "" && req.ID != systemID {
			continue
		}
		if !req.Satisfies(systemVersion) {
			return fmt.Errorf("incompatible system %q: world %q requires version constraint %q, but system version is %q", systemID, worldID, req.Version, systemVersion)
		}
	}
	return nil
}

// contentDriftWarnings is what a lockfile raises against the content now on disk: a
// digest that changed, a version that changed, and an entry the lockfile does not
// hold at all.
//
// Content whose digest cannot be taken is skipped rather than reported, and that is
// also what keeps the version comparison safe: a directory with no behavioural files
// is a content half that is not on disk, which is why its manifest is nil.
func contentDriftWarnings(gameManifest *core.GameManifest, sysManifest *core.SystemManifest, worldManifest *core.WorldManifest, lock content.ContentLock, sysDir, worldDir string) []string {
	var warnings []string

	// Current digests
	currentSysDigest, err := content.BehaviouralDigest(sysDir)
	if err == nil {
		if entry, ok := lock.FindEntry("system", gameManifest.SystemID); ok {
			if entry.SHA256 != "" && entry.SHA256 != currentSysDigest {
				warnings = append(warnings, fmt.Sprintf("system %q digest changed (was %s, now %s)", gameManifest.SystemID, entry.SHA256, currentSysDigest))
			}
			if entry.Version != "" && sysManifest.Version != "" && entry.Version != sysManifest.Version {
				warnings = append(warnings, fmt.Sprintf("system %q version changed (was %s, now %s)", gameManifest.SystemID, entry.Version, sysManifest.Version))
			}
		} else {
			warnings = append(warnings, fmt.Sprintf("system %q was not in the lockfile", gameManifest.SystemID))
		}
	}

	currentWorldDigest, err := content.BehaviouralDigest(worldDir)
	if err == nil {
		if entry, ok := lock.FindEntry("world", gameManifest.WorldID); ok {
			if entry.SHA256 != "" && entry.SHA256 != currentWorldDigest {
				warnings = append(warnings, fmt.Sprintf("world %q digest changed (was %s, now %s)", gameManifest.WorldID, entry.SHA256, currentWorldDigest))
			}
			if entry.Version != "" && worldManifest.Version != "" && entry.Version != worldManifest.Version {
				warnings = append(warnings, fmt.Sprintf("world %q version changed (was %s, now %s)", gameManifest.WorldID, entry.Version, worldManifest.Version))
			}
		} else {
			warnings = append(warnings, fmt.Sprintf("world %q was not in the lockfile", gameManifest.WorldID))
		}
	}

	return warnings
}
