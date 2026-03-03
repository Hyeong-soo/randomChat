package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/henry/randomchat/internal/auth"
	"github.com/henry/randomchat/internal/config"
)

type loginStatus int

const (
	loginWaiting loginStatus = iota
	loginAuthenticating
	loginDone
	loginError
)

type LoginModel struct {
	status    loginStatus
	username  string
	errMsg    string
	serverURL string
	config    *config.Config
}

func NewLoginModel(cfg *config.Config) LoginModel {
	return LoginModel{
		status:    loginWaiting,
		serverURL: cfg.ServerURL,
		config:    cfg,
	}
}

func (m LoginModel) Init() tea.Cmd {
	return nil
}

func (m LoginModel) Update(msg tea.Msg) (LoginModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if m.status == loginWaiting || m.status == loginError {
				m.status = loginAuthenticating
				m.errMsg = ""
				return m, m.startAuth()
			}
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case AuthDoneMsg:
		m.status = loginDone
		m.username = msg.Username
		return m, nil
	case ErrorMsg:
		m.status = loginError
		m.errMsg = msg.Err.Error()
		return m, nil
	}
	return m, nil
}

func (m LoginModel) View() string {
	switch m.status {
	case loginWaiting:
		return Title.Render("RandomChat") + "\n\n" +
			"GitHub login required.\n" +
			Subtle.Render("Press Enter to open browser for authentication...") + "\n\n" +
			Subtle.Render("Press q to quit.")
	case loginAuthenticating:
		return Title.Render("RandomChat") + "\n\n" +
			"Opening browser...\n" +
			"Waiting for authentication..." + "\n\n" +
			Subtle.Render("This will timeout after 2 minutes.")
	case loginDone:
		return Title.Render("RandomChat") + "\n\n" +
			fmt.Sprintf("Logged in! (github: %s)", m.username)
	case loginError:
		return Title.Render("RandomChat") + "\n\n" +
			ErrorText.Render("Authentication failed: "+m.errMsg) + "\n\n" +
			Subtle.Render("Press Enter to retry, q to quit.")
	}
	return ""
}

func (m LoginModel) startAuth() tea.Cmd {
	return func() tea.Msg {
		token, err := auth.StartOAuth(m.serverURL)
		if err != nil {
			return ErrorMsg{Err: err}
		}
		// Fetch user profile
		username, avatarURL, err := fetchProfile(m.serverURL, token)
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("failed to fetch profile: %w", err)}
		}
		// Save session
		if err := m.config.SaveSession(token, username, avatarURL); err != nil {
			return ErrorMsg{Err: fmt.Errorf("failed to save session: %w", err)}
		}
		return AuthDoneMsg{
			Token:     token,
			Username:  username,
			AvatarURL: avatarURL,
		}
	}
}
