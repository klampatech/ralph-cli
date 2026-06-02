package harness

import (
	"context"
	"fmt"
	"io"
)

// Mock is a Harness implementation for tests. It records every Invoke call
// in Calls and returns the scripted exit code for that call.
//
// Use NewMock() for a default-success mock, or NewMockWithExits(...) to
// script a sequence of exit codes (the last one is reused for any further
// calls). This is the canonical "harness mock" mentioned in SPEC §14.1:
// "Mock harness for tests (records argv + stdin)".
type Mock struct {
	Calls    []MockCall
	script   []int
	nextIdx  int
	exitCode int
}

// MockCall is a single recorded Invoke.
type MockCall struct {
	Mode        Mode
	Prompt      string
	Model       string
	ProjectRoot string
	Iteration   int
	LoopCount   int
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	Command     string
}

// NewMock returns a Mock that returns exit 0 for every Invoke.
func NewMock() *Mock {
	return &Mock{script: []int{0}}
}

// NewMockWithExits configures a script of exit codes returned in order
// for each Invoke call. The last code is reused once the script is
// exhausted, so callers can write NewMockWithExits(0, 0, 1) for
// "first two succeed, third and beyond fail with exit 1".
func NewMockWithExits(codes ...int) *Mock {
	if len(codes) == 0 {
		codes = []int{0}
	}
	return &Mock{script: codes}
}

// Name implements Harness.
func (m *Mock) Name() string { return "mock" }

// Version implements Harness. Returns a fixed string for tests.
func (m *Mock) Version() (string, error) { return "mock-1.0.0", nil }

// Invoke records the call and returns the next scripted exit code.
func (m *Mock) Invoke(ctx context.Context, req InvokeRequest) (Result, error) {
	idx := m.nextIdx
	if idx >= len(m.script) {
		idx = len(m.script) - 1
	}
	rec := MockCall{
		Mode:        req.Mode,
		Prompt:      req.Prompt,
		Model:       req.Model,
		ProjectRoot: req.ProjectRoot,
		Iteration:   req.IterationCount,
		LoopCount:   req.LoopCount,
		Stdin:       req.Stdin,
		Stdout:      req.Stdout,
		Stderr:      req.Stderr,
		Command:     req.Command,
	}
	m.Calls = append(m.Calls, rec)
	m.nextIdx++

	// Default mock behavior: write the prompt to stdout (if a writer was
	// provided) and return the scripted exit code. This lets the loop
	// driver think the iteration succeeded without a real claude binary.
	if req.Stdout != nil {
		_, _ = fmt.Fprintln(req.Stdout, req.Prompt)
	}
	return Result{ExitCode: m.script[idx]}, nil
}

// PromptFor returns the prompt text that was passed on the Nth call.
func (m *Mock) PromptFor(n int) string {
	if n >= len(m.Calls) {
		return ""
	}
	return m.Calls[n].Prompt
}
