package chat

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
	"shamsi_attendance/internal/database"
)

var AppHub *Hub

// InitHub یک بار هنگام اجرای سرور صدا زده می شود
func InitHub() {
	AppHub = NewHub()
	go AppHub.Run()
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// چون سامانه لوکال و آفلاین است، بررسی Origin را روی شبکه داخلی باز می گذاریم
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// ServeWS درخواست HTTP را به WebSocket ارتقا می دهد
func ServeWS(w http.ResponseWriter, r *http.Request, userID string) {
	// دریافت آیدی گروه از URL
	roomIDStr := r.URL.Query().Get("room_id")
	roomID, err := strconv.Atoi(roomIDStr)
	if err != nil || roomID <= 0 {
		http.Error(w, "Invalid Room ID", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[CHAT WS ERROR] Upgrade failed: %v", err)
		return
	}

	client := &Client{
		Hub:    AppHub,
		Conn:   conn,
		Send:   make(chan Message, 256),
		RoomID: roomID,
		UserID: userID,
	}

	client.Hub.Register <- client

	// شروع گوروتین های خواندن و نوشتن موازی (بدون بلاک کردن برنامه)
	go client.WritePump()
	go client.ReadPump()
}

func GetChatHistory(w http.ResponseWriter, r *http.Request) {
	roomIDStr := r.URL.Query().Get("room_id")
	roomID, err := strconv.Atoi(roomIDStr)
	if err != nil {
		http.Error(w, "Invalid Room ID", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	query := `
		SELECT id, room_id, sender_id, COALESCE(sender_name, 'کاربر'), COALESCE(sender_avatar, ''), message_text, message_type, media_url, created_at 
		FROM chat_messages 
		WHERE room_id = $1 
		ORDER BY created_at ASC 
		LIMIT 500;
	`
	rows, err := database.DB.Query(ctx, query, roomID)
	if err != nil {
		http.Error(w, "DB Error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var messages []ChatMessage
	for rows.Next() {
		var msg ChatMessage
		err := rows.Scan(&msg.ID, &msg.RoomID, &msg.SenderID, &msg.SenderName, &msg.SenderAvatar, &msg.MessageText, &msg.MessageType, &msg.MediaURL, &msg.CreatedAt)
		if err == nil {
			messages = append(messages, msg)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

// GetServerPlaylist خواندن خودکار فایل‌ها و دسته‌بندی پلی‌لیست‌ها از پوشه‌ها
func GetServerPlaylist(w http.ResponseWriter, r *http.Request) {
	// تغییر مسیر به playlists (حرف s اضافه شده است)
	baseDir := "static/uploads/chat_media/playlists"
	os.MkdirAll(baseDir, os.ModePerm)

	type PlaylistItem struct {
		Name     string `json:"name"`
		URL      string `json:"url"`
		Playlist string `json:"playlist"`
	}

	var allTracks []PlaylistItem

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		http.Error(w, "Failed to read directory", http.StatusInternalServerError)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// این یک پوشه (آلبوم/پلی‌لیست) است
			playlistName := entry.Name()
			files, _ := os.ReadDir(filepath.Join(baseDir, playlistName))
			for _, f := range files {
				if !f.IsDir() {
					allTracks = append(allTracks, PlaylistItem{
						Name:     f.Name(),
						URL:      "/static/uploads/chat_media/playlists/" + playlistName + "/" + f.Name(),
						Playlist: playlistName,
					})
				}
			}
		} else {
			// فایل‌هایی که مستقیم در روت هستند
			allTracks = append(allTracks, PlaylistItem{
				Name:     entry.Name(),
				URL:      "/static/uploads/chat_media/playlists/" + entry.Name(),
				Playlist: "پیش‌فرض",
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(allTracks)
}

// UploadAudio آپلود موزیک با پشتیبانی از پوشه‌بندی و دسته‌بندی
func UploadAudio(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(50 << 20) // 50MB
	if err != nil {
		http.Error(w, "File too large", http.StatusBadRequest)
		return
	}

	file, handler, err := r.FormFile("audio_file")
	if err != nil {
		http.Error(w, "Invalid file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// 🚀 دریافت نام آلبوم/پوشه از فرانت‌اند
	playlistName := strings.TrimSpace(r.FormValue("playlist_name"))
	if playlistName == "" {
		playlistName = "سایر_آهنگ‌ها"
	}
	// پاکسازی کاراکترهای غیرمجاز برای نام پوشه
	playlistName = strings.ReplaceAll(playlistName, "/", "_")
	playlistName = strings.ReplaceAll(playlistName, "\\", "_")

	uploadDir := filepath.Join("static", "uploads", "chat_media", "playlists", playlistName)
	os.MkdirAll(uploadDir, os.ModePerm)

	fileName := handler.Filename
	filePath := filepath.Join(uploadDir, fileName)

	dst, err := os.Create(filePath)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	defer dst.Close()
	io.Copy(dst, file)

	fileURL := "/static/uploads/chat_media/playlists/" + playlistName + "/" + fileName

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": fileURL})
}

// MoveAudio انتقال آهنگ به آلبوم جدید (و ساخت خودکار پوشه)
func MoveAudio(w http.ResponseWriter, r *http.Request) {
	fileName := r.FormValue("file_name")
	oldPlaylist := r.FormValue("old_playlist")
	newPlaylist := strings.TrimSpace(r.FormValue("new_playlist"))

	if fileName == "" || oldPlaylist == "" || newPlaylist == "" {
		http.Error(w, "پارامترهای نامعتبر", http.StatusBadRequest)
		return
	}

	newPlaylist = strings.ReplaceAll(newPlaylist, "/", "_")
	newPlaylist = strings.ReplaceAll(newPlaylist, "\\", "_")

	// مدیریت آلبوم پیش‌فرض (فایل‌های روت)
	oldDir := filepath.Join("static", "uploads", "chat_media", "playlists")
	if oldPlaylist != "پیش‌فرض" {
		oldDir = filepath.Join(oldDir, oldPlaylist)
	}
	oldPath := filepath.Join(oldDir, fileName)

	// ساخت پوشه آلبوم جدید
	newDir := filepath.Join("static", "uploads", "chat_media", "playlists", newPlaylist)
	os.MkdirAll(newDir, os.ModePerm)
	newPath := filepath.Join(newDir, fileName)

	// جابجایی فایل فیزیکی در سرور
	err := os.Rename(oldPath, newPath)
	if err != nil {
		http.Error(w, "خطا در انتقال فایل", http.StatusInternalServerError)
		return
	}

	// اگر پوشه قدیمی خالی شد، آن را پاک کنیم تا دیتابیس تمیز بماند
	if oldPlaylist != "پیش‌فرض" {
		files, _ := os.ReadDir(filepath.Join("static", "uploads", "chat_media", "playlists", oldPlaylist))
		if len(files) == 0 {
			os.Remove(filepath.Join("static", "uploads", "chat_media", "playlists", oldPlaylist))
		}
	}

	w.WriteHeader(http.StatusOK)
}