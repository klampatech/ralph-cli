package events

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEmitterNDJSONFormat(t *testing.T) {
	var buf bytes.Buffer
	e := NewEmitter(&buf, "ses_TEST", "/tmp/proj")

	if err := e.Info(EventRunStart, map[string]any{"command": "loop", "model": "opus"}); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if err := e.EmitError("harness_missing", "claude not found on PATH"); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d:\n%s", len(lines), buf.String())
	}

	// Line 1: run.start, info, contains the data fields.
	var env1 Envelope
	if err := json.Unmarshal([]byte(lines[0]), &env1); err != nil {
		t.Fatalf("unmarshal line 1: %v", err)
	}
	if env1.Event != EventRunStart {
		t.Errorf("Event: got %q, want %q", env1.Event, EventRunStart)
	}
	if env1.Level != LevelInfo {
		t.Errorf("Level: got %q, want %q", env1.Level, LevelInfo)
	}
	if env1.SessionID != "ses_TEST" {
		t.Errorf("SessionID: got %q, want %q", env1.SessionID, "ses_TEST")
	}
	if env1.Project != "/tmp/proj" {
		t.Errorf("Project: got %q, want %q", env1.Project, "/tmp/proj")
	}
	if env1.TS.IsZero() {
		t.Error("TS should be set")
	}
	// Data is a map; spot-check the "command" field round-trips.
	d1, ok := env1.Data.(map[string]any)
	if !ok {
		t.Fatalf("Data is not a map: %T", env1.Data)
	}
	if d1["command"] != "loop" {
		t.Errorf("Data.command: got %v, want %q", d1["command"], "loop")
	}

	// Line 2: error, error level.
	var env2 Envelope
	if err := json.Unmarshal([]byte(lines[1]), &env2); err != nil {
		t.Fatalf("unmarshal line 2: %v", err)
	}
	if env2.Event != EventError {
		t.Errorf("Event: got %q, want %q", env2.Event, EventError)
	}
	if env2.Level != LevelError {
		t.Errorf("Level: got %q, want %q", env2.Level, LevelError)
	}
}

func TestEmitterConcurrentSafe(t *testing.T) {
	var buf bytes.Buffer
	e := NewEmitter(&buf, "ses_TEST", "/tmp/proj")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = e.Info("test", map[string]int{"i": 1})
		}()
	}
	wg.Wait()

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 50 {
		t.Errorf("expected 50 lines, got %d", len(lines))
	}
	for i, line := range lines {
		var env Envelope
		if err := json.Unmarshal([]byte(line), &env); err != nil {
			t.Errorf("line %d not valid JSON: %v\n%s", i, err, line)
		}
	}
}

func TestEmitterSessionID(t *testing.T) {
	sid := NewSessionID()
	if !strings.HasPrefix(sid, "ses_") {
		t.Errorf("session id should start with 'ses_', got %q", sid)
	}
	if len(sid) != 4+16 {
		t.Errorf("session id length: got %d, want 20", len(sid))
	}
}

func TestEmitterTimestampIsUTC(t *testing.T) {
	var buf bytes.Buffer
	e := NewEmitter(&buf, "ses_TEST", "/tmp/proj")
	_ = e.Info("test", nil)

	var env Envelope
	if err := json.Unmarshal(bytes.TrimRight(buf.Bytes(), "\n"), &env); err != nil {
		t.Fatal(err)
	}
	loc := env.TS.Location()
	if loc != time.UTC {
		t.Errorf("TS location: got %v, want UTC", loc)
	}
}
