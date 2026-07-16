package discoverycron

import (
	"fmt"
	"log"
	"strings"
)

// stdLogger is a minimal key-value logger over the standard library's log
// package, matching the shape (message + alternating key/value pairs) that
// activities.DiscoveryActivities' Temporal-based logging already uses, so
// discovery_trace-tagged log lines look the same regardless of which path
// produced them.
type stdLogger struct{}

func newStdLogger() *stdLogger { return &stdLogger{} }

func (l *stdLogger) Info(msg string, kv ...any)  { l.log("INFO", msg, kv) }
func (l *stdLogger) Warn(msg string, kv ...any)  { l.log("WARN", msg, kv) }
func (l *stdLogger) Error(msg string, kv ...any) { l.log("ERROR", msg, kv) }

func (l *stdLogger) log(level, msg string, kv []any) {
	var b strings.Builder
	b.WriteString(level)
	b.WriteString("  ")
	b.WriteString(msg)
	for i := 0; i+1 < len(kv); i += 2 {
		fmt.Fprintf(&b, " %v=%v", kv[i], kv[i+1])
	}
	log.Println(b.String())
}
