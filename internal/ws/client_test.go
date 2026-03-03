package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Hyeong-soo/randomChat/internal/protocol"
	"nhooyr.io/websocket"
)

// newTestWSServer creates a mock WebSocket server that accepts connections
// and optionally sends messages from the provided channel.
func newTestWSServer(t *testing.T, handler func(conn *websocket.Conn)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			t.Logf("accept error: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")
		if handler != nil {
			handler(conn)
		}
	}))
	return srv
}

func TestConnectAndClose(t *testing.T) {
	srv := newTestWSServer(t, func(conn *websocket.Conn) {
		// Keep connection open until client closes
		ctx := context.Background()
		for {
			_, _, err := conn.Read(ctx)
			if err != nil {
				return
			}
		}
	})
	defer srv.Close()

	client := &Client{}
	ctx := context.Background()
	err := client.Connect(ctx, srv.URL, "test-token", "")
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Verify connected state
	client.mu.Lock()
	if client.closed {
		t.Error("client.closed should be false after connect")
	}
	if client.conn == nil {
		t.Error("client.conn should not be nil after connect")
	}
	client.mu.Unlock()

	client.Close()

	client.mu.Lock()
	if !client.closed {
		t.Error("client.closed should be true after Close()")
	}
	client.mu.Unlock()
}

func TestConnectWithRoomID(t *testing.T) {
	var receivedQuery string
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		receivedAuth = r.Header.Get("Authorization")
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")
		ctx := context.Background()
		conn.Read(ctx) // block until close
	}))
	defer srv.Close()

	client := &Client{}
	ctx := context.Background()
	err := client.Connect(ctx, srv.URL, "tok", "room-42")
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	if !strings.Contains(receivedQuery, "room=room-42") {
		t.Errorf("query = %q, expected to contain room=room-42", receivedQuery)
	}
	// Token should be in Authorization header, not in URL
	if receivedAuth != "Bearer tok" {
		t.Errorf("auth = %q, expected 'Bearer tok'", receivedAuth)
	}
	if strings.Contains(receivedQuery, "token=") {
		t.Errorf("token should not be in URL query params, got: %q", receivedQuery)
	}
}

func TestSend(t *testing.T) {
	received := make(chan []byte, 1)
	srv := newTestWSServer(t, func(conn *websocket.Conn) {
		ctx := context.Background()
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		received <- data
	})
	defer srv.Close()

	client := &Client{}
	ctx := context.Background()
	if err := client.Connect(ctx, srv.URL, "tok", ""); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	msg := protocol.MessageOut{Type: protocol.TypeMessage, Text: "hello"}
	if err := client.Send(msg); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	select {
	case data := <-received:
		var got protocol.MessageOut
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if got.Type != protocol.TypeMessage {
			t.Errorf("type = %q, want %q", got.Type, protocol.TypeMessage)
		}
		if got.Text != "hello" {
			t.Errorf("text = %q, want %q", got.Text, "hello")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestSendWhenClosed(t *testing.T) {
	srv := newTestWSServer(t, func(conn *websocket.Conn) {
		ctx := context.Background()
		conn.Read(ctx)
	})
	defer srv.Close()

	client := &Client{}
	ctx := context.Background()
	if err := client.Connect(ctx, srv.URL, "tok", ""); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	client.Close()

	err := client.Send(protocol.MessageOut{Type: protocol.TypeMessage, Text: "hi"})
	if err == nil {
		t.Error("Send on closed client should return error")
	}
}

func TestListen(t *testing.T) {
	srv := newTestWSServer(t, func(conn *websocket.Conn) {
		ctx := context.Background()
		// Send a matched message
		msg := protocol.ServerMsg{
			Type:   protocol.TypeMatched,
			RoomID: "room-1",
			Stranger: &protocol.StrangerProfile{
				Username:  "bob",
				AvatarURL: "https://example.com/bob.png",
			},
		}
		data, _ := json.Marshal(msg)
		conn.Write(ctx, websocket.MessageText, data)

		// Send a chat message
		chatMsg := protocol.ServerMsg{
			Type: protocol.TypeMessage,
			Text: "hello from server",
		}
		data, _ = json.Marshal(chatMsg)
		conn.Write(ctx, websocket.MessageText, data)

		// Close to end test
		time.Sleep(100 * time.Millisecond)
		conn.Close(websocket.StatusNormalClosure, "done")
	})
	defer srv.Close()

	client := &Client{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx, srv.URL, "tok", ""); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	ch := client.Listen(ctx)

	var msgs []protocol.ServerMsg
	for msg := range ch {
		msgs = append(msgs, msg)
	}

	// Should have received matched + message + connection_closed
	if len(msgs) < 2 {
		t.Fatalf("got %d messages, want at least 2", len(msgs))
	}

	if msgs[0].Type != protocol.TypeMatched {
		t.Errorf("msg[0].Type = %q, want %q", msgs[0].Type, protocol.TypeMatched)
	}
	if msgs[0].RoomID != "room-1" {
		t.Errorf("msg[0].RoomID = %q, want %q", msgs[0].RoomID, "room-1")
	}
	if msgs[1].Type != protocol.TypeMessage {
		t.Errorf("msg[1].Type = %q, want %q", msgs[1].Type, protocol.TypeMessage)
	}
	if msgs[1].Text != "hello from server" {
		t.Errorf("msg[1].Text = %q, want %q", msgs[1].Text, "hello from server")
	}
}

func TestListenContextCancel(t *testing.T) {
	srv := newTestWSServer(t, func(conn *websocket.Conn) {
		// Keep connection open
		ctx := context.Background()
		conn.Read(ctx)
	})
	defer srv.Close()

	client := &Client{}
	ctx, cancel := context.WithCancel(context.Background())

	if err := client.Connect(ctx, srv.URL, "tok", ""); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	ch := client.Listen(ctx)

	// Cancel the context
	cancel()

	// Channel should close without blocking
	select {
	case _, ok := <-ch:
		if ok {
			// Got a message, drain remaining
			for range ch {
			}
		}
		// Channel closed — expected
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Listen to stop after context cancel")
	}
}

func TestConnectionClosedOnServerDrop(t *testing.T) {
	srv := newTestWSServer(t, func(conn *websocket.Conn) {
		// Close immediately with abnormal closure
		conn.Close(websocket.StatusGoingAway, "server shutting down")
	})
	defer srv.Close()

	client := &Client{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx, srv.URL, "tok", ""); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	ch := client.Listen(ctx)

	var msgs []protocol.ServerMsg
	for msg := range ch {
		msgs = append(msgs, msg)
	}

	// Should get a connection_closed message since client didn't initiate close
	found := false
	for _, msg := range msgs {
		if msg.Type == protocol.TypeConnectionClosed {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected connection_closed message, got: %+v", msgs)
	}
}

func TestCloseIdempotent(t *testing.T) {
	srv := newTestWSServer(t, func(conn *websocket.Conn) {
		ctx := context.Background()
		conn.Read(ctx)
	})
	defer srv.Close()

	client := &Client{}
	ctx := context.Background()
	if err := client.Connect(ctx, srv.URL, "tok", ""); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Close multiple times should not panic
	client.Close()
	client.Close()
	client.Close()
}

func TestCloseWithoutConnect(t *testing.T) {
	client := &Client{}
	// Should not panic
	client.Close()
}

func TestToWSURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://example.com", "wss://example.com"},
		{"http://localhost:8787", "ws://localhost:8787"},
		{"localhost:8787", "ws://localhost:8787"},
	}

	for _, tt := range tests {
		got := toWSURL(tt.input)
		if got != tt.want {
			t.Errorf("toWSURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestConnectFailure(t *testing.T) {
	client := &Client{}
	ctx := context.Background()
	// Connect to a non-existent server
	err := client.Connect(ctx, "http://127.0.0.1:1", "tok", "")
	if err == nil {
		t.Error("Connect to invalid address should return error")
		client.Close()
	}
}

func TestReconnect(t *testing.T) {
	connectCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connectCount++
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")
		ctx := context.Background()
		conn.Read(ctx)
	}))
	defer srv.Close()

	client := &Client{}
	ctx := context.Background()

	// Initial connect
	if err := client.Connect(ctx, srv.URL, "tok", ""); err != nil {
		t.Fatalf("initial Connect failed: %v", err)
	}

	// Reconnect should close and reconnect
	if err := client.Reconnect(ctx); err != nil {
		t.Fatalf("Reconnect failed: %v", err)
	}
	defer client.Close()

	// Should have connected at least twice (initial + reconnect, with possible retries)
	if connectCount < 2 {
		t.Errorf("connectCount = %d, want >= 2", connectCount)
	}
}

func TestReconnectAllFail(t *testing.T) {
	client := &Client{}
	// Set server URL to something that will always fail
	client.serverURL = "http://127.0.0.1:1"
	client.token = "tok"
	client.roomID = ""

	ctx := context.Background()
	err := client.Reconnect(ctx)
	if err == nil {
		t.Error("Reconnect to unreachable server should return error")
	}
	if !strings.Contains(err.Error(), "reconnect failed after 3 attempts") {
		t.Errorf("error = %q, want to contain 'reconnect failed after 3 attempts'", err.Error())
	}
}
