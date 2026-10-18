package harness

import (
	"strconv"
	"sync/atomic"
	"time"
)

var checkIDCounter atomic.Uint64

// NewCheckID returns a process-unique check identifier. It is the single
// definition shared by the engine's default resolver and the rules engine.
func NewCheckID() string {
	seq := checkIDCounter.Add(1)
	return "chk_" + strconv.FormatInt(time.Now().UnixNano(), 36) + strconv.FormatUint(seq, 36)
}
