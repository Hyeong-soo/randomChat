package config

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestConfig(t *testing.T) *Config {
	t.Helper()
	dir := t.TempDir()
	return &Config{
		ConfigDir:      dir,
		SessionFile:    filepath.Join(dir, "session.json"),
		AvatarCacheDir: filepath.Join(dir, "avatars"),
		ServerURL:      "http://localhost:8787",
	}
}

func TestSaveAndLoadSession(t *testing.T) {
	cfg := newTestConfig(t)

	err := cfg.SaveSession("tok-abc", "alice", "https://example.com/a.png")
	if err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	sess := cfg.LoadSession()
	if sess == nil {
		t.Fatal("LoadSession returned nil after SaveSession")
	}
	if sess.Token != "tok-abc" {
		t.Errorf("Token = %q, want %q", sess.Token, "tok-abc")
	}
	if sess.Username != "alice" {
		t.Errorf("Username = %q, want %q", sess.Username, "alice")
	}
	if sess.AvatarURL != "https://example.com/a.png" {
		t.Errorf("AvatarURL = %q, want %q", sess.AvatarURL, "https://example.com/a.png")
	}
}

func TestClearSession(t *testing.T) {
	cfg := newTestConfig(t)

	// Save then clear
	if err := cfg.SaveSession("tok-abc", "alice", ""); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}
	if err := cfg.ClearSession(); err != nil {
		t.Fatalf("ClearSession failed: %v", err)
	}

	sess := cfg.LoadSession()
	if sess != nil {
		t.Errorf("LoadSession after ClearSession returned %+v, want nil", sess)
	}
}

func TestLoadSessionEmptyToken(t *testing.T) {
	cfg := newTestConfig(t)

	// Save session with empty token
	if err := cfg.SaveSession("", "alice", ""); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	sess := cfg.LoadSession()
	if sess != nil {
		t.Errorf("LoadSession with empty token returned %+v, want nil", sess)
	}
}

func TestLoadSessionNoFile(t *testing.T) {
	cfg := newTestConfig(t)

	sess := cfg.LoadSession()
	if sess != nil {
		t.Errorf("LoadSession with no file returned %+v, want nil", sess)
	}
}

func TestLoadSessionCorruptedJSON(t *testing.T) {
	cfg := newTestConfig(t)

	// Write invalid JSON
	if err := os.MkdirAll(cfg.ConfigDir, 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(cfg.SessionFile, []byte("not json"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	sess := cfg.LoadSession()
	if sess != nil {
		t.Errorf("LoadSession with corrupted JSON returned %+v, want nil", sess)
	}
}

func TestLoadReturnsError(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load() returned nil config")
	}
	if cfg.ConfigDir == "" {
		t.Error("ConfigDir is empty")
	}
	if cfg.SessionFile == "" {
		t.Error("SessionFile is empty")
	}
	if cfg.AvatarCacheDir == "" {
		t.Error("AvatarCacheDir is empty")
	}
	if cfg.ServerURL == "" {
		t.Error("ServerURL is empty")
	}
}

func TestLoadServerURLFromEnv(t *testing.T) {
	original := os.Getenv("RANDOMCHAT_SERVER")
	defer os.Setenv("RANDOMCHAT_SERVER", original)

	os.Setenv("RANDOMCHAT_SERVER", "https://custom.example.com")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.ServerURL != "https://custom.example.com" {
		t.Errorf("ServerURL = %q, want %q", cfg.ServerURL, "https://custom.example.com")
	}
}
