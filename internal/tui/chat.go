package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/Hyeong-soo/randomChat/internal/config"
	"github.com/Hyeong-soo/randomChat/internal/profile"
	"github.com/Hyeong-soo/randomChat/internal/protocol"
	"github.com/Hyeong-soo/randomChat/internal/ws"
)

type ChatMessage struct {
	From      string
	Text      string
	Timestamp string
	System    bool
}

type ChatModel struct {
	messages     []ChatMessage
	textInput    textinput.Model
	stranger     *protocol.StrangerProfile
	asciiAvatar  string
	wsClient     *ws.Client
	serverURL    string
	token        string
	roomID       string
	cfg          *config.Config
	program      *tea.Program
	width        int
	height       int
	scrollOff    int
	bellNext     bool
	typing       bool
	lastTyping   time.Time
	disconnected bool
	disconnectAt time.Time
	cancel       context.CancelFunc
}

func NewChatModel(cfg *config.Config, serverURL, token, roomID string, stranger *protocol.StrangerProfile) ChatModel {
	ti := textinput.New()
	ti.Placeholder = "Type a message..."
	ti.Focus()
	ti.CharLimit = 2000
	ti.Prompt = "> "

	return ChatModel{
		stranger:  stranger,
		textInput: ti,
		wsClient:  &ws.Client{},
		serverURL: serverURL,
		token:     token,
		roomID:    roomID,
		cfg:       cfg,
		width:     80,
		height:    24,
	}
}

func (m *ChatModel) SetProgram(p *tea.Program) {
	m.program = p
}

func (m *ChatModel) CloseWS() {
	if m.cancel != nil {
		m.cancel()
	}
	if m.wsClient != nil {
		m.wsClient.Close()
	}
}

func (m ChatModel) Init() tea.Cmd {
	return tea.Batch(m.connectToRoom(), m.loadAvatar(), textinput.Blink)
}

func (m ChatModel) Update(msg tea.Msg) (ChatModel, tea.Cmd) {
	// Reset bell flag after it has been rendered
	m.bellNext = false

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textInput.Width = msg.Width - 4
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.CloseWS()
			return m, tea.Quit
		case "esc":
			return m, m.skipStranger()
		case "enter":
			if m.disconnected {
				return m, nil
			}
			text := strings.TrimSpace(m.textInput.Value())
			if text == "" {
				return m, nil
			}
			m.textInput.Reset()
			if strings.HasPrefix(text, "/") {
				return m.handleCommand(text)
			}
			m.messages = append(m.messages, ChatMessage{
				From: "you",
				Text: text,
			})
			return m, tea.Batch(m.sendMessage(text), m.sendTypingStopped())
		case "pgup":
			m.scrollOff += 5
			if m.scrollOff > len(m.messages)-1 {
				m.scrollOff = len(m.messages) - 1
			}
			return m, nil
		case "pgdown":
			m.scrollOff -= 5
			if m.scrollOff < 0 {
				m.scrollOff = 0
			}
			return m, nil
		}

	case IncomingMsg:
		m.messages = append(m.messages, ChatMessage{
			From:      "stranger",
			Text:      msg.Text,
			Timestamp: msg.Timestamp,
		})
		m.scrollOff = 0
		m.bellNext = true
		return m, nil

	case typingTimeoutMsg:
		if !m.lastTyping.IsZero() && time.Since(m.lastTyping) >= 3*time.Second {
			return m, m.sendTypingStopped()
		}
		return m, nil

	case StrangerTypingMsg:
		m.typing = msg.Typing
		return m, nil

	case StrangerLeftMsg:
		if m.disconnected {
			return m, nil
		}
		m.disconnected = true
		m.disconnectAt = time.Now()
		m.textInput.Blur()
		m.messages = append(m.messages, ChatMessage{
			System: true,
			Text:   "Stranger has disconnected.",
		})
		return m, m.returnToLobbyAfterDelay()

	case avatarLoadedMsg:
		m.asciiAvatar = msg.ascii
		return m, nil

	case wsConnectedMsg:
		m.messages = append(m.messages, ChatMessage{
			System: true,
			Text:   fmt.Sprintf("Connected! You're chatting with %s", m.stranger.Username),
		})
		return m, nil

	case returnToLobbyMsg:
		m.CloseWS()
		return m, nil

	case ErrorMsg:
		m.messages = append(m.messages, ChatMessage{
			System: true,
			Text:   "Error: " + msg.Err.Error(),
		})
		return m, nil
	}

	// Forward remaining key events to textinput
	if !m.disconnected {
		prevVal := m.textInput.Value()
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		// Detect typing changes
		if m.textInput.Value() != prevVal {
			return m, tea.Batch(cmd, m.sendTypingIfNeeded())
		}
		return m, cmd
	}
	return m, nil
}

func (m ChatModel) View() string {
	var sb strings.Builder

	// Terminal bell on new incoming message
	if m.bellNext {
		sb.WriteString("\a")
	}

	// Top: profile box (avatar left, info+graph right)
	if m.stranger != nil {
		var right strings.Builder
		right.WriteString(m.stranger.Username)
		if m.stranger.GithubCreatedAt != "" {
			if t, err := time.Parse(time.RFC3339, m.stranger.GithubCreatedAt); err == nil {
				years := time.Since(t).Hours() / 24 / 365
				right.WriteString(Subtle.Render(fmt.Sprintf("  GitHub %dy", int(years))))
			}
		}
		if m.stranger.PublicRepos > 0 {
			right.WriteString(Subtle.Render(fmt.Sprintf("  repos:%d", m.stranger.PublicRepos)))
		}
		if m.stranger.TopLanguages != "" {
			right.WriteString(Subtle.Render(fmt.Sprintf("  [%s]", m.stranger.TopLanguages)))
		}
		if m.stranger.TopRepo != "" {
			star := fmt.Sprintf("  ★ %s", m.stranger.TopRepo)
			if m.stranger.TopRepoStars > 0 {
				star += fmt.Sprintf("(%d)", m.stranger.TopRepoStars)
			}
			right.WriteString(Subtle.Render(star))
		}
		if m.stranger.Bio != "" {
			bio := m.stranger.Bio
			if len([]rune(bio)) > 60 {
				bio = string([]rune(bio)[:57]) + "..."
			}
			right.WriteString("\n" + Subtle.Render(bio))
		}
		if m.stranger.ContributionGraph != "" {
			right.WriteString("\n" + renderContributionGraph(m.stranger.ContributionGraph, m.stranger.Contributions))
		} else if m.stranger.Contributions > 0 {
			right.WriteString(Subtle.Render(fmt.Sprintf("  🌱%d", m.stranger.Contributions)))
		}
		if m.typing {
			right.WriteString(Subtle.Render(" (typing...)"))
		}

		var content string
		if m.asciiAvatar != "" {
			content = lipgloss.JoinHorizontal(lipgloss.Top, m.asciiAvatar, "  ", right.String())
		} else {
			content = right.String()
		}
		sb.WriteString(ProfileBox.Render(content))
		sb.WriteString("\n")
	}

	// Middle: messages
	visibleLines := m.height - 10
	if visibleLines < 5 {
		visibleLines = 5
	}

	var msgLines []string
	for _, msg := range m.messages {
		if msg.System {
			msgLines = append(msgLines, Subtle.Render("  --- "+msg.Text+" ---"))
		} else if msg.From == "you" {
			msgLines = append(msgLines, MessageYou.Render("You: "+msg.Text))
		} else {
			msgLines = append(msgLines, MessageStranger.Render(m.stranger.Username+": "+msg.Text))
		}
	}

	start := len(msgLines) - visibleLines - m.scrollOff
	if start < 0 {
		start = 0
	}
	end := start + visibleLines
	if end > len(msgLines) {
		end = len(msgLines)
	}

	for i := start; i < end; i++ {
		sb.WriteString(msgLines[i] + "\n")
	}
	for i := end - start; i < visibleLines; i++ {
		sb.WriteString("\n")
	}

	// Bottom: input
	bar := StatusBar.Width(m.width)
	if m.disconnected {
		sb.WriteString(bar.Render("Stranger disconnected. Returning to lobby..."))
	} else {
		sb.WriteString(m.textInput.View() + "\n")
		sb.WriteString(bar.Render("/skip  /quit  /help  |  Esc=skip  PgUp/PgDn=scroll"))
	}

	return sb.String()
}

func (m ChatModel) handleCommand(cmd string) (ChatModel, tea.Cmd) {
	switch cmd {
	case "/skip":
		return m, m.skipStranger()
	case "/quit":
		m.CloseWS()
		return m, tea.Quit
	case "/help":
		m.messages = append(m.messages, ChatMessage{
			System: true,
			Text:   "Commands: /skip (find new stranger), /quit (exit), /help (show this)",
		})
		return m, nil
	case "/status":
		m.messages = append(m.messages, ChatMessage{
			System: true,
			Text:   fmt.Sprintf("Chatting with %s | Messages: %d", m.stranger.Username, len(m.messages)),
		})
		return m, nil
	default:
		m.messages = append(m.messages, ChatMessage{
			System: true,
			Text:   "Unknown command. Type /help for available commands.",
		})
		return m, nil
	}
}

func (m ChatModel) sendMessage(text string) tea.Cmd {
	return func() tea.Msg {
		err := m.wsClient.Send(protocol.MessageOut{
			Type: protocol.TypeMessage,
			Text: text,
		})
		if err != nil {
			return ErrorMsg{Err: fmt.Errorf("failed to send: %w", err)}
		}
		return nil
	}
}

func (m *ChatModel) sendTypingIfNeeded() tea.Cmd {
	now := time.Now()
	if now.Sub(m.lastTyping) < 2*time.Second {
		return nil
	}
	m.lastTyping = now
	return tea.Batch(
		func() tea.Msg {
			m.wsClient.Send(protocol.TypingOut{
				Type:  protocol.TypeTyping,
				State: "typing",
			})
			return nil
		},
		m.scheduleTypingTimeout(),
	)
}

func (m *ChatModel) sendTypingStopped() tea.Cmd {
	m.lastTyping = time.Time{}
	return func() tea.Msg {
		m.wsClient.Send(protocol.TypingOut{
			Type:  protocol.TypeTyping,
			State: "stopped",
		})
		return nil
	}
}

func (m *ChatModel) scheduleTypingTimeout() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return typingTimeoutMsg{at: t}
	})
}

func (m ChatModel) skipStranger() tea.Cmd {
	return func() tea.Msg {
		m.wsClient.Send(protocol.SkipOut{Type: protocol.TypeSkip})
		return StrangerLeftMsg{}
	}
}

func (m ChatModel) connectToRoom() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		m.cancel = cancel
		if err := m.wsClient.Connect(ctx, m.serverURL, m.token, m.roomID); err != nil {
			return ErrorMsg{Err: fmt.Errorf("failed to join room: %w", err)}
		}

		ch := m.wsClient.Listen(ctx)
		go func() {
			for msg := range ch {
				if m.program == nil {
					continue
				}
				switch msg.Type {
				case protocol.TypeMessage:
					m.program.Send(IncomingMsg{
						Text:      msg.Text,
						Timestamp: msg.Timestamp,
					})
				case protocol.TypeStrangerLeft:
					m.program.Send(StrangerLeftMsg{})
				case protocol.TypeTyping:
					m.program.Send(StrangerTypingMsg{Typing: msg.State == "typing"})
				case protocol.TypeConnectionClosed:
					// Server closes our WS after peer disconnects.
					// If stranger_left wasn't received yet, treat as disconnect.
					m.program.Send(StrangerLeftMsg{})
					return
				case protocol.TypeError:
					m.program.Send(ErrorMsg{Err: fmt.Errorf("%s", msg.Message)})
				}
			}
		}()

		return wsConnectedMsg{}
	}
}

func (m ChatModel) loadAvatar() tea.Cmd {
	return func() tea.Msg {
		if m.stranger == nil {
			return nil
		}
		ascii, _ := profile.FetchAvatar(m.cfg, m.stranger.AvatarURL, m.stranger.Username)
		return avatarLoadedMsg{ascii: ascii}
	}
}

func (m ChatModel) returnToLobbyAfterDelay() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return returnToLobbyMsg{}
	})
}

func renderContributionGraph(graph string, total int) string {
	greenShades := []string{
		"\x1b[48;2;22;27;34m \x1b[0m",
		"\x1b[48;2;14;68;41m \x1b[0m",
		"\x1b[48;2;0;109;50m \x1b[0m",
		"\x1b[48;2;38;166;65m \x1b[0m",
		"\x1b[48;2;57;211;83m \x1b[0m",
	}

	weeks := len(graph) / 7
	if weeks == 0 {
		return ""
	}

	var sb strings.Builder
	for day := 0; day < 7; day++ {
		for week := 0; week < weeks; week++ {
			idx := week*7 + day
			if idx < len(graph) {
				level := int(graph[idx] - '0')
				if level < 0 || level > 4 {
					level = 0
				}
				sb.WriteString(greenShades[level])
			}
		}
		if day < 6 {
			sb.WriteByte('\n')
		}
	}
	sb.WriteString(Subtle.Render(fmt.Sprintf(" %d contributions", total)))
	return sb.String()
}
