package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

var (
	replace = func(groups []string, a slog.Attr) slog.Attr {
		// Remove the directory from the source's filename.
		if a.Key == slog.SourceKey {
			source := a.Value.Any().(*slog.Source)
			source.File = filepath.Base(source.File)
		}
		return a
	}
	logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true, ReplaceAttr: replace}))
)

// Infof is an example of a user-defined logging function that wraps slog.
// The log record contains the source position of the caller of Infof.
func Infof(format string, args ...any) {
	if !logger.Enabled(context.Background(), slog.LevelInfo) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // skip [Callers, Infof]
	r := slog.NewRecord(time.Now(), slog.LevelInfo, fmt.Sprintf(format, args...), pcs[0])
	_ = logger.Handler().Handle(context.Background(), r)
}

// Errorf is an example of a user-defined logging function that wraps slog.
// The log record contains the source position of the caller of Errorf.
func Errorf(format string, args ...any) {
	if !logger.Enabled(context.Background(), slog.LevelInfo) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // skip [Callers, Errorf]
	r := slog.NewRecord(time.Now(), slog.LevelError, fmt.Sprintf(format, args...), pcs[0])
	_ = logger.Handler().Handle(context.Background(), r)
}

// Debugf is an example of a user-defined logging function that wraps slog.
// The log record contains the source position of the caller of Debugf.
func Debugf(format string, args ...any) {
	if !logger.Enabled(context.Background(), slog.LevelDebug) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // skip [Callers, Debugf]
	r := slog.NewRecord(time.Now(), slog.LevelDebug, fmt.Sprintf(format, args...), pcs[0])
	_ = logger.Handler().Handle(context.Background(), r)
}

// Warnf is an example of a user-defined logging function that wraps slog.
// The log record contains the source position of the caller of Warnf.
func Warnf(format string, args ...any) {
	if !logger.Enabled(context.Background(), slog.LevelWarn) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // skip [Callers, Warnf]
	r := slog.NewRecord(time.Now(), slog.LevelWarn, fmt.Sprintf(format, args...), pcs[0])
	_ = logger.Handler().Handle(context.Background(), r)
}
