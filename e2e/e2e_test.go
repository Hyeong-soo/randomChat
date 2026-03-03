package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Hyeong-soo/randomChat/internal/protocol"
	"github.com/Hyeong-soo/randomChat/internal/ws"
)

// recvWithTimeout reads from the channel with a timeout.
func recvWithTimeout(t *testing.T, ch <-chan protocol.ServerMsg, timeout time.Duration) protocol.ServerMsg {
	t.Helper()
	select {
	case msg, ok := <-ch:
		if !ok {
			t.Fatal("channel closed unexpectedly")
		}
		return msg
	case <-time.After(timeout):
		t.Fatal("timeout waiting for message")
		return protocol.ServerMsg{} // unreachable
	}
}

// expectNoMessage verifies no message arrives within the given duration.
func expectNoMessage(t *testing.T, ch <-chan protocol.ServerMsg, wait time.Duration) {
	t.Helper()
	select {
	case msg, ok := <-ch:
		if ok {
			t.Fatalf("expected no message, got type=%q", msg.Type)
		}
	case <-time.After(wait):
		// Good, no message received
	}
}

// connectMatchmaker connects a ws.Client to the mock matchmaker, starts listening,
// and returns the client and its message channel.
func connectMatchmaker(t *testing.T, serverURL, token string) (*ws.Client, <-chan protocol.ServerMsg) {
	t.Helper()
	client := &ws.Client{}
	ctx := context.Background()
	if err := client.Connect(ctx, serverURL, token, ""); err != nil {
		t.Fatalf("Connect(%s) failed: %v", token, err)
	}
	ch := client.Listen(ctx)
	return client, ch
}

// connectChatRoom connects a ws.Client to the mock chat room, starts listening,
// and returns the client and its message channel.
func connectChatRoom(t *testing.T, serverURL, token, roomID string) (*ws.Client, <-chan protocol.ServerMsg) {
	t.Helper()
	client := &ws.Client{}
	ctx := context.Background()
	if err := client.Connect(ctx, serverURL, token, roomID); err != nil {
		t.Fatalf("Connect(%s, room=%s) failed: %v", token, roomID, err)
	}
	ch := client.Listen(ctx)
	return client, ch
}

// matchTwoUsers connects two tokens, waits for "waiting" then "matched", and returns
// the room_id that both clients received.
func matchTwoUsers(t *testing.T, serverURL, tokenA, tokenB string) string {
	t.Helper()

	clientA, chA := connectMatchmaker(t, serverURL, tokenA)
	defer clientA.Close()

	// Wait for A to get "waiting" before connecting B, so the server has time to enqueue.
	msgA := recvWithTimeout(t, chA, 2*time.Second)
	if msgA.Type != protocol.TypeWaiting {
		t.Fatalf("A: expected waiting, got %q", msgA.Type)
	}

	clientB, chB := connectMatchmaker(t, serverURL, tokenB)
	defer clientB.Close()

	msgB := recvWithTimeout(t, chB, 2*time.Second)
	if msgB.Type != protocol.TypeWaiting {
		t.Fatalf("B: expected waiting, got %q", msgB.Type)
	}

	// Both should receive "matched"
	matchedA := recvWithTimeout(t, chA, 2*time.Second)
	if matchedA.Type != protocol.TypeMatched {
		t.Fatalf("A: expected matched, got %q", matchedA.Type)
	}

	matchedB := recvWithTimeout(t, chB, 2*time.Second)
	if matchedB.Type != protocol.TypeMatched {
		t.Fatalf("B: expected matched, got %q", matchedB.Type)
	}

	if matchedA.RoomID != matchedB.RoomID {
		t.Fatalf("room_id mismatch: A=%q B=%q", matchedA.RoomID, matchedB.RoomID)
	}
	if matchedA.RoomID == "" {
		t.Fatal("room_id is empty")
	}

	return matchedA.RoomID
}

func TestMatchingFlow(t *testing.T) {
	ms := newMockServer()
	defer ms.Close()

	clientA, chA := connectMatchmaker(t, ms.URL(), "userA")
	defer clientA.Close()

	// A should receive "waiting"
	msg := recvWithTimeout(t, chA, 2*time.Second)
	if msg.Type != protocol.TypeWaiting {
		t.Fatalf("expected waiting, got %q", msg.Type)
	}

	clientB, chB := connectMatchmaker(t, ms.URL(), "userB")
	defer clientB.Close()

	// B should receive "waiting"
	msg = recvWithTimeout(t, chB, 2*time.Second)
	if msg.Type != protocol.TypeWaiting {
		t.Fatalf("expected waiting, got %q", msg.Type)
	}

	// Both should receive "matched" with matching room_id
	matchedA := recvWithTimeout(t, chA, 2*time.Second)
	if matchedA.Type != protocol.TypeMatched {
		t.Fatalf("A: expected matched, got %q", matchedA.Type)
	}
	if matchedA.RoomID == "" {
		t.Fatal("A: room_id is empty")
	}
	if matchedA.Stranger == nil {
		t.Fatal("A: stranger is nil")
	}
	if matchedA.Stranger.Username != "bob" {
		t.Errorf("A: stranger.username = %q, want %q", matchedA.Stranger.Username, "bob")
	}

	matchedB := recvWithTimeout(t, chB, 2*time.Second)
	if matchedB.Type != protocol.TypeMatched {
		t.Fatalf("B: expected matched, got %q", matchedB.Type)
	}
	if matchedB.RoomID != matchedA.RoomID {
		t.Fatalf("room_id mismatch: A=%q B=%q", matchedA.RoomID, matchedB.RoomID)
	}
	if matchedB.Stranger == nil {
		t.Fatal("B: stranger is nil")
	}
	if matchedB.Stranger.Username != "alice" {
		t.Errorf("B: stranger.username = %q, want %q", matchedB.Stranger.Username, "alice")
	}
}

func TestMessageRelay(t *testing.T) {
	ms := newMockServer()
	defer ms.Close()

	roomID := matchTwoUsers(t, ms.URL(), "userA", "userB")

	// Connect both to the chat room
	clientA, chA := connectChatRoom(t, ms.URL(), "userA", roomID)
	defer clientA.Close()

	clientB, chB := connectChatRoom(t, ms.URL(), "userB", roomID)
	defer clientB.Close()

	// A sends a message -> B receives it
	err := clientA.Send(protocol.MessageOut{Type: protocol.TypeMessage, Text: "hello"})
	if err != nil {
		t.Fatalf("A.Send failed: %v", err)
	}

	msg := recvWithTimeout(t, chB, 2*time.Second)
	if msg.Type != protocol.TypeMessage {
		t.Fatalf("B: expected message, got %q", msg.Type)
	}
	if msg.Text != "hello" {
		t.Errorf("B: text = %q, want %q", msg.Text, "hello")
	}
	if msg.From != "stranger" {
		t.Errorf("B: from = %q, want %q", msg.From, "stranger")
	}

	// B sends a message -> A receives it
	err = clientB.Send(protocol.MessageOut{Type: protocol.TypeMessage, Text: "hi back"})
	if err != nil {
		t.Fatalf("B.Send failed: %v", err)
	}

	msg = recvWithTimeout(t, chA, 2*time.Second)
	if msg.Type != protocol.TypeMessage {
		t.Fatalf("A: expected message, got %q", msg.Type)
	}
	if msg.Text != "hi back" {
		t.Errorf("A: text = %q, want %q", msg.Text, "hi back")
	}
}

func TestSkipFlow(t *testing.T) {
	ms := newMockServer()
	defer ms.Close()

	roomID := matchTwoUsers(t, ms.URL(), "userA", "userB")

	clientA, _ := connectChatRoom(t, ms.URL(), "userA", roomID)
	defer clientA.Close()

	clientB, chB := connectChatRoom(t, ms.URL(), "userB", roomID)
	defer clientB.Close()

	// A sends skip
	err := clientA.Send(protocol.SkipOut{Type: protocol.TypeSkip})
	if err != nil {
		t.Fatalf("A.Send(skip) failed: %v", err)
	}

	// B should receive stranger_left
	msg := recvWithTimeout(t, chB, 2*time.Second)
	if msg.Type != protocol.TypeStrangerLeft {
		t.Fatalf("B: expected stranger_left, got %q", msg.Type)
	}
}

func TestDisconnect(t *testing.T) {
	ms := newMockServer()
	defer ms.Close()

	roomID := matchTwoUsers(t, ms.URL(), "userA", "userB")

	clientA, _ := connectChatRoom(t, ms.URL(), "userA", roomID)

	clientB, chB := connectChatRoom(t, ms.URL(), "userB", roomID)
	defer clientB.Close()

	// A disconnects
	clientA.Close()

	// B should receive stranger_left
	msg := recvWithTimeout(t, chB, 2*time.Second)
	if msg.Type != protocol.TypeStrangerLeft {
		t.Fatalf("B: expected stranger_left, got %q", msg.Type)
	}
}

func TestSelfMatchPrevention(t *testing.T) {
	ms := newMockServer()
	defer ms.Close()

	// Connect two sockets with the same token (same user)
	clientA1, chA1 := connectMatchmaker(t, ms.URL(), "userA")
	defer clientA1.Close()

	msg := recvWithTimeout(t, chA1, 2*time.Second)
	if msg.Type != protocol.TypeWaiting {
		t.Fatalf("A1: expected waiting, got %q", msg.Type)
	}

	clientA2, chA2 := connectMatchmaker(t, ms.URL(), "userA")
	defer clientA2.Close()

	msg = recvWithTimeout(t, chA2, 2*time.Second)
	if msg.Type != protocol.TypeWaiting {
		t.Fatalf("A2: expected waiting, got %q", msg.Type)
	}

	// Neither should receive "matched" within a short timeout
	expectNoMessage(t, chA1, 1*time.Second)
	expectNoMessage(t, chA2, 500*time.Millisecond)
}

func TestTypingIndicator(t *testing.T) {
	ms := newMockServer()
	defer ms.Close()

	roomID := matchTwoUsers(t, ms.URL(), "userA", "userB")

	clientA, chA := connectChatRoom(t, ms.URL(), "userA", roomID)
	defer clientA.Close()

	clientB, chB := connectChatRoom(t, ms.URL(), "userB", roomID)
	defer clientB.Close()

	// A sends typing "typing" -> B receives it
	err := clientA.Send(protocol.TypingOut{Type: protocol.TypeTyping, State: "typing"})
	if err != nil {
		t.Fatalf("A.Send(typing) failed: %v", err)
	}

	msg := recvWithTimeout(t, chB, 2*time.Second)
	if msg.Type != protocol.TypeTyping {
		t.Fatalf("B: expected typing, got %q", msg.Type)
	}
	if msg.State != "typing" {
		t.Errorf("B: state = %q, want %q", msg.State, "typing")
	}

	// A sends typing "stopped" -> B receives it
	err = clientA.Send(protocol.TypingOut{Type: protocol.TypeTyping, State: "stopped"})
	if err != nil {
		t.Fatalf("A.Send(stopped) failed: %v", err)
	}

	msg = recvWithTimeout(t, chB, 2*time.Second)
	if msg.Type != protocol.TypeTyping {
		t.Fatalf("B: expected typing, got %q", msg.Type)
	}
	if msg.State != "stopped" {
		t.Errorf("B: state = %q, want %q", msg.State, "stopped")
	}

	// B sends typing "typing" -> A receives it
	err = clientB.Send(protocol.TypingOut{Type: protocol.TypeTyping, State: "typing"})
	if err != nil {
		t.Fatalf("B.Send(typing) failed: %v", err)
	}

	msg = recvWithTimeout(t, chA, 2*time.Second)
	if msg.Type != protocol.TypeTyping {
		t.Fatalf("A: expected typing, got %q", msg.Type)
	}
	if msg.State != "typing" {
		t.Errorf("A: state = %q, want %q", msg.State, "typing")
	}
}

func TestMessageLengthLimit(t *testing.T) {
	ms := newMockServer()
	defer ms.Close()

	roomID := matchTwoUsers(t, ms.URL(), "userA", "userB")

	clientA, chA := connectChatRoom(t, ms.URL(), "userA", roomID)
	defer clientA.Close()

	// Connect B so peer exists (but we don't need its channel for this test)
	clientB, _ := connectChatRoom(t, ms.URL(), "userB", roomID)
	defer clientB.Close()

	// Send a message that's too long (2001 chars)
	longText := strings.Repeat("x", 2001)
	err := clientA.Send(protocol.MessageOut{Type: protocol.TypeMessage, Text: longText})
	if err != nil {
		t.Fatalf("A.Send(long) failed: %v", err)
	}

	// A should receive an error
	msg := recvWithTimeout(t, chA, 2*time.Second)
	if msg.Type != protocol.TypeError {
		t.Fatalf("A: expected error, got %q", msg.Type)
	}
	if !strings.Contains(msg.Message, "too long") {
		t.Errorf("A: error message = %q, want to contain 'too long'", msg.Message)
	}
}
