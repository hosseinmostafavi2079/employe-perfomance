package chat

import "time"

const (
	RoomTypeGroup  = "group"
	RoomTypeDirect = "direct"
)

const (
	MessageTypeText      = "text"
	MessageTypeFile      = "file" // نوع جدید برای آپلود فایل و عکس
	MessageTypeAudioSync = "audio_sync"
	MessageTypeSystem    = "system"
	MessageTypeAudioNew  = "audio_new"
	MessageTypeDelete    = "delete_msg" // سیگنال حذف زنده پیام
)

type ChatRoom struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	RoomType  string    `json:"room_type"`
	Avatar    string    `json:"avatar"`
	CreatedAt time.Time `json:"created_at"`
}

type ChatRoomMember struct {
	RoomID   int       `json:"room_id"`
	UserID   string    `json:"user_id"`
	JoinedAt time.Time `json:"joined_at"`
}

type ChatMessage struct {
	ID           int       `json:"id"`
	RoomID       int       `json:"room_id"`
	SenderID     string    `json:"sender_id"`
	SenderName   string    `json:"sender_name"`
	SenderAvatar string    `json:"sender_avatar"`
	MessageText  string    `json:"message_text"`
	MessageType  string    `json:"message_type"`
	MediaURL     string    `json:"media_url"`
	CreatedAt    time.Time `json:"created_at"`
}

type ChatPlaylist struct {
	ID        int       `json:"id"`
	RoomID    int       `json:"room_id"`
	AddedBy   string    `json:"added_by"`
	FilePath  string    `json:"file_path"`
	FileName  string    `json:"file_name"`
	CreatedAt time.Time `json:"created_at"`
}

type Message struct {
	ID           int    `json:"id"` // 🚀 برای پیدا کردن و حذف پیام
	RoomID       int    `json:"room_id"`
	SenderID     string `json:"sender_id"`
	SenderName   string `json:"sender_name"`
	SenderAvatar string `json:"sender_avatar"`
	MessageText  string `json:"message_text"`
	MessageType  string `json:"message_type"`
	MediaURL     string `json:"media_url"`
}