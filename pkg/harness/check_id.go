package harness

import (
	"strconv"
	"time"
)

// NewCheckID returns a process-unique check identifier. It is the single
// definition shared by the engine's default resolver and the rules engine.
func NewCheckID() string {
	return "chk_" + strconv.FormatInt(time.Now().UnixNano(), 36)
}
