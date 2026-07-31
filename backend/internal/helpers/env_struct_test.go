package helpers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadEnvStruct_ReadsDefaultsOverridesAndMinimums(t *testing.T) {
	t.Setenv("ENV_STRUCT_INT", "42")
	t.Setenv("ENV_STRUCT_DURATION", "500ms")
	t.Setenv("ENV_STRUCT_CLAMPED", "0")
	t.Setenv("ENV_STRUCT_INVALID", "not-an-integer")

	cfg := struct {
		Integer  int           `env:"ENV_STRUCT_INT,default=10"`
		Duration time.Duration `env:"ENV_STRUCT_DURATION,default=5s,min=1s"`
		Clamped  int           `env:"ENV_STRUCT_CLAMPED,default=6,min=1"`
		Default  int           `env:"ENV_STRUCT_DEFAULT,default=25"`
		Invalid  int           `env:"ENV_STRUCT_INVALID,default=9"`
	}{}

	LoadEnvStruct(&cfg)

	require.Equal(t, 42, cfg.Integer)
	require.Equal(t, 1*time.Second, cfg.Duration)
	require.Equal(t, 1, cfg.Clamped)
	require.Equal(t, 25, cfg.Default)
	require.Equal(t, 9, cfg.Invalid)
}
