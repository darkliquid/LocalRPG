package media

import (
	"errors"
	"html"
	"regexp"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
)

// ErrNoSpeakableText reports that a segment reduced to nothing and has no audio.
var ErrNoSpeakableText = errors.New("segment has no speakable text")

// MarkdownAware clients consume narrator Markdown themselves, for example an
// engine that maps emphasis to SSML. A client that does not implement this is
// sent plain speakable text.
type MarkdownAware interface {
	SupportsMarkdown() bool
}

// TextPolicy selects how narration Markdown is treated before synthesis.
type TextPolicy int

const (
	// TextPolicyAuto reduces Markdown unless the client is MarkdownAware.
	TextPolicyAuto TextPolicy = iota
	// TextPolicyStrip always reduces Markdown.
	TextPolicyStrip
	// TextPolicyKeep sends the raw text unchanged.
	TextPolicyKeep
)

// TextPolicyFromConfig maps a configuration string to a policy. An empty or
// unrecognised value is treated as auto.
func TextPolicyFromConfig(cfg config.TTSConfig) TextPolicy {
	switch strings.ToLower(strings.TrimSpace(cfg.Markdown)) {
	case "strip":
		return TextPolicyStrip
	case "keep":
		return TextPolicyKeep
	default:
		return TextPolicyAuto
	}
}

// SpeakableTextFor applies a policy to one segment's text, preserving or stripping
// Markdown and performance audio tags according to client capabilities and policy.
func SpeakableTextFor(policy TextPolicy, client TTSClient, text string) string {
	var processed string
	switch policy {
	case TextPolicyKeep:
		processed = text
	case TextPolicyStrip:
		processed = SpeakableText(text)
	default:
		if aware, ok := client.(MarkdownAware); ok && aware.SupportsMarkdown() {
			processed = text
		} else {
			processed = SpeakableText(text)
		}
	}

	if !ClientSupportsAudioTags(client) {
		processed = StripAudioTags(processed)
	}

	return strings.TrimSpace(processed)
}

// ClientSupportsAudioTags reports whether a client declares AudioTags support.
func ClientSupportsAudioTags(client TTSClient) bool {
	if client == nil {
		return false
	}
	if adv, ok := client.(SpeechCueAdvertiser); ok {
		return adv.SpeechCueCapabilities().AudioTags
	}
	return false
}

var (
	wikilinkOrAudioTagRe = regexp.MustCompile(`(\[\[[^\]]+\]\])|(\[[a-zA-Z][a-zA-Z\s_-]{1,28}\])`)
)

// StripAudioTags removes bracketed performance tags and normalizes whitespace,
// while preserving double-bracket wikilinks.
func StripAudioTags(text string) string {
	if text == "" {
		return ""
	}
	replaced := wikilinkOrAudioTagRe.ReplaceAllStringFunc(text, func(m string) string {
		if strings.HasPrefix(m, "[[") {
			return m
		}
		return " "
	})
	return strings.TrimSpace(strings.Join(strings.Fields(replaced), " "))
}

var (
	// The fence line itself goes; the words inside a fence are still worth saying.
	codeFenceRe = regexp.MustCompile("(?m)^\\s*```.*$")
	// A scene break or thematic break is a line of three or more identical marks.
	sceneBreakRe = regexp.MustCompile(`(?m)^\s*(?:-{3,}|\*{3,}|_{3,})\s*$`)
	wikilinkRe   = regexp.MustCompile(`\[\[([^\]\|]+)(?:\|([^\]]+))?\]\]`)
	inlineCodeRe = regexp.MustCompile("`([^`\n]+)`")
	tripleEmRe   = regexp.MustCompile(`\*\*\*([^*\n]+)\*\*\*`)
	doubleEmRe   = regexp.MustCompile(`\*\*([^*\n]+)\*\*`)
	starEmRe     = regexp.MustCompile(`\*([^*\n]+)\*`)
	// Underscore emphasis only applies at word boundaries so snake_case survives.
	underscoreEmRe = regexp.MustCompile(`\b_([^_\n]+)_\b`)
	headingRe      = regexp.MustCompile(`(?m)^\s{0,3}#{1,6}\s+`)
	listRe         = regexp.MustCompile(`(?m)^\s*(?:[-*+]|\d+[.)])\s+`)
	quoteRe        = regexp.MustCompile(`(?m)^\s*>\s?`)
)

// SpeakableText reduces narrator Markdown to the words a person would read aloud.
// The grammar mirrors the frontend's constrained MarkdownProse renderer, so what
// is spoken matches what is shown: wikilinks, emphasis, inline code, headings,
// lists, blockquotes, scene breaks, and common HTML entities.
//
// Punctuation prosody depends on is preserved untouched, including em dashes and
// ellipses, which the built-in engine's token set understands.
func SpeakableText(text string) string {
	if text == "" {
		return ""
	}

	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "\t", " ")

	// Scene breaks must go before emphasis, or `***` would be read as emphasis.
	text = sceneBreakRe.ReplaceAllString(text, "\n")
	text = codeFenceRe.ReplaceAllString(text, "")

	text = wikilinkRe.ReplaceAllStringFunc(text, func(match string) string {
		groups := wikilinkRe.FindStringSubmatch(match)
		if len(groups) > 2 && strings.TrimSpace(groups[2]) != "" {
			return strings.TrimSpace(groups[2])
		}
		return strings.TrimSpace(groups[1])
	})

	text = inlineCodeRe.ReplaceAllString(text, "$1")
	text = tripleEmRe.ReplaceAllString(text, "$1")
	text = doubleEmRe.ReplaceAllString(text, "$1")
	text = starEmRe.ReplaceAllString(text, "$1")
	text = underscoreEmRe.ReplaceAllString(text, "$1")

	text = headingRe.ReplaceAllString(text, "")
	text = listRe.ReplaceAllString(text, "")
	text = quoteRe.ReplaceAllString(text, "")

	text = html.UnescapeString(text)

	// Collapse whitespace: a newline is a beat on screen and a pause in speech,
	// which the punctuation already supplies.
	return strings.TrimSpace(strings.Join(strings.Fields(text), " "))
}
