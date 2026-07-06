package helpers

import "testing"

func TestGetEnv_Int(t *testing.T) {
	t.Setenv("X_INT", "42")
	if got := GetEnv("X_INT", 7); got != 42 {
		t.Errorf("got %d, want 42", got)
	}
	t.Setenv("X_INT", "notanumber")
	if got := GetEnv("X_INT", 7); got != 7 {
		t.Errorf("unparseable: got %d, want default 7", got)
	}
	t.Setenv("X_INT", "")
	if got := GetEnv("X_INT", 7); got != 7 {
		t.Errorf("empty: got %d, want default 7", got)
	}
}

func TestGetEnv_Bool(t *testing.T) {
	t.Setenv("X_BOOL", "true")
	if !GetEnv("X_BOOL", false) {
		t.Error("got false, want true")
	}
	t.Setenv("X_BOOL", "nope")
	if GetEnv("X_BOOL", false) {
		t.Error("unparseable: got true, want default false")
	}
}

func TestGetEnv_String(t *testing.T) {
	t.Setenv("X_STR", "hello")
	if got := GetEnv("X_STR", "def"); got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
	t.Setenv("X_STR", "")
	if got := GetEnv("X_STR", "def"); got != "def" {
		t.Errorf("empty: got %q, want default", got)
	}
}

func TestGetEnv_Float(t *testing.T) {
	t.Setenv("X_F", "2.5")
	if got := GetEnv("X_F", 1.0); got != 2.5 {
		t.Errorf("got %v, want 2.5", got)
	}
}

func TestGetEnv_Int64(t *testing.T) {
	t.Setenv("X_I64", "9000000000")
	if got := GetEnv("X_I64", int64(1)); got != 9000000000 {
		t.Errorf("got %v, want 9000000000", got)
	}
}
