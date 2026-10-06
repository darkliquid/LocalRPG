package provider

// Tier is a coarse, user-facing classification of how a provider runs.
type Tier string

const (
	// TierOfflineBasic is pure algorithm or template: no model, no network.
	TierOfflineBasic Tier = "offline-basic"
	// TierOfflineNeural runs a small model in-process on the CPU, no network.
	TierOfflineNeural Tier = "offline-neural"
	// TierLocalServer needs a server the user runs (Ollama, Kokoro-FastAPI,
	// ComfyUI, faster-whisper). Local, but only offline if that server is.
	TierLocalServer Tier = "local-server"
	// TierCloud sends data to a remote provider and needs a key.
	TierCloud Tier = "cloud"
)

// Valid reports whether t is one of the known tiers.
func (t Tier) Valid() bool {
	switch t {
	case TierOfflineBasic, TierOfflineNeural, TierLocalServer, TierCloud:
		return true
	default:
		return false
	}
}

// TierCaveat returns the default honest description of a tier.
func TierCaveat(t Tier) string {
	switch t {
	case TierOfflineBasic:
		return "Deterministic and simple, and it runs entirely on your machine. Its output is more limited and repetitive than a model's."
	case TierOfflineNeural:
		return "Runs a small model on your CPU, entirely on your machine. Quality is well below a large local or cloud model."
	case TierLocalServer:
		return "Needs a server you run yourself. Local, but only offline while that server is."
	case TierCloud:
		return "Sends your text to a remote provider and needs an API key. Metered in most cases."
	}
	return ""
}

// EffectiveCaveat returns a descriptor's own caveat when it sets one, else the
// tier's default. It is what the catalogue and the docs show.
func (d Descriptor) EffectiveCaveat() string {
	if d.Caveat != "" {
		return d.Caveat
	}
	return TierCaveat(d.Tier)
}
