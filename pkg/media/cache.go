package media

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func ComputeAudioCacheKey(speakerID, voiceConfigHash, utteranceText string) string {
	hasher := sha256.New()
	hasher.Write([]byte(speakerID + ":" + voiceConfigHash + ":" + utteranceText))
	return hex.EncodeToString(hasher.Sum(nil))
}

func ComputeAudioCacheKeyWithRate(speakerID, voiceID string, pitch, speechRate float64, text string) string {
	raw := fmt.Sprintf("%s:%s:%.2f:%.2f:%s", speakerID, voiceID, pitch, speechRate, text)
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])
}

// ComputeAudioCacheKeyForVoice hashes everything that changes a clip: provider,
// voice, prosody, provider options, and text. A voice with no options falls back
// to the legacy key, so the value token a client already holds stays valid.
func ComputeAudioCacheKeyForVoice(speakerID string, voice *entity.VoiceConfig, text string) string {
	if voice == nil || len(voice.Options) == 0 {
		voiceID, pitch, rate := "", 0.0, 0.0
		if voice != nil {
			voiceID, pitch, rate = voice.VoiceID, voice.Pitch, voice.SpeechRate
		}
		return ComputeAudioCacheKeyWithRate(speakerID, voiceID, pitch, rate, text)
	}

	// encoding/json sorts map keys, so the options hash is deterministic
	// regardless of insertion order.
	payload := struct {
		Provider   string         `json:"provider"`
		VoiceID    string         `json:"voice_id"`
		Pitch      float64        `json:"pitch"`
		SpeechRate float64        `json:"speech_rate"`
		Options    map[string]any `json:"options"`
	}{
		Provider:   voice.Provider,
		VoiceID:    voice.VoiceID,
		Pitch:      voice.Pitch,
		SpeechRate: voice.SpeechRate,
		Options:    voice.Options,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		// A map of canonical scalars cannot fail to marshal; degrade rather than
		// panic so a hand-edited note never loses a turn.
		encoded = []byte(voice.Provider + "|" + voice.VoiceID)
	}

	hash := sha256.Sum256([]byte("v2:" + speakerID + ":" + string(encoded)))
	return hex.EncodeToString(hash[:])
}

func ComputeArtCacheKey(entityID, appearanceHash, worldStyleHash string) string {
	hasher := sha256.New()
	hasher.Write([]byte(entityID + ":" + appearanceHash + ":" + worldStyleHash))
	return hex.EncodeToString(hasher.Sum(nil))
}

type ContentCache struct {
	baseDir string
}

func NewContentCache(baseDir string) *ContentCache {
	return &ContentCache{baseDir: baseDir}
}

func (c *ContentCache) Subdir(category string) string {
	dir := filepath.Join(c.baseDir, category)
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func (c *ContentCache) Exists(category, filename string) bool {
	path := filepath.Join(c.Subdir(category), filename)
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func (c *ContentCache) Put(category, filename string, data []byte) (string, error) {
	path := filepath.Join(c.Subdir(category), filename)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("write cache file: %w", err)
	}
	return path, nil
}

func (c *ContentCache) Get(category, filename string) ([]byte, error) {
	path := filepath.Join(c.Subdir(category), filename)
	return os.ReadFile(path)
}
