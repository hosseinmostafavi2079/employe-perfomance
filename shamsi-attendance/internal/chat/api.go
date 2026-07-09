package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"shamsi_attendance/internal/database"
)

type ChatContact struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// GetContacts دریافت لیست پرسنل همراه با عکس پروفایل (بدون خطای SQL)
func GetContacts(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT e.employee_code, e.full_name, COALESCE(ep.avatar_path, '') 
		FROM employees e 
		LEFT JOIN employee_profiles ep ON e.employee_code = ep.employee_code
	`
	rows, err := database.DB.Query(context.Background(), query)
	if err != nil {
		http.Error(w, "DB Error", 500)
		return
	}
	defer rows.Close()
	
	var contacts []struct {
		Code   string `json:"code"`
		Name   string `json:"name"`
		Avatar string `json:"avatar"`
	}
	for rows.Next() {
		var code, name, avatar string
		rows.Scan(&code, &name, &avatar)
		contacts = append(contacts, struct{Code string `json:"code"`; Name string `json:"name"`; Avatar string `json:"avatar"`}{code, name, avatar})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(contacts)
}

// GetUserRooms دریافت لیست چت‌ها
func GetUserRooms(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	
	var roomExists int
	database.DB.QueryRow(context.Background(), "SELECT 1 FROM chat_rooms WHERE id = 1").Scan(&roomExists)
	if roomExists == 0 {
		database.DB.Exec(context.Background(), "INSERT INTO chat_rooms (id, name, room_type) VALUES (1, 'رادیو سازمان', 'group')")
		database.DB.Exec(context.Background(), "SELECT setval(pg_get_serial_sequence('chat_rooms', 'id'), coalesce(max(id),1), false) FROM chat_rooms")
	}
	database.DB.Exec(context.Background(), "INSERT INTO chat_room_members (room_id, user_id) VALUES (1, $1) ON CONFLICT DO NOTHING", userID)

	query := `
		SELECT cr.id, cr.name, cr.room_type 
		FROM chat_rooms cr
		JOIN chat_room_members crm ON cr.id = crm.room_id
		WHERE crm.user_id = $1 ORDER BY cr.id ASC
	`
	rows, _ := database.DB.Query(context.Background(), query, userID)
	defer rows.Close()

	var rooms []ChatRoom
	for rows.Next() {
		var room ChatRoom
		rows.Scan(&room.ID, &room.Name, &room.RoomType)
		
		if room.RoomType == RoomTypeDirect {
			database.DB.QueryRow(context.Background(), `
				SELECT e.full_name, COALESCE(ep.avatar_path, '')
				FROM employees e 
				LEFT JOIN employee_profiles ep ON e.employee_code = ep.employee_code
				JOIN chat_room_members crm ON e.employee_code = crm.user_id 
				WHERE crm.room_id = $1 AND crm.user_id != $2 LIMIT 1`, room.ID, userID).Scan(&room.Name, &room.Avatar)
		}
		rooms = append(rooms, room)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rooms)
}

// CreateRoom ساخت چت خصوصی یا گروه جدید
func CreateRoom(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CreatorID string   `json:"creator_id"`
		TargetID  string   `json:"target_id"`
		GroupName string   `json:"group_name"`
		Members   []string `json:"members"`
		RoomType  string   `json:"room_type"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	ctx := context.Background()
	
	if req.RoomType == RoomTypeDirect {
		var existingRoomID int
		err := database.DB.QueryRow(ctx, `
			SELECT r.id FROM chat_rooms r
			JOIN chat_room_members m1 ON r.id = m1.room_id
			JOIN chat_room_members m2 ON r.id = m2.room_id
			WHERE r.room_type = 'direct' AND m1.user_id = $1 AND m2.user_id = $2
		`, req.CreatorID, req.TargetID).Scan(&existingRoomID)
		
		if err == nil && existingRoomID > 0 {
			json.NewEncoder(w).Encode(map[string]int{"room_id": existingRoomID})
			return
		}
		
		err = database.DB.QueryRow(ctx, "INSERT INTO chat_rooms (name, room_type) VALUES ('Direct', 'direct') RETURNING id").Scan(&existingRoomID)
		if err == nil {
			database.DB.Exec(ctx, "INSERT INTO chat_room_members (room_id, user_id) VALUES ($1, $2), ($1, $3)", existingRoomID, req.CreatorID, req.TargetID)
		}
		json.NewEncoder(w).Encode(map[string]int{"room_id": existingRoomID})
		return
	}
	
	if req.RoomType == RoomTypeGroup {
		var newRoomID int
		err := database.DB.QueryRow(ctx, "INSERT INTO chat_rooms (name, room_type) VALUES ($1, 'group') RETURNING id", req.GroupName).Scan(&newRoomID)
		if err == nil {
			database.DB.Exec(ctx, "INSERT INTO chat_room_members (room_id, user_id) VALUES ($1, $2)", newRoomID, req.CreatorID)
			for _, mem := range req.Members {
				database.DB.Exec(ctx, "INSERT INTO chat_room_members (room_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", newRoomID, mem)
			}
		}
		json.NewEncoder(w).Encode(map[string]int{"room_id": newRoomID})
		return
	}
}

// UploadChatMedia آپلود فایل و عکس
func UploadChatMedia(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(50 << 20)
	if err != nil {
		http.Error(w, "File too large", http.StatusBadRequest)
		return
	}
	file, handler, err := r.FormFile("chat_media")
	if err != nil {
		http.Error(w, "Invalid file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	uploadDir := filepath.Join("static", "uploads", "chat_media", "files")
	os.MkdirAll(uploadDir, os.ModePerm)

	fileName := fmt.Sprintf("%d_%s", time.Now().Unix(), handler.Filename)
	filePath := filepath.Join(uploadDir, fileName)
	dst, _ := os.Create(filePath)
	defer dst.Close()
	io.Copy(dst, file)

	fileURL := "/static/uploads/chat_media/files/" + fileName
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": fileURL, "name": handler.Filename})
}

// DeleteMessageAPI سیستم حذف هوشمند
func DeleteMessageAPI(w http.ResponseWriter, r *http.Request) {
	msgID, _ := strconv.Atoi(r.URL.Query().Get("msg_id"))
	userID := r.URL.Query().Get("user_id")
	
	var mediaURL, senderID, msgType string
	var roomID int
	err := database.DB.QueryRow(context.Background(), "SELECT media_url, sender_id, room_id, message_type FROM chat_messages WHERE id=$1", msgID).Scan(&mediaURL, &senderID, &roomID, &msgType)
	
	if err != nil || senderID != userID {
		http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}
	
	if msgType == MessageTypeFile && mediaURL != "" {
		os.Remove("." + mediaURL)
	}
	
	database.DB.Exec(context.Background(), "DELETE FROM chat_messages WHERE id=$1", msgID)
	
	if AppHub != nil {
		AppHub.Broadcast <- Message{ RoomID: roomID, MessageType: MessageTypeDelete, MessageText: strconv.Itoa(msgID) }
	}
	w.WriteHeader(http.StatusOK)
}