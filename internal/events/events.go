// Package events implements the NDJSON status event emitter per SPEC §9.
//
// The emitter is the contract surface that downstream consumers (Paperclip
// adapters, CI integrations, governance tools) pin to. The envelope
// (ts, level, event, session_id, project, data) and the 9 event types in
// §9.2 are STABLE for v0.x.
package events

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Level is the severity of a status event. Only "info" | "warn" | "error"
// per the schema.
type Level string

const (
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// Envelope is the top-level structure of every status event. Field names
// are the stable contract (SPEC §9.4). Do not rename.
type Envelope struct {
	TS        time.Time   `json:"ts"`
	Level     Level       `json:"level"`
	Event     string      `json:"event"`
	SessionID string      `json:"session_id"`
	Project   string      `json:"project"`
	Data      interface{} `json:"data"`
}

// Event type names from SPEC §9.2. Re-exported as constants so callers
// don't pass string literals.
const (
	EventRunStart        = "run.start"
	EventRunEnd          = "run.end"
	EventIterationStart  = "iteration.start"
	EventIterationEnd    = "iteration.end"
	EventToolCall        = "tool_call"
	EventCommit          = "commit"
	EventPush            = "push"
	// EventPushSkipped is emitted when the loop intentionally does not
	// run `git push` (e.g. no origin remote configured). Added in v0.1.2
	// (issue #8); additive, not a breaking change to the schema.
	EventPushSkipped     = "push.skipped"
	EventAbortRequested  = "abort.requested"
	EventError           = "error"
)

// Emitter writes status events as NDJSON to its writer. Safe for
// concurrent use from the loop driver (one goroutine per iteration could
// race on the writer; the mutex serializes the actual Write).
type Emitter struct {
	mu        sync.Mutex
	w         io.Writer
	sessionID string
	project   string
	enc       *json.Encoder
}

// NewEmitter returns an Emitter that writes to w, stamping every event
// with sessionID and project. If w is nil, defaults to os.Stdout.
func NewEmitter(w io.Writer, sessionID, project string) *Emitter {
	if w == nil {
		w = os.Stdout
	}
	return &Emitter{
		w:         w,
		sessionID: sessionID,
		project:   project,
		enc:       json.NewEncoder(w),
	}
}

// Emit writes one event as a single JSON line (NDJSON format).
// Errors from the underlying Write are returned so the loop driver can
// log them; the event is still considered "sent" once the bytes are
// flushed.
func (e *Emitter) Emit(level Level, event string, data interface{}) error {
	env := Envelope{
		TS:        time.Now().UTC(),
		Level:     level,
		Event:     event,
		SessionID: e.sessionID,
		Project:   e.project,
		Data:      data,
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	// json.Encoder always appends a newline — perfect for NDJSON.
	return e.enc.Encode(env)
}

// Info is a convenience for info-level events.
func (e *Emitter) Info(event string, data interface{}) error {
	return e.Emit(LevelInfo, event, data)
}

// Warn is a convenience for warn-level events.
func (e *Emitter) Warn(event string, data interface{}) error {
	return e.Emit(LevelWarn, event, data)
}

// Error is a convenience for error-level events. Note: this is a status
// event, not a Go error. To surface a hard error, use EmitError below.
func (e *Emitter) Error(event string, data interface{}) error {
	return e.Emit(LevelError, event, data)
}

// EmitError is the canonical "error" status event. The code field is a
// short stable string ("harness_missing", "git_push_failed", etc.) and
// message is the human-readable detail.
func (e *Emitter) EmitError(code, message string) error {
	return e.Emit(LevelError, EventError, map[string]string{
		"code":    code,
		"message": message,
	})
}

// NewSessionID returns a fresh opaque session id of the form "ses_<ulid-ish>".
//
// We don't need true ULID monotonicity for v0.1 (Paperclip won't care
// about ordering across hosts in v0.1), so a 16-byte hex string is
// enough. If we ever need monotonic IDs, swap in oklog/ulid.
func NewSessionID() string {
	return fmt.Sprintf("ses_%016x", newULIDish())
}

// newULIDish returns 8 random bytes encoded as 16 hex chars.
// Indirected through a package-level func so tests can stub it.
var newULIDish = func() []byte {
	var b [8]byte
	if _, err := readRandom(b[:]); err != nil {
		// Crypto rand shouldn't fail in practice; fall back to time.
		ts := time.Now().UnixNano()
		for i := 0; i < 8; i++ {
			b[i] = byte(ts >> (8 * i))
		}
	}
	return b[:]
}

// readRandom is a package-level indirection so tests can substitute
// a deterministic source if needed.
var readRandom = func(b []byte) (int, error) {
	return readRandomDefault(b)
}

func readRandomDefault(b []byte) (int, error) {
	// crypto/rand via os-level indirection
	return readCryptoRand(b)
}
