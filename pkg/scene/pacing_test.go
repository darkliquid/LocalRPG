package scene

import (
	"strings"
	"testing"
)

// TestSilentBeatUsesReadingDuration guards the renderer's reading rule, which the
// theatre mirrors in frontend/src/lib/pacing.ts.
func TestSilentBeatUsesReadingDuration(t *testing.T) {
	short := ReadingDuration("Go.")
	long := ReadingDuration(strings.Repeat("word ", 40))
	if short < MinimumBeatDuration {
		t.Fatalf("short beat = %v, want at least %v", short, MinimumBeatDuration)
	}
	if long <= short {
		t.Fatalf("long beat %v should exceed short beat %v", long, short)
	}
}

func TestReadingDurationFloorsAnEmptyText(t *testing.T) {
	if got := ReadingDuration("   "); got != MinimumBeatDuration {
		t.Fatalf("empty text = %v, want the floor %v", got, MinimumBeatDuration)
	}
}
