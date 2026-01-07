package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestLogger captures structured logs for assertion in tests.
type TestLogger struct {
	mu      sync.RWMutex
	t       *testing.T
	Entries []LogEntry
	Logger  *slog.Logger
	buffer  *bytes.Buffer
}

// LogEntry represents a captured log entry.
type LogEntry struct {
	Time    time.Time
	Level   slog.Level
	Message string
	Attrs   map[string]any
	Raw     string  // The raw log line
}

// NewTestLogger creates a logger that captures all log entries for testing.
func NewTestLogger(t *testing.T) *TestLogger {
	t.Helper()

	tl := &TestLogger{
		t:       t,
		Entries: make([]LogEntry, 0),
		buffer:  &bytes.Buffer{},
	}

	// Create a handler that writes to our buffer
	handler := &captureHandler{
		testLogger: tl,
		handler:    slog.NewJSONHandler(tl.buffer, &slog.HandlerOptions{Level: slog.LevelDebug}),
	}

	tl.Logger = slog.New(handler)
	return tl
}

// captureHandler wraps a slog handler to capture entries.
type captureHandler struct {
	testLogger *TestLogger
	handler    slog.Handler
	attrs      []slog.Attr  // Accumulated attrs from WithAttrs calls
	group      string       // Current group name
}

func (h *captureHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *captureHandler) Handle(ctx context.Context, r slog.Record) error {
	entry := LogEntry{
		Time:    r.Time,
		Level:   r.Level,
		Message: r.Message,
		Attrs:   make(map[string]any),
	}

	// Include attrs from WithAttrs calls
	for _, a := range h.attrs {
		key := a.Key
		if h.group != "" {
			key = h.group + "." + key
		}
		entry.Attrs[key] = a.Value.Any()
	}

	// Include attrs from the record
	r.Attrs(func(a slog.Attr) bool {
		key := a.Key
		if h.group != "" {
			key = h.group + "." + key
		}
		entry.Attrs[key] = a.Value.Any()
		return true
	})

	h.testLogger.mu.Lock()
	h.testLogger.Entries = append(h.testLogger.Entries, entry)
	h.testLogger.mu.Unlock()

	return h.handler.Handle(ctx, r)
}

func (h *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	return &captureHandler{
		testLogger: h.testLogger,
		handler:    h.handler.WithAttrs(attrs),
		attrs:      newAttrs,
		group:      h.group,
	}
}

func (h *captureHandler) WithGroup(name string) slog.Handler {
	newGroup := name
	if h.group != "" {
		newGroup = h.group + "." + name
	}
	return &captureHandler{
		testLogger: h.testLogger,
		handler:    h.handler.WithGroup(name),
		attrs:      h.attrs,
		group:      newGroup,
	}
}


// GetEntries returns a copy of all captured log entries.
func (l *TestLogger) GetEntries() []LogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]LogEntry, len(l.Entries))
	copy(result, l.Entries)
	return result
}

// GetEntriesOfLevel returns entries at a specific level.
func (l *TestLogger) GetEntriesOfLevel(level slog.Level) []LogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []LogEntry
	for _, e := range l.Entries {
		if e.Level == level {
			result = append(result, e)
		}
	}
	return result
}

// GetEntriesWithAttr returns entries that have a specific attribute.
func (l *TestLogger) GetEntriesWithAttr(key string) []LogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []LogEntry
	for _, e := range l.Entries {
		if _, exists := e.Attrs[key]; exists {
			result = append(result, e)
		}
	}
	return result
}

// GetEntriesWithAttrValue returns entries that have a specific attribute value.
func (l *TestLogger) GetEntriesWithAttrValue(key string, value any) []LogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []LogEntry
	for _, e := range l.Entries {
		if v, exists := e.Attrs[key]; exists && v == value {
			result = append(result, e)
		}
	}
	return result
}

// GetEntriesContaining returns entries whose message contains a substring.
func (l *TestLogger) GetEntriesContaining(substring string) []LogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []LogEntry
	for _, e := range l.Entries {
		if strings.Contains(e.Message, substring) {
			result = append(result, e)
		}
	}
	return result
}

// Count returns the total number of log entries.
func (l *TestLogger) Count() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.Entries)
}

// CountLevel returns the count of entries at a specific level.
func (l *TestLogger) CountLevel(level slog.Level) int {
	l.mu.RLock()
	defer l.mu.RUnlock()

	count := 0
	for _, e := range l.Entries {
		if e.Level == level {
			count++
		}
	}
	return count
}

// Clear removes all captured entries.
func (l *TestLogger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Entries = make([]LogEntry, 0)
	l.buffer.Reset()
}

// GetOutput returns the raw output buffer content.
func (l *TestLogger) GetOutput() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.buffer.String()
}

// Assertion methods

// AssertContains asserts that at least one log entry contains the message.
func (l *TestLogger) AssertContains(t *testing.T, msg string) {
	t.Helper()

	entries := l.GetEntriesContaining(msg)
	if len(entries) == 0 {
		t.Errorf("Expected log to contain message %q, but it wasn't found", msg)
	}
}

// AssertNotContains asserts that no log entry contains the message.
func (l *TestLogger) AssertNotContains(t *testing.T, msg string) {
	t.Helper()

	entries := l.GetEntriesContaining(msg)
	if len(entries) > 0 {
		t.Errorf("Expected log to not contain message %q, but found %d entries", msg, len(entries))
	}
}

// AssertLevel asserts that there are exactly count entries at the given level.
func (l *TestLogger) AssertLevel(t *testing.T, level slog.Level, count int) {
	t.Helper()

	actual := l.CountLevel(level)
	if actual != count {
		t.Errorf("Expected %d entries at level %s, got %d", count, level.String(), actual)
	}
}

// AssertLevelAtLeast asserts that there are at least count entries at the given level.
func (l *TestLogger) AssertLevelAtLeast(t *testing.T, level slog.Level, minCount int) {
	t.Helper()

	actual := l.CountLevel(level)
	if actual < minCount {
		t.Errorf("Expected at least %d entries at level %s, got %d", minCount, level.String(), actual)
	}
}

// AssertNoErrors asserts that there are no ERROR level entries.
func (l *TestLogger) AssertNoErrors(t *testing.T) {
	t.Helper()

	errors := l.GetEntriesOfLevel(slog.LevelError)
	if len(errors) > 0 {
		messages := make([]string, len(errors))
		for i, e := range errors {
			messages[i] = e.Message
		}
		t.Errorf("Expected no errors, got %d: %v", len(errors), messages)
	}
}

// AssertHasError asserts that there is at least one ERROR level entry.
func (l *TestLogger) AssertHasError(t *testing.T) {
	t.Helper()

	errors := l.GetEntriesOfLevel(slog.LevelError)
	if len(errors) == 0 {
		t.Error("Expected at least one error log entry")
	}
}

// AssertErrorContains asserts that an error entry contains the message.
func (l *TestLogger) AssertErrorContains(t *testing.T, msg string) {
	t.Helper()

	errors := l.GetEntriesOfLevel(slog.LevelError)
	for _, e := range errors {
		if strings.Contains(e.Message, msg) {
			return
		}
	}
	t.Errorf("Expected an error containing %q, but none found", msg)
}

// AssertAttrPresent asserts that at least one entry has the given attribute.
func (l *TestLogger) AssertAttrPresent(t *testing.T, key string) {
	t.Helper()

	entries := l.GetEntriesWithAttr(key)
	if len(entries) == 0 {
		t.Errorf("Expected at least one log entry with attribute %q", key)
	}
}

// AssertAttrValue asserts that at least one entry has the attribute with the given value.
func (l *TestLogger) AssertAttrValue(t *testing.T, key string, value any) {
	t.Helper()

	entries := l.GetEntriesWithAttrValue(key, value)
	if len(entries) == 0 {
		t.Errorf("Expected at least one log entry with %s=%v", key, value)
	}
}

// AssertEmpty asserts that no log entries have been captured.
func (l *TestLogger) AssertEmpty(t *testing.T) {
	t.Helper()

	count := l.Count()
	if count != 0 {
		t.Errorf("Expected no log entries, got %d", count)
	}
}

// AssertCount asserts the total number of log entries.
func (l *TestLogger) AssertCount(t *testing.T, expected int) {
	t.Helper()

	actual := l.Count()
	if actual != expected {
		t.Errorf("Expected %d log entries, got %d", expected, actual)
	}
}

// DiscardLogger returns a logger that discards all output.
// Use this for tests that don't need to verify logging.
func DiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelError + 100, // Effectively disable all logging
	}))
}

// NewBufferLogger creates a logger that writes JSON to a buffer.
// Returns both the logger and the buffer for inspection.
func NewBufferLogger(level slog.Level) (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{
		Level: level,
	}))
	return logger, buf
}

// ParseJSONLogs parses JSON-formatted log lines from a buffer.
func ParseJSONLogs(buf *bytes.Buffer) ([]map[string]any, error) {
	var entries []map[string]any
	decoder := json.NewDecoder(buf)

	for decoder.More() {
		var entry map[string]any
		if err := decoder.Decode(&entry); err != nil {
			return entries, err
		}
		entries = append(entries, entry)
	}

	return entries, nil
}
