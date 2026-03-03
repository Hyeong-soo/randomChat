package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Session struct {
	Token     string `json:"token"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
}

type Config struct {
	ConfigDir      string
	SessionFile    string
	AvatarCacheDir string
	ServerURL      string
}

func Load() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	configDir := os.Getenv("RANDOMCHAT_CONFIG")
	if configDir == "" {
		configDir = filepath.Join(home, ".config", "randomchat")
	}

	serverURL := os.Getenv("RANDOMCHAT_SERVER")
	if serverURL == "" {
		serverURL = "https://randomchat.hyeongsoo.workers.dev"
	}

	return &Config{
		ConfigDir:      configDir,
		SessionFile:    filepath.Join(configDir, "session.json"),
		AvatarCacheDir: filepath.Join(configDir, "avatars"),
		ServerURL:      serverURL,
	}, nil
}

func (c *Config) SaveSession(token, username, avatarURL string) error {
	if err := os.MkdirAll(c.ConfigDir, 0700); err != nil {
		return err
	}
	sess := Session{
		Token:     token,
		Username:  username,
		AvatarURL: avatarURL,
	}
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.SessionFile, data, 0600)
}

func (c *Config) LoadSession() *Session {
	data, err := os.ReadFile(c.SessionFile)
	if err != nil {
		return nil
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil
	}
	if sess.Token == "" {
		return nil
	}
	return &sess
}

func (c *Config) ClearSession() error {
	return os.Remove(c.SessionFile)
}
