package transactioncron

import (
	"fmt"
	"log"
	"strings"
)

// stdLogger is a minimal key-value logger over the standard library's log
// package, mirroring internal/discoverycron's stdLogger so discovery- and
// transaction-cron log lines have the same shape in journald.
type stdLogger struct{}

func newStdLogger() *stdLogger { return &stdLogger{} }

func (l *stdLogger) Info(msg string, kv ...any) { l.log("INFO", msg, kv) }
func (l *stdLogger) Warn(msg string, kv ...any) { l.log("WARN", msg, kv) }

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
