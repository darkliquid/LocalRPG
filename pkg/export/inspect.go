package export

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/darkliquid/localrpg/pkg/media"
)

// bundleHolderPattern finds the compressed story a bundle carries.
var bundleHolderPattern = regexp.MustCompile(`<script id="localrpg-bundle" type="application/octet-stream">([^<]+)</script>`)

// ClipReport is one clip a bundle carries, and whether a browser will play it.
type ClipReport struct {
	Bytes     int
	Complete  bool
	Decodable bool
	Problem   string
}

// BeatReport is one beat of a bundle: what it says, and what it can play.
type BeatReport struct {
	Kind    string
	Speaker string
	Text    string
	Clips   []ClipReport
}

// InspectBundle reads a bundle and reports what each beat can play, so a bundle a browser
// refuses can be diagnosed without the app that made it, and without a browser.
func InspectBundle(path string) ([]BeatReport, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read bundle: %w", err)
	}

	match := bundleHolderPattern.FindSubmatch(raw)
	if match == nil {
		return nil, fmt.Errorf("%s is not a bundle: no compressed story in it", path)
	}

	packed, err := base64.StdEncoding.DecodeString(string(match[1]))
	if err != nil {
		return nil, fmt.Errorf("decode bundle: %w", err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(packed))
	if err != nil {
		return nil, fmt.Errorf("open bundle: %w", err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read bundle: %w", err)
	}

	var bundle struct {
		Story json.RawMessage `json:"story"`
	}
	if err := json.Unmarshal(decoded, &bundle); err != nil {
		return nil, fmt.Errorf("decode story: %w", err)
	}

	var story struct {
		Scenes []struct {
			Beats []struct {
				Kind    string   `json:"kind"`
				Speaker string   `json:"speaker"`
				Text    string   `json:"text"`
				Audio   []string `json:"audio"`
			} `json:"beats"`
		} `json:"scenes"`
	}
	if err := json.Unmarshal(bundle.Story, &story); err != nil {
		return nil, fmt.Errorf("decode scenes: %w", err)
	}

	beats := make([]BeatReport, 0, 8)
	for _, scene := range story.Scenes {
		for _, beat := range scene.Beats {
			report := BeatReport{Kind: beat.Kind, Speaker: beat.Speaker, Text: beat.Text}
			for _, uri := range beat.Audio {
				report.Clips = append(report.Clips, inspectClip(uri))
			}
			beats = append(beats, report)
		}
	}
	return beats, nil
}

// inspectClip reports a clip a bundle carries, from the bytes the browser will be handed.
func inspectClip(uri string) ClipReport {
	data, err := decodeDataURI(uri)
	if err != nil {
		return ClipReport{Problem: err.Error()}
	}

	problem := media.ClipProblem(data)
	report := ClipReport{Bytes: len(data), Complete: problem == nil, Decodable: problem == nil}
	if problem != nil {
		report.Problem = problem.Error()
	}
	return report
}

// decodeDataURI reads the bytes out of a data URI.
func decodeDataURI(uri string) ([]byte, error) {
	comma := strings.Index(uri, ",")
	if comma < 0 || !strings.HasPrefix(uri, "data:") {
		return nil, fmt.Errorf("not a data URI")
	}
	meta, payload := uri[:comma], uri[comma+1:]
	if !strings.Contains(meta, ";base64") {
		return nil, fmt.Errorf("data URI is not base64: %s", meta)
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("decode data URI: %w", err)
	}
	return data, nil
}
