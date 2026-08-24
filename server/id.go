package server

import (
	"fmt"
	"sync/atomic"
	"time"
)

// idCounter disambiguates IDs generated in the same nanosecond — both
// within ingest loops and across concurrent requests. UnixNano alone
// collides on fast machines and coarse-clock platforms; a collision
// surfaces as a spurious 409 or a silently skipped ingest item.
var idCounter atomic.Uint64

// newID returns a unique ID with the given prefix.
func newID(prefix string) string {
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), idCounter.Add(1))
}
