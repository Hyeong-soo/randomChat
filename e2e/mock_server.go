package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/henry/randomchat/internal/protocol"
	"nhooyr.io/websocket"
)

// mockServer simulates the Worker's Matchmaker and ChatRoom behavior for E2E tests.
type mockServer struct {
	srv *httptest.Server

	mu       sync.Mutex
	queue    []*matchEntry        // Matchmaker queue
	rooms    map[string]*chatRoom // room_id -> chatRoom
	profiles map[string]profile   // token -> profile
}

type matchEntry struct {
	conn    *websocket.Conn
	token   string
	profile profile
}

type profile struct {
	GithubID  string
	Username  string
	AvatarURL string
}

type chatRoom struct {
	mu    sync.Mutex
	conns []*websocket.Conn
}

const maxMessageLen = 2000

func newMockServer() *mockServer {
	ms := &mockServer{
		rooms: make(map[string]*chatRoom),
		profiles: map[string]profile{
			"userA": {GithubID: "1", Username: "alice", AvatarURL: "https://example.com/alice.png"},
			"userB": {GithubID: "2", Username: "bob", AvatarURL: "https://example.com/bob.png"},
			"userC": {GithubID: "3", Username: "carol", AvatarURL: "https://example.com/carol.png"},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", ms.handleWS)
	mux.HandleFunc("/auth/me", ms.handleAuthMe)

	ms.srv = httptest.NewServer(mux)
	return ms
}

func (ms *mockServer) URL() string {
	return ms.srv.URL
}

func (ms *mockServer) Close() {
	ms.srv.Close()
}

// handleAuthMe returns a profile JSON for the given Bearer token.
func (ms *mockServer) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	token := strings.TrimPrefix(auth, "Bearer ")
	if token == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ms.mu.Lock()
	p, ok := ms.profiles[token]
	ms.mu.Unlock()
	if !ok {
		http.Error(w, "Unknown token", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"github_id":  p.GithubID,
		"username":   p.Username,
		"avatar_url": p.AvatarURL,
	})
}

// handleWS routes WebSocket connections to Matchmaker or ChatRoom based on query params.
func (ms *mockServer) handleWS(w http.ResponseWriter, r *http.Request) {
	// Read token from Authorization header (preferred) or query param (legacy)
	var token string
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		token = strings.TrimPrefix(auth, "Bearer ")
	}
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	roomID := r.URL.Query().Get("room")

	if token == "" {
		http.Error(w, "Missing token", http.StatusUnauthorized)
		return
	}

	ms.mu.Lock()
	p, ok := ms.profiles[token]
	ms.mu.Unlock()
	if !ok {
		http.Error(w, "Unknown token", http.StatusUnauthorized)
		return
	}

	if roomID != "" {
		ms.handleChatRoom(w, r, token, p, roomID)
	} else {
		ms.handleMatchmaker(w, r, token, p)
	}
}

// handleMatchmaker simulates the Matchmaker DO.
func (ms *mockServer) handleMatchmaker(w http.ResponseWriter, r *http.Request, token string, p profile) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}

	// Send waiting
	sendJSON(conn, protocol.ServerMsg{Type: protocol.TypeWaiting})

	entry := &matchEntry{conn: conn, token: token, profile: p}

	ms.mu.Lock()
	ms.queue = append(ms.queue, entry)
	ms.tryMatch()
	ms.mu.Unlock()

	// Keep connection open until server closes it (after match) or client disconnects
	ctx := r.Context()
	for {
		_, _, err := conn.Read(ctx)
		if err != nil {
			ms.removeFromQueue(entry)
			return
		}
	}
}

// tryMatch attempts to pair users in the queue. Must be called with ms.mu held.
func (ms *mockServer) tryMatch() {
	for i := 0; i < len(ms.queue); i++ {
		for j := i + 1; j < len(ms.queue); j++ {
			a := ms.queue[i]
			b := ms.queue[j]

			// Self-match prevention: same github_id = same user
			if a.profile.GithubID == b.profile.GithubID {
				continue
			}

			// Remove both from queue
			ms.queue = append(ms.queue[:j], ms.queue[j+1:]...)
			ms.queue = append(ms.queue[:i], ms.queue[i+1:]...)

			roomID := fmt.Sprintf("room-%d", time.Now().UnixNano())

			// Create room
			ms.rooms[roomID] = &chatRoom{}

			strangerA := protocol.StrangerProfile{Username: a.profile.Username, AvatarURL: a.profile.AvatarURL}
			strangerB := protocol.StrangerProfile{Username: b.profile.Username, AvatarURL: b.profile.AvatarURL}

			// Send matched to A (stranger = B's profile)
			sendJSON(a.conn, protocol.ServerMsg{
				Type:     protocol.TypeMatched,
				RoomID:   roomID,
				Stranger: &strangerB,
			})
			// Send matched to B (stranger = A's profile)
			sendJSON(b.conn, protocol.ServerMsg{
				Type:     protocol.TypeMatched,
				RoomID:   roomID,
				Stranger: &strangerA,
			})

			// Close both WS connections (server-side close after match)
			a.conn.Close(websocket.StatusNormalClosure, "matched")
			b.conn.Close(websocket.StatusNormalClosure, "matched")

			// Recurse for remaining queue
			ms.tryMatch()
			return
		}
	}
}

func (ms *mockServer) removeFromQueue(entry *matchEntry) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	for i, e := range ms.queue {
		if e == entry {
			ms.queue = append(ms.queue[:i], ms.queue[i+1:]...)
			return
		}
	}
}

// handleChatRoom simulates the ChatRoom DO.
func (ms *mockServer) handleChatRoom(w http.ResponseWriter, r *http.Request, _ string, _ profile, roomID string) {
	ms.mu.Lock()
	room, ok := ms.rooms[roomID]
	if !ok {
		room = &chatRoom{}
		ms.rooms[roomID] = room
	}
	ms.mu.Unlock()

	room.mu.Lock()
	if len(room.conns) >= 2 {
		room.mu.Unlock()
		http.Error(w, "Room is full", http.StatusForbidden)
		return
	}
	room.mu.Unlock()

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}

	room.mu.Lock()
	room.conns = append(room.conns, conn)
	room.mu.Unlock()

	defer func() {
		// On disconnect, notify peer
		room.mu.Lock()
		peer := room.getPeerLocked(conn)
		room.removeLocked(conn)
		room.mu.Unlock()

		if peer != nil {
			sendJSON(peer, protocol.ServerMsg{Type: protocol.TypeStrangerLeft})
			peer.Close(websocket.StatusNormalClosure, "peer_disconnected")
		}
	}()

	ctx := r.Context()
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}

		var raw struct {
			Type  string `json:"type"`
			Text  string `json:"text"`
			State string `json:"state"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			sendJSON(conn, protocol.ServerMsg{Type: protocol.TypeError, Message: "Invalid JSON"})
			continue
		}

		room.mu.Lock()
		peer := room.getPeerLocked(conn)
		room.mu.Unlock()

		switch raw.Type {
		case protocol.TypeMessage:
			if len(raw.Text) > maxMessageLen {
				sendJSON(conn, protocol.ServerMsg{
					Type:    protocol.TypeError,
					Message: "Message too long (max 2000 characters)",
				})
				continue
			}
			if peer != nil {
				sendJSON(peer, protocol.ServerMsg{
					Type:      protocol.TypeMessage,
					From:      "stranger",
					Text:      raw.Text,
					Timestamp: time.Now().UTC().Format(time.RFC3339),
				})
			}

		case protocol.TypeSkip:
			if peer != nil {
				sendJSON(peer, protocol.ServerMsg{Type: protocol.TypeStrangerLeft})
				peer.Close(websocket.StatusNormalClosure, "skipped")
			}
			conn.Close(websocket.StatusNormalClosure, "skipped")
			return

		case protocol.TypeTyping:
			if raw.State != "typing" && raw.State != "stopped" {
				continue
			}
			if peer != nil {
				sendJSON(peer, protocol.ServerMsg{
					Type:  protocol.TypeTyping,
					State: raw.State,
				})
			}

		default:
			sendJSON(conn, protocol.ServerMsg{Type: protocol.TypeError, Message: "Unknown message type"})
		}
	}
}

func (r *chatRoom) getPeerLocked(conn *websocket.Conn) *websocket.Conn {
	for _, c := range r.conns {
		if c != conn {
			return c
		}
	}
	return nil
}

func (r *chatRoom) removeLocked(conn *websocket.Conn) {
	for i, c := range r.conns {
		if c == conn {
			r.conns = append(r.conns[:i], r.conns[i+1:]...)
			return
		}
	}
}

func sendJSON(conn *websocket.Conn, msg interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn.Write(ctx, websocket.MessageText, data)
}
