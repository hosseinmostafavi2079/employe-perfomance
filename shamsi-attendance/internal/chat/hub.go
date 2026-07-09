package chat

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"shamsi_attendance/internal/database"
)

// AudioState وضعیت لحظه‌ای پخش آهنگ در یک گروه را نگه می‌دارد
type AudioState struct {
	MediaURL      string
	FileName      string
	IsPlaying     bool
	Position      float64
	LastUpdatedAt time.Time
}

type Hub struct {
	Rooms           map[int]map[*Client]bool
	RoomAudioStates map[int]*AudioState
	Broadcast       chan Message
	Register        chan *Client
	Unregister      chan *Client
}

func NewHub() *Hub {
	return &Hub{
		Broadcast:       make(chan Message),
		Register:        make(chan *Client),
		Unregister:      make(chan *Client),
		Rooms:           make(map[int]map[*Client]bool),
		RoomAudioStates: make(map[int]*AudioState),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			if h.Rooms[client.RoomID] == nil {
				h.Rooms[client.RoomID] = make(map[*Client]bool)
			}
			h.Rooms[client.RoomID][client] = true

			// ارسال وضعیت فعلی آهنگ به کاربری که تازه وارد شده یا صفحه را رفرش کرده
			if state, ok := h.RoomAudioStates[client.RoomID]; ok && state.MediaURL != "" {
				pos := state.Position
				if state.IsPlaying {
					pos += time.Since(state.LastUpdatedAt).Seconds()
				}

				status := "pause"
				if state.IsPlaying {
					status = "play"
				}

				initData := fmt.Sprintf(`{"url":"%s", "status":"%s", "pos":%f, "name":"%s"}`, state.MediaURL, status, pos, state.FileName)

				initMsg := Message{
					RoomID:      client.RoomID,
					SenderID:    "system",
					MessageType: "audio_init",
					MessageText: initData,
				}

				select {
				case client.Send <- initMsg:
				default:
				}
			}

		case client := <-h.Unregister:
			if clients, ok := h.Rooms[client.RoomID]; ok {
				if _, ok := clients[client]; ok {
					delete(clients, client)
					close(client.Send)
					if len(clients) == 0 {
						delete(h.Rooms, client.RoomID)
					}
				}
			}

		case message := <-h.Broadcast:
			// بروزرسانی وضعیت مرکزی آهنگ در حافظه سرور
			if message.MessageType == MessageTypeAudioNew {
				h.RoomAudioStates[message.RoomID] = &AudioState{
					MediaURL:      message.MediaURL,
					FileName:      message.MessageText,
					IsPlaying:     true,
					Position:      0,
					LastUpdatedAt: time.Now(),
				}
			} else if message.MessageType == MessageTypeAudioSync {
				state, exists := h.RoomAudioStates[message.RoomID]
				if !exists {
					state = &AudioState{}
					h.RoomAudioStates[message.RoomID] = state
				}
				state.IsPlaying = (message.MessageText == "play")
				pos, _ := strconv.ParseFloat(message.MediaURL, 64)
				state.Position = pos
				state.LastUpdatedAt = time.Now()
			}

			// 🚀 ذخیره پیام در دیتابیس (ارسال با & برای دریافت ID جهت امکان حذف)
			if message.MessageType == MessageTypeText || message.MessageType == MessageTypeSystem || message.MessageType == MessageTypeFile {
				h.saveMessageToDB(&message) 
			}

			if clients, ok := h.Rooms[message.RoomID]; ok {
				for client := range clients {
					select {
					case client.Send <- message:
					default:
						close(client.Send)
						delete(clients, client)
					}
				}
			}
		}
	}
}

// saveMessageToDB دریافت شناسه بعد از ذخیره (دریافت به صورت پوینتر)
func (h *Hub) saveMessageToDB(msg *Message) {
	query := `INSERT INTO chat_messages (room_id, sender_id, sender_name, sender_avatar, message_text, message_type, media_url) 
			  VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`
	
	err := database.DB.QueryRow(context.Background(), query, msg.RoomID, msg.SenderID, msg.SenderName, msg.SenderAvatar, msg.MessageText, msg.MessageType, msg.MediaURL).Scan(&msg.ID)
	
	if err != nil {
		log.Printf("[CHAT DB ERROR] Failed to save message: %v\n", err)
	}
}