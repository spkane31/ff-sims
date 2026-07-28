package helpers

import (
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// EnvValue is the set of types GetEnv knows how to parse from an environment
// variable's string value.
type EnvValue interface {
	int | int64 | float64 | bool | string | time.Duration
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
	case *time.Duration:
		d, err := time.ParseDuration(raw)
		if err != nil {
			return def
		}
		*p = d
	}
	return out
}

var durationType = reflect.TypeFor[time.Duration]()

// LoadEnvStruct populates fields of cfg from their env tags. A tag's first
// value is the environment variable name; its optional default and min values
// use comma-separated key=value pairs, for example:
//
//	PoolSize int           `env:"WORKER_POOL_SIZE,default=10,min=1"`
//	Timeout  time.Duration `env:"WORKER_TIMEOUT,default=5s,min=1s"`
//
// Empty or invalid environment values use the default, matching GetEnv's
// behavior. cfg must point to a struct and tagged fields must be supported by
// EnvValue.
func LoadEnvStruct(cfg any) {
	value := reflect.ValueOf(cfg)
	if value.Kind() != reflect.Pointer || value.IsNil() || value.Elem().Kind() != reflect.Struct {
		panic("LoadEnvStruct requires a pointer to a struct")
	}

	value = value.Elem()
	for i := range value.NumField() {
		field := value.Type().Field(i)
		tag := field.Tag.Get("env")
		if tag == "" {
			continue
		}

		name, defaultValue, minimum := parseEnvTag(tag)
		target := value.Field(i)
		setEnvField(target, name, defaultValue)
		if minimum != "" {
			clampEnvField(target, minimum)
		}
	}
}

func parseEnvTag(tag string) (name, defaultValue, minimum string) {
	parts := strings.Split(tag, ",")
	name = parts[0]
	for _, part := range parts[1:] {
		key, value, found := strings.Cut(part, "=")
		if !found {
			continue
		}
		switch key {
		case "default":
			defaultValue = value
		case "min":
			minimum = value
		}
	}
	return name, defaultValue, minimum
}

func setEnvField(field reflect.Value, name, defaultValue string) {
	raw := os.Getenv(name)
	if raw != "" && parseEnvValue(field, raw) {
		return
	}
	if defaultValue != "" && !parseEnvValue(field, defaultValue) {
		panic("LoadEnvStruct has an invalid default for " + name)
	}
}

func clampEnvField(field reflect.Value, minimum string) {
	minimumValue := reflect.New(field.Type()).Elem()
	if !parseEnvValue(minimumValue, minimum) {
		panic("LoadEnvStruct has an invalid minimum for " + minimum)
	}
	if belowEnvMinimum(field, minimumValue) {
		field.Set(minimumValue)
	}
}

func belowEnvMinimum(field, minimum reflect.Value) bool {
	switch field.Kind() {
	case reflect.Int, reflect.Int64:
		return field.Int() < minimum.Int()
	case reflect.Float64:
		return field.Float() < minimum.Float()
	default:
		panic("LoadEnvStruct minimum is not supported for " + field.Type().String())
	}
}

func parseEnvValue(field reflect.Value, raw string) bool {
	if field.Type() == durationType {
		value, err := time.ParseDuration(raw)
		if err != nil {
			return false
		}
		field.SetInt(int64(value))
		return true
	}

	switch field.Kind() {
	case reflect.Int:
		value, err := strconv.Atoi(raw)
		if err != nil {
			return false
		}
		field.SetInt(int64(value))
	case reflect.Int64:
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return false
		}
		field.SetInt(value)
	case reflect.Float64:
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return false
		}
		field.SetFloat(value)
	case reflect.Bool:
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return false
		}
		field.SetBool(value)
	case reflect.String:
		field.SetString(raw)
	default:
		panic("LoadEnvStruct does not support " + field.Type().String())
	}
	return true
}
