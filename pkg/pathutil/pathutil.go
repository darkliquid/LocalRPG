package pathutil

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// ErrEmptyID is returned when an identifier is empty or whitespace.
	ErrEmptyID = errors.New("identifier cannot be empty")
	// ErrPathTraversal is returned when an identifier contains directory traversal tokens.
	ErrPathTraversal = errors.New("identifier cannot contain path separators or traversal sequences")
	// ErrInvalidID is returned when an identifier contains illegal characters.
	ErrInvalidID = errors.New("identifier contains invalid characters")

	validIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

// ValidateID ensures an identifier consists strictly of alphanumeric characters,
// dashes, or underscores, and contains no directory separators or traversal tokens.
func ValidateID(id string) error {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return ErrEmptyID
	}
	if strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") || strings.Contains(trimmed, "..") || strings.ContainsRune(trimmed, 0) {
		return ErrPathTraversal
	}
	if !validIDPattern.MatchString(trimmed) {
		return fmt.Errorf("%w: %q", ErrInvalidID, trimmed)
	}
	return nil
}

// SanitizeID normalizes an input string into a safe identifier,
// guaranteeing that path separators and traversal tokens are stripped.
func SanitizeID(id string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return "unnamed"
	}
	var buf strings.Builder
	precededBySeparator := true
	for _, r := range strings.ToLower(trimmed) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			buf.WriteRune(r)
			precededBySeparator = false
		case r == '_':
			buf.WriteRune('_')
			precededBySeparator = false
		case r == ' ' || r == '-' || r == '/' || r == '\\' || r == '.':
			if buf.Len() > 0 && !precededBySeparator {
				buf.WriteRune('-')
				precededBySeparator = true
			}
		}
	}
	res := strings.Trim(buf.String(), "-_")
	if res == "" {
		return "unnamed"
	}
	return res
}

// ResolveSafeChild resolves relativePath under baseDir and verifies that the
// resulting path remains strictly contained within baseDir.
func ResolveSafeChild(baseDir, relativePath string) (string, error) {
	if filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("path %q is absolute", relativePath)
	}
	cleanBase := filepath.Clean(baseDir)
	target := filepath.Clean(filepath.Join(cleanBase, relativePath))

	rel, err := filepath.Rel(cleanBase, target)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return "", fmt.Errorf("path %q escapes directory %q", relativePath, baseDir)
	}
	return target, nil
}

// ValidateUserPath normalizes an intentional user-provided path, rejecting null
// bytes and control characters while preserving intended directory locations.
func ValidateUserPath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("path cannot be empty")
	}
	if strings.ContainsRune(trimmed, 0) {
		return "", errors.New("path contains null byte")
	}
	return filepath.Clean(trimmed), nil
}
