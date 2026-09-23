package media

import "github.com/darkliquid/localrpg/pkg/config"

// FilterVoiceProfiles returns the profiles the active provider can synthesise. A
// profile with no provider is portable and always kept; a profile bound to
// another provider is excluded from assignment rather than removed, so switching
// back restores it.
func FilterVoiceProfiles(profiles []config.VoiceProfile, activeProvider string) []config.VoiceProfile {
	kept := make([]config.VoiceProfile, 0, len(profiles))
	for _, profile := range profiles {
		if profile.Provider == "" || profile.Provider == activeProvider {
			kept = append(kept, profile)
		}
	}
	return kept
}
