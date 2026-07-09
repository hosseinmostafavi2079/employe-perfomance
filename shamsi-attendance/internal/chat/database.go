package chat

import (
	"context"
	"log"

	"shamsi_attendance/internal/database"
)

func InitChatTables() error {
	query := `
	CREATE TABLE IF NOT EXISTS chat_rooms (
		id SERIAL PRIMARY KEY, name VARCHAR(255), room_type VARCHAR(50) NOT NULL DEFAULT 'group', created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS chat_room_members (
		room_id INTEGER NOT NULL, user_id VARCHAR(100) NOT NULL, joined_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (room_id, user_id)
	);
	CREATE TABLE IF NOT EXISTS chat_messages (
		id SERIAL PRIMARY KEY, room_id INTEGER NOT NULL, sender_id VARCHAR(100) NOT NULL, message_text TEXT, message_type VARCHAR(50) DEFAULT 'text', media_url VARCHAR(500), created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS chat_playlists (
		id SERIAL PRIMARY KEY, room_id INTEGER NOT NULL, added_by VARCHAR(100) NOT NULL, file_path VARCHAR(500) NOT NULL, file_name VARCHAR(255) NOT NULL, created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := database.DB.Exec(context.Background(), query)
	if err != nil {
		return err
	}

	// آپدیت افزایشی و بدون قطعی جداول فعلی برای ذخیره نام و عکس
	_, _ = database.DB.Exec(context.Background(), `ALTER TABLE chat_messages ADD COLUMN IF NOT EXISTS sender_name VARCHAR(255) DEFAULT 'کاربر';`)
	_, _ = database.DB.Exec(context.Background(), `ALTER TABLE chat_messages ADD COLUMN IF NOT EXISTS sender_avatar VARCHAR(500) DEFAULT '';`)

	log.Println("[SUCCESS] Chat module tables verified/updated successfully.")
	return nil
}