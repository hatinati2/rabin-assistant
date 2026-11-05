package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRabinLoginClientSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/authentication/login" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["userName"] != "unit-user" || body["password"] != "unit-pass" {
			t.Fatal("unexpected login payload")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"accessToken":"unit-token"}}`))
	}))
	defer server.Close()

	client := RabinLoginClient{
		HTTP: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				clone := req.Clone(req.Context())
				clone.URL.Scheme = "http"
				clone.URL.Host = server.Listener.Addr().String()
				return server.Client().Do(clone)
			}),
		},
	}
	token, err := client.Login(context.Background(), "unit-user", "unit-pass")
	if err != nil {
		t.Fatal(err)
	}
	if token != "unit-token" {
		t.Fatalf("unexpected token %q", token)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestRabinCredentialStoreSessionIsEncrypted(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key")
	if err := os.WriteFile(keyPath, []byte("01234567890123456789012345678901"), 0600); err != nil {
		t.Fatal(err)
	}
	store := NewRabinCredentialStore(dir, keyPath)

	const token = "sensitive-session-token"
	if err := store.SaveAccessToken(token); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "rabin-session.enc.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), token) {
		t.Fatal("session token was stored in plaintext")
	}
	got, updatedAt, err := store.LoadAccessToken()
	if err != nil {
		t.Fatal(err)
	}
	if got != token || updatedAt.IsZero() {
		t.Fatal("session token did not round-trip")
	}
	at, saved, err := store.SessionStatus()
	if err != nil || !saved || at.IsZero() {
		t.Fatal("session status not reported")
	}
	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}
	got, _, err = store.LoadAccessToken()
	if err != nil || got != "" {
		t.Fatal("session was not removed with credentials")
	}
}
