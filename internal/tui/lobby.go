package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Hyeong-soo/randomChat/internal/protocol"
	"github.com/Hyeong-soo/randomChat/internal/ws"
)

type LobbyModel struct {
	elapsed     time.Duration
	frame       int
	wsClient    *ws.Client
	serverURL   string
	token       string
	err         string
	connected   bool
	program     *tea.Program
	cancel      context.CancelFunc
	retrying    bool
	retryCount  int
	maxRetries  int
	retryFailed bool
	width       int
	height      int
}

func NewLobbyModel(serverURL, token string) LobbyModel {
	return LobbyModel{
		serverURL:  serverURL,
		token:      token,
		wsClient:   &ws.Client{},
		maxRetries: 3,
		width:      80,
		height:     24,
	}
}

func (m *LobbyModel) SetProgram(p *tea.Program) {
	m.program = p
}

func (m LobbyModel) Init() tea.Cmd {
	return tea.Batch(m.connectToMatchmaker(), m.tick())
}

type retryMsg struct {
	attempt int
}

func (m LobbyModel) Update(msg tea.Msg) (LobbyModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			if m.cancel != nil {
				m.cancel()
			}
			if m.wsClient != nil {
				m.wsClient.Close()
			}
			return m, tea.Quit
		case "enter":
			if m.retryFailed {
				m.retryFailed = false
				m.retryCount = 0
				m.err = ""
				return m, m.connectToMatchmaker()
			}
		}
	case TickMsg:
		m.elapsed += time.Second
		m.frame++
		return m, m.tick()
	case wsConnectedMsg:
		m.connected = true
		m.retrying = false
		m.retryCount = 0
		return m, nil
	case retryMsg:
		m.retrying = true
		m.retryCount = msg.attempt
		return m, m.connectToMatchmaker()
	case MatchedMsg:
		if m.cancel != nil {
			m.cancel()
		}
		if m.wsClient != nil {
			m.wsClient.Close()
		}
		return m, nil
	case ErrorMsg:
		// If we haven't exhausted retries, schedule a retry
		if m.retryCount < m.maxRetries {
			attempt := m.retryCount + 1
			delays := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
			delay := delays[m.retryCount]
			m.retrying = true
			m.retryCount = attempt
			m.err = ""
			return m, tea.Tick(delay, func(t time.Time) tea.Msg {
				return retryMsg{attempt: attempt}
			})
		}
		// All retries exhausted
		m.retrying = false
		m.retryFailed = true
		m.err = msg.Err.Error()
		return m, nil
	}
	return m, nil
}

func (m LobbyModel) View() string {
	var sb strings.Builder

	sb.WriteString(Title.Render("RandomChat") + "\n\n")

	if m.retryFailed {
		sb.WriteString(ErrorText.Render("Connection failed: "+m.err) + "\n\n")
		sb.WriteString(Subtle.Render("Press Enter to retry, q to quit."))
		return sb.String()
	}

	if m.retrying {
		sb.WriteString(fmt.Sprintf("Retrying... (attempt %d/%d)\n\n", m.retryCount, m.maxRetries))
	} else {
		sb.WriteString("Looking for someone to chat with...\n\n")
	}

	// Animated progress
	spinChars := []string{"|", "/", "-", "\\"}
	spin := spinChars[m.frame%len(spinChars)]
	bar := renderProgressBar(m.frame)

	sb.WriteString(fmt.Sprintf("  %s %s\n\n", ProgressBar.Render(spin), bar))
	sb.WriteString(Subtle.Render(fmt.Sprintf("  Waiting for %ds", int(m.elapsed.Seconds()))))

	if m.elapsed >= 2*time.Minute {
		sb.WriteString("\n\n" + Subtle.Render("Still looking... Press q to quit or keep waiting."))
	}

	return sb.String()
}

func renderProgressBar(frame int) string {
	width := 20
	pos := frame % (width * 2)
	if pos >= width {
		pos = width*2 - pos - 1
	}

	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < width; i++ {
		if i == pos {
			sb.WriteString("=")
		} else {
			sb.WriteString(" ")
		}
	}
	sb.WriteString("]")
	return ProgressBar.Render(sb.String())
}

type wsConnectedMsg struct{}

func (m LobbyModel) connectToMatchmaker() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		m.cancel = cancel
		if err := m.wsClient.Connect(ctx, m.serverURL, m.token, ""); err != nil {
			return ErrorMsg{Err: fmt.Errorf("failed to connect: %w", err)}
		}

		// Start listening in background and forward messages to program
		ch := m.wsClient.Listen(ctx)
		go func() {
			for msg := range ch {
				if m.program == nil {
					continue
				}
				switch msg.Type {
				case protocol.TypeMatched:
					m.program.Send(MatchedMsg{
						RoomID:   msg.RoomID,
						Stranger: msg.Stranger,
					})
					return // Stop listening after match
				case protocol.TypeWaiting:
					// Already showing waiting UI
				case protocol.TypeConnectionClosed:
					// Clean close after matched — ignore
					return
				case protocol.TypeError:
					m.program.Send(ErrorMsg{Err: fmt.Errorf("%s", msg.Message)})
				}
			}
		}()

		return wsConnectedMsg{}
	}
}

func (m LobbyModel) tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return TickMsg{}
	})
}
