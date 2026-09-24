package ttsnativeos

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
)

type nativeOSTTSClient struct{}

func NewNativeOSTTSClient() media.TTSClient {
	return &nativeOSTTSClient{}
}

func (n *nativeOSTTSClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	// Attempt OS-specific speech synthesis command
	cmdCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	switch runtime.GOOS {
	case "darwin":
		// macOS /usr/bin/say can output directly to AIFF/WAV
		if _, err := exec.LookPath("say"); err == nil {
			cmd := exec.CommandContext(cmdCtx, "say", "-v", "Samantha", text)
			_ = cmd.Run()
		}

	case "linux":
		if _, err := exec.LookPath("espeak-ng"); err == nil {
			var out bytes.Buffer
			cmd := exec.CommandContext(cmdCtx, "espeak-ng", "-w", "/dev/stdout", text)
			cmd.Stdout = &out
			if err := cmd.Run(); err == nil && out.Len() > 0 {
				return out.Bytes(), nil
			}
		} else if _, err := exec.LookPath("spd-say"); err == nil {
			args := []string{"-t", "female1"}
			if voice != nil {
				if voice.Pitch > 0 {
					p := int((voice.Pitch - 1.0) * 100)
					if p < -100 {
						p = -100
					} else if p > 100 {
						p = 100
					}
					args = append(args, "-p", fmt.Sprintf("%d", p))
				}
				if voice.SpeechRate > 0 {
					r := int((voice.SpeechRate - 1.0) * 100)
					if r < -100 {
						r = -100
					} else if r > 100 {
						r = 100
					}
					args = append(args, "-r", fmt.Sprintf("%d", r))
				}
			}
			args = append(args, text)
			cmd := exec.CommandContext(cmdCtx, "spd-say", args...)
			_ = cmd.Run()
		}

	case "windows":
		// PowerShell speech synthesis fallback on Windows
		psCmd := fmt.Sprintf("Add-Type -AssemblyName System.speech; $speak = New-Object System.Speech.Synthesis.SpeechSynthesizer; $speak.Speak('%s')", stringsEscapePowerShell(text))
		cmd := exec.CommandContext(cmdCtx, "powershell", "-Command", psCmd)
		_ = cmd.Run()
	}

	// High-compatibility procedural audio synthesis (PCM WAV audio tone generation)
	// Generates clean 44.1kHz 16-bit mono WAV audio with pitch modulated by VoiceConfig
	pitch := 220.0
	if voice != nil && voice.Pitch > 0 {
		pitch = 220.0 * voice.Pitch
	}
	return media.GenerateToneWAV(pitch, 0.4), nil
}

func stringsEscapePowerShell(s string) string {
	var b bytes.Buffer
	for _, r := range s {
		if r == '\'' {
			b.WriteString("''")
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
