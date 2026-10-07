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

	var warnings []string

	sysDir := paths.SystemDir(gameManifest.SystemID)
	sysManifest, sysErr := core.LoadSystemManifest(filepath.Join(sysDir, "system.yaml"))
	if sysErr != nil {
		if os.IsNotExist(sysErr) || strings.Contains(sysErr.Error(), "no such file or directory") {
			warnings = append(warnings, fmt.Sprintf("system %q is missing on disk", gameManifest.SystemID))
		} else {
			return content.ContentLock{}, nil, fmt.Errorf("load system %q: %w", gameManifest.SystemID, sysErr)
		}
	}

	worldDir := paths.WorldDir(gameManifest.WorldID)
	worldManifest, worldErr := core.LoadWorldManifest(filepath.Join(worldDir, "world.yaml"))
	if worldErr != nil {
		if os.IsNotExist(worldErr) || strings.Contains(worldErr.Error(), "no such file or directory") {
			warnings = append(warnings, fmt.Sprintf("world %q is missing on disk", gameManifest.WorldID))
		} else {
			return content.ContentLock{}, nil, fmt.Errorf("load world %q: %w", gameManifest.WorldID, worldErr)
		}
	}

	// 1. Check requirements
	if sysManifest != nil && worldManifest != nil {
		for _, req := range worldManifest.Requires {
			if req.Type == "" || req.Type == "system" {
				if req.ID == "" || req.ID == gameManifest.SystemID {
					if !req.Satisfies(sysManifest.Version) {
						return content.ContentLock{}, nil, fmt.Errorf("incompatible system %q: world %q requires version constraint %q, but system version is %q", gameManifest.SystemID, gameManifest.WorldID, req.Version, sysManifest.Version)
					}
				}
			}
		}
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

	return lock, warnings, nil
}
