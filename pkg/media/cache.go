package media

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	// regardless of insertion order. Text is part of the payload: without it
	// every utterance under a voice with options shares one key and serves the
	// same clip, which is what a narrated turn did.
	payload := struct {
		Provider   string                 `json:"provider"`
		VoiceID    string                 `json:"voice_id"`
		Pitch      float64                `json:"pitch"`
		SpeechRate float64                `json:"speech_rate"`
		Options    map[string]interface{} `json:"options"`
		Text       string                 `json:"text"`
	}{
		Provider:   voice.Provider,
		VoiceID:    voice.VoiceID,
		Pitch:      voice.Pitch,
		SpeechRate: voice.SpeechRate,
		Options:    voice.Options,
		Text:       text,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		// A map of canonical scalars cannot fail to marshal; degrade rather than
		// panic so a hand-edited note never loses a turn.
		encoded = []byte(voice.Provider + "|" + voice.VoiceID + "|" + text)
	}

	// v3: the options-aware key gained the utterance text, so keys written under
	// v2 were shared between utterances and are deliberately abandoned.
	hash := sha256.Sum256([]byte("v3:" + speakerID + ":" + string(encoded)))
	return hex.EncodeToString(hash[:])
}

// ClipKeyForPath names the key a stored clip was written under: the file name is
// the key, so nothing else has to be consulted to name the audio it holds.
func ClipKeyForPath(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
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

// Put stores a cache file by writing it beside its name and renaming it into place, so a
// reader never sees a half-written file. The cache is read while it is written: two
// goroutines synthesizing the same utterance, or an export reading clips the app is
// generating, would otherwise see a partial clip and mistake it for a broken one.
func (c *ContentCache) Put(category, filename string, data []byte) (string, error) {
	dir := c.Subdir(category)
	path := filepath.Join(dir, filename)

	temp, err := os.CreateTemp(dir, filename+".*.part")
	if err != nil {
		return "", fmt.Errorf("write cache file: %w", err)
	}
	name := temp.Name()

	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		_ = os.Remove(name)
		return "", fmt.Errorf("write cache file: %w", err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(name)
		return "", fmt.Errorf("write cache file: %w", err)
	}
	if err := os.Chmod(name, 0644); err != nil {
		_ = os.Remove(name)
		return "", fmt.Errorf("write cache file: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return "", fmt.Errorf("write cache file: %w", err)
	}

	return path, nil
}

func (c *ContentCache) Get(category, filename string) ([]byte, error) {
	path := filepath.Join(c.Subdir(category), filename)
	return os.ReadFile(path)
}
