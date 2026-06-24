package sleeper_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"workers/internal/sleeper"
)

func TestGetUser_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/user/testuser" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(sleeper.User{
			UserID: "123", Username: "testuser", DisplayName: "Test User",
		})
	}))
	defer srv.Close()

	c := sleeper.NewWithBaseURL(srv.URL)
	u, err := c.GetUser(context.Background(), "testuser")
	if err != nil {
		t.Fatalf("GetUser error: %v", err)
	}
	if u.UserID != "123" {
		t.Errorf("got UserID %q, want %q", u.UserID, "123")
	}
}

func TestGetUser_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := sleeper.NewWithBaseURL(srv.URL)
	_, err := c.GetUser(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	var nfe *sleeper.NotFoundError
	if !errors.As(err, &nfe) {
		t.Errorf("expected NotFoundError, got %T: %v", err, err)
	}
}

func TestGetUser_RateLimitRetries(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		json.NewEncoder(w).Encode(sleeper.User{UserID: "456"})
	}))
	defer srv.Close()

	c := sleeper.NewWithBaseURL(srv.URL)
	u, err := c.GetUser(context.Background(), "someone")
	if err != nil {
		t.Fatalf("GetUser error after retries: %v", err)
	}
	if u.UserID != "456" {
		t.Errorf("got UserID %q, want %q", u.UserID, "456")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}
