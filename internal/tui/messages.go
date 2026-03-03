package tui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/henry/randomchat/internal/protocol"
)

// Custom Msg types for Bubbletea

type AuthDoneMsg struct {
	Token     string
	Username  string
	AvatarURL string
}

type MatchedMsg struct {
	RoomID   string
	Stranger *protocol.StrangerProfile
}

type IncomingMsg struct {
	Text      string
	Timestamp string
}

type StrangerLeftMsg struct{}

type StrangerTypingMsg struct {
	Typing bool
}

type ErrorMsg struct {
	Err error
}

type TickMsg struct{}

type typingTimeoutMsg struct {
	at time.Time
}

type avatarLoadedMsg struct {
	ascii string
}

type returnToLobbyMsg struct{}

// fetchProfile calls /auth/me to get the user's profile
func fetchProfile(serverURL, token string) (username, avatarURL string, err error) {
	req, err := http.NewRequest("GET", serverURL+"/auth/me", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var profile struct {
		Username  string `json:"username"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return "", "", err
	}
	return profile.Username, profile.AvatarURL, nil
}
