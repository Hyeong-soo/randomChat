package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"net/http"

	"github.com/Hyeong-soo/randomChat/internal/protocol"
	"nhooyr.io/websocket"
)

type Client struct {
	conn      *websocket.Conn
	serverURL string
	token     string
	roomID    string
	msgCh     chan protocol.ServerMsg
	mu        sync.Mutex
	closed    bool
}

func (c *Client) Connect(ctx context.Context, serverURL, token, roomID string) error {
	c.serverURL = serverURL
	c.token = token
	c.roomID = roomID
	c.msgCh = make(chan protocol.ServerMsg, 64)
	c.closed = false

	wsURL := fmt.Sprintf("%s/ws", toWSURL(serverURL))
	if roomID != "" {
		wsURL += "?room=" + roomID
	}

	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + token},
		},
	})
	if err != nil {
		return fmt.Errorf("websocket connect failed: %w", err)
	}
	c.conn = conn
	return nil
}

func (c *Client) Send(msg interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.conn == nil {
		return fmt.Errorf("connection closed")
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.conn.Write(ctx, websocket.MessageText, data)
}

func (c *Client) Listen(ctx context.Context) <-chan protocol.ServerMsg {
	go func() {
		defer close(c.msgCh)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			_, data, err := c.conn.Read(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				// Suppress error on clean close (we initiated it or server did after matched/skip)
				c.mu.Lock()
				wasClosed := c.closed
				c.mu.Unlock()
				if wasClosed {
					return
				}
				c.msgCh <- protocol.ServerMsg{Type: protocol.TypeConnectionClosed}
				return
			}
			var msg protocol.ServerMsg
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}
			c.msgCh <- msg
		}
	}()
	return c.msgCh
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.conn == nil {
		return
	}
	c.closed = true
	c.conn.Close(websocket.StatusNormalClosure, "bye")
}

func (c *Client) Reconnect(ctx context.Context) error {
	c.Close()

	delays := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
	var lastErr error
	for _, delay := range delays {
		time.Sleep(delay)
		if err := c.Connect(ctx, c.serverURL, c.token, c.roomID); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return fmt.Errorf("reconnect failed after 3 attempts: %w", lastErr)
}

func toWSURL(httpURL string) string {
	if len(httpURL) >= 8 && httpURL[:8] == "https://" {
		return "wss://" + httpURL[8:]
	}
	if len(httpURL) >= 7 && httpURL[:7] == "http://" {
		return "ws://" + httpURL[7:]
	}
	return "ws://" + httpURL
}
