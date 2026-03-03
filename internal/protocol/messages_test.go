package protocol

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestServerMsgJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		msg  ServerMsg
	}{
		{
			name: "matched message",
			msg: ServerMsg{
				Type:   TypeMatched,
				RoomID: "room-123",
				Stranger: &StrangerProfile{
					Username:  "alice",
					AvatarURL: "https://example.com/avatar.png",
				},
			},
		},
		{
			name: "chat message",
			msg: ServerMsg{
				Type:      TypeMessage,
				From:      "alice",
				Text:      "hello world",
				Timestamp: "2024-01-01T00:00:00Z",
			},
		},
		{
			name: "stranger left",
			msg: ServerMsg{
				Type: TypeStrangerLeft,
			},
		},
		{
			name: "waiting",
			msg: ServerMsg{
				Type: TypeWaiting,
			},
		},
		{
			name: "error message",
			msg: ServerMsg{
				Type:    TypeError,
				Message: "something went wrong",
			},
		},
		{
			name: "typing",
			msg: ServerMsg{
				Type:  TypeTyping,
				State: "typing",
			},
		},
		{
			name: "connection closed",
			msg: ServerMsg{
				Type: TypeConnectionClosed,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.msg)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var got ServerMsg
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if !reflect.DeepEqual(got, tt.msg) {
				t.Errorf("roundtrip mismatch:\n  got:  %+v\n  want: %+v", got, tt.msg)
			}
		})
	}
}

func TestServerMsgJSONFieldNames(t *testing.T) {
	// Verify that JSON tags match the expected wire format (matching TypeScript types)
	msg := ServerMsg{
		Type:   TypeMatched,
		RoomID: "r1",
		Stranger: &StrangerProfile{
			Username:  "bob",
			AvatarURL: "https://example.com/a.png",
		},
		From:      "bob",
		Text:      "hi",
		Timestamp: "ts",
		Message:   "err",
		State:     "typing",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}

	expectedFields := []string{"type", "room_id", "stranger", "from", "text", "timestamp", "message", "state"}
	for _, field := range expectedFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("expected JSON field %q not found", field)
		}
	}

	// Verify stranger sub-object field names
	strangerRaw, ok := raw["stranger"].(map[string]interface{})
	if !ok {
		t.Fatal("stranger field is not an object")
	}
	for _, field := range []string{"username", "avatar_url"} {
		if _, ok := strangerRaw[field]; !ok {
			t.Errorf("expected stranger JSON field %q not found", field)
		}
	}
}

func TestClientMessageTypes(t *testing.T) {
	// MessageOut
	msgOut := MessageOut{Type: TypeMessage, Text: "hello"}
	data, err := json.Marshal(msgOut)
	if err != nil {
		t.Fatalf("Marshal MessageOut failed: %v", err)
	}
	var rawMsg map[string]interface{}
	json.Unmarshal(data, &rawMsg)
	if rawMsg["type"] != TypeMessage {
		t.Errorf("MessageOut type = %v, want %v", rawMsg["type"], TypeMessage)
	}
	if rawMsg["text"] != "hello" {
		t.Errorf("MessageOut text = %v, want %v", rawMsg["text"], "hello")
	}

	// SkipOut
	skipOut := SkipOut{Type: TypeSkip}
	data, _ = json.Marshal(skipOut)
	json.Unmarshal(data, &rawMsg)
	if rawMsg["type"] != TypeSkip {
		t.Errorf("SkipOut type = %v, want %v", rawMsg["type"], TypeSkip)
	}

	// TypingOut
	typingOut := TypingOut{Type: TypeTyping, State: "typing"}
	data, _ = json.Marshal(typingOut)
	json.Unmarshal(data, &rawMsg)
	if rawMsg["type"] != TypeTyping {
		t.Errorf("TypingOut type = %v, want %v", rawMsg["type"], TypeTyping)
	}
	if rawMsg["state"] != "typing" {
		t.Errorf("TypingOut state = %v, want %v", rawMsg["state"], "typing")
	}
}

func TestProtocolConstants(t *testing.T) {
	constants := map[string]string{
		"TypeMatched":          "matched",
		"TypeMessage":          "message",
		"TypeStrangerLeft":     "stranger_left",
		"TypeWaiting":          "waiting",
		"TypeError":            "error",
		"TypeTyping":           "typing",
		"TypeSkip":             "skip",
		"TypeConnectionClosed": "connection_closed",
	}

	actual := map[string]string{
		"TypeMatched":          TypeMatched,
		"TypeMessage":          TypeMessage,
		"TypeStrangerLeft":     TypeStrangerLeft,
		"TypeWaiting":          TypeWaiting,
		"TypeError":            TypeError,
		"TypeTyping":           TypeTyping,
		"TypeSkip":             TypeSkip,
		"TypeConnectionClosed": TypeConnectionClosed,
	}

	for name, expected := range constants {
		if actual[name] != expected {
			t.Errorf("%s = %q, want %q", name, actual[name], expected)
		}
	}
}
