package chat

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512000 // محدودیت نیم مگابایتی برای امنیت (جلوگیری از حملات سایبری)
)

// Client واسط بین سوکت کلاینت (فرانت اند) و هاب سرور است
type Client struct {
	Hub    *Hub
	Conn   *websocket.Conn
	Send   chan Message
	RoomID int
	UserID string
}

// ReadPump پیام ها را از مرورگر می خواند و به هاب می دهد
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.Unregister <- c
		c.Conn.Close()
	}()
	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error { c.Conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	for {
		_, messageBytes, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WS error: %v", err)
			}
			break
		}

		var msg Message
		err = json.Unmarshal(messageBytes, &msg)
		if err == nil {
			// ایمن سازی: اورراید کردن آیدی فرستنده و گروه از روی سشن سرور (جلوگیری از هک)
			msg.RoomID = c.RoomID
			msg.SenderID = c.UserID
			c.Hub.Broadcast <- msg
		}
	}
}

// WritePump پیام ها را از هاب می گیرد و به مرورگر می فرستد
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// هاب کانال را بسته است
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}

			msgJSON, _ := json.Marshal(message)
			w.Write(msgJSON)

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}