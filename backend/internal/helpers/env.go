package helpers

import (
	"os"
	"strconv"
)

// EnvValue is the set of types GetEnv knows how to parse from an environment
// variable's string value.
type EnvValue interface {
	int | int64 | float64 | bool | string
}

// GetEnv returns the environment variable key parsed as T, or def when the
// variable is unset, empty, or fails to parse.
func GetEnv[T EnvValue](key string, def T) T {
	raw := os.Getenv(key)
	if raw == "" {
		return def
	}
	var out T
	switch p := any(&out).(type) {
	case *int:
		n, err := strconv.Atoi(raw)
		if err != nil {
			return def
		}
		*p = n
	case *int64:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return def
		}
		*p = n
	case *float64:
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return def
		}
		*p = f
	case *bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return def
		}
		*p = b
	case *string:
		*p = raw
	}
	return out
}
