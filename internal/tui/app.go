package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/henry/randomchat/internal/config"
)

type Screen int

const (
	ScreenLogin Screen = iota
	ScreenLobby
	ScreenChat
)

type App struct {
	screen     Screen
	loginModel LoginModel
	lobbyModel LobbyModel
	chatModel  ChatModel
	config     *config.Config
	token      string
	username   string
	avatarURL  string
	program    *tea.Program
}

func NewApp(cfg *config.Config) App {
	app := App{
		config: cfg,
	}

	// Check for existing session
	sess := cfg.LoadSession()
	if sess != nil {
		app.screen = ScreenLobby
		app.token = sess.Token
		app.username = sess.Username
		app.avatarURL = sess.AvatarURL
		app.lobbyModel = NewLobbyModel(cfg.ServerURL, sess.Token)
	} else {
		app.screen = ScreenLogin
		app.loginModel = NewLoginModel(cfg)
	}

	return app
}

func (a *App) SetProgram(p *tea.Program) {
	a.program = p
	a.lobbyModel.SetProgram(p)
	a.chatModel.SetProgram(p)
}

func (a App) Init() tea.Cmd {
	switch a.screen {
	case ScreenLogin:
		return a.loginModel.Init()
	case ScreenLobby:
		return a.lobbyModel.Init()
	default:
		return nil
	}
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Forward to chat if active
		if a.screen == ScreenChat {
			var cmd tea.Cmd
			a.chatModel, cmd = a.chatModel.Update(msg)
			return a, cmd
		}
		return a, nil
	}

	switch a.screen {
	case ScreenLogin:
		return a.updateLogin(msg)
	case ScreenLobby:
		return a.updateLobby(msg)
	case ScreenChat:
		return a.updateChat(msg)
	}
	return a, nil
}

func (a App) View() string {
	switch a.screen {
	case ScreenLogin:
		return a.loginModel.View()
	case ScreenLobby:
		return a.lobbyModel.View()
	case ScreenChat:
		return a.chatModel.View()
	}
	return ""
}

func (a App) updateLogin(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case AuthDoneMsg:
		a.token = msg.Token
		a.username = msg.Username
		a.avatarURL = msg.AvatarURL
		// Transition to lobby
		a.screen = ScreenLobby
		a.lobbyModel = NewLobbyModel(a.config.ServerURL, a.token)
		a.lobbyModel.SetProgram(a.program)
		return a, a.lobbyModel.Init()
	default:
		var cmd tea.Cmd
		a.loginModel, cmd = a.loginModel.Update(msg)
		return a, cmd
	}
}

func (a App) updateLobby(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case MatchedMsg:
		// Transition to chat
		a.screen = ScreenChat
		a.chatModel = NewChatModel(a.config, a.config.ServerURL, a.token, msg.RoomID, msg.Stranger)
		a.chatModel.SetProgram(a.program)
		return a, a.chatModel.Init()
	default:
		var cmd tea.Cmd
		a.lobbyModel, cmd = a.lobbyModel.Update(msg)
		return a, cmd
	}
}

func (a App) updateChat(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case returnToLobbyMsg:
		// Close chat WebSocket before transitioning
		a.chatModel.CloseWS()
		// Return to lobby for new match
		a.screen = ScreenLobby
		a.lobbyModel = NewLobbyModel(a.config.ServerURL, a.token)
		a.lobbyModel.SetProgram(a.program)
		return a, a.lobbyModel.Init()
	default:
		var cmd tea.Cmd
		a.chatModel, cmd = a.chatModel.Update(msg)
		return a, cmd
	}
}
