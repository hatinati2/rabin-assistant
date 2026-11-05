package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func newTestCredentialStore(t *testing.T) *RabinCredentialStore {
	t.Helper()
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "rabin.key")
	if err := os.WriteFile(keyPath, bytes.Repeat([]byte{0x5a}, 32), 0600); err != nil {
		t.Fatal(err)
	}
	return NewRabinCredentialStore(dir, keyPath)
}

func TestRabinCredentialStoreEncryptsAndRoundTrips(t *testing.T) {
	store := newTestCredentialStore(t)
	if err := store.Save("test-user", "very-secret-password"); err != nil {
		t.Fatal(err)
	}

	username, saved, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !saved || username != "test-user" {
		t.Fatalf("unexpected status: username=%q saved=%v", username, saved)
	}

	data, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("test-user")) || bytes.Contains(data, []byte("very-secret-password")) {
		t.Fatal("credential file contains plaintext credentials")
	}
	info, err := os.Stat(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("credential file permissions = %04o, want 0600", info.Mode().Perm())
	}
}

func TestRabinCredentialStoreKeepsPasswordWhenBlank(t *testing.T) {
	store := newTestCredentialStore(t)
	if err := store.Save("first-user", "saved-password"); err != nil {
		t.Fatal(err)
	}
	if err := store.Save("second-user", ""); err != nil {
		t.Fatal(err)
	}

	store.mu.Lock()
	username, password, err := store.loadLocked()
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if username != "second-user" || password != "saved-password" {
		t.Fatalf("credentials were not preserved: username=%q password-preserved=%v", username, password == "saved-password")
	}
}

func TestRabinCredentialStoreDelete(t *testing.T) {
	store := newTestCredentialStore(t)
	if err := store.Save("test-user", "saved-password"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}
	username, saved, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if saved || username != "" {
		t.Fatalf("credentials still present after delete: username=%q saved=%v", username, saved)
	}
}

func TestRabinCredentialStoreRejectsTamperedCiphertext(t *testing.T) {
	store := newTestCredentialStore(t)
	if err := store.Save("test-user", "saved-password"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-3] ^= 1
	if err := os.WriteFile(store.path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Status(); err == nil {
		t.Fatal("tampered credential file was accepted")
	}
}
