package media

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
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
