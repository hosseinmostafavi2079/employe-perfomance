package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// DB متغیر عمومی دسترسی به دیتابیس داکر
var DB *pgxpool.Pool

// ConnectToDatabase برقراری اتصال امن با دیتابیس (پشتیبانی از هماهنگی محیط داکر و لوکال)
func ConnectToDatabase() {
	// استفاده از متغیر محیطی در صورت وجود، در غیر این صورت بازگشت به مقدار پیش‌فرض پورت ۵۴۳۳
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://attendance_admin:shamsi_secure_pass_2026@localhost:5433/shamsi_attendance_platform?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("خطا در ایجاد بستر اتصال به پایگاه داده: %v\n", err)
	}

	err = pool.Ping(ctx)
	if err != nil {
		log.Fatalf("پایگاه داده پاسخ نمی‌دهد! خطا: %v\n", err)
	}

	DB = pool
	fmt.Println("تاییدیه معماری: اتصال امن به پایگاه داده PostgreSQL برقرار شد.")
	
	CreateTables()
}

// CreateTables ساخت ساختار پیشرفته پرسنلی و به‌روزرسانی اجباری پسوردها به صورت هش‌شده
func CreateTables() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// ۱. ایجاد جداول در صورت عدم وجود
	queries := []string{
		`CREATE TABLE IF NOT EXISTS employees (
			id SERIAL PRIMARY KEY,
			employee_code VARCHAR(50) UNIQUE NOT NULL,
			full_name VARCHAR(255) NOT NULL,
			role VARCHAR(50) NOT NULL DEFAULT 'EMPLOYEE',
			created_at TIMESTAMPTZ DEFAULT NOW()
		);`,

		`ALTER TABLE employees ADD COLUMN IF NOT EXISTS password VARCHAR(255) NOT NULL DEFAULT '123456';`,

		`CREATE TABLE IF NOT EXISTS projects (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) UNIQUE NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);`,

		`CREATE TABLE IF NOT EXISTS attendance (
			id SERIAL PRIMARY KEY,
			employee_code VARCHAR(50) NOT NULL,
			check_in TIMESTAMPTZ NOT NULL,
			check_out TIMESTAMPTZ,
			shamsi_date VARCHAR(10) NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS work_logs (
			id SERIAL PRIMARY KEY,
			employee_code VARCHAR(50) NOT NULL,
			project_id INT REFERENCES projects(id) ON DELETE CASCADE,
			hours_spent NUMERIC(4,2) NOT NULL,
			description TEXT,
			shamsi_date VARCHAR(10) NOT NULL
		);`,
	}

	for _, query := range queries {
		_, err := DB.Exec(ctx, query)
		if err != nil {
			log.Fatalf("خطا در ساخت یا ارتقای جداول پایگاه داده: %v\n", err)
		}
	}

	// ایجاد هش امن برای کاربران پایه جهت جلوگیری از ذخیره متن خام در دیتابیس
	adminHash, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	empHash, _ := bcrypt.GenerateFromPassword([]byte("emp123"), bcrypt.DefaultCost)

	// ۲. تزریق و آپدیت کاربران ادمین و کارمند نمونه با پسوردهای امن ساختاریافته
	seedQuery := fmt.Sprintf(`
		INSERT INTO employees (employee_code, full_name, password, role)
		VALUES 
			('ADMIN', 'مدیر کل سیستم ارشد', '%s', 'ADMIN'),
			('EMP-1001', 'مهندس علیرضا حسینی', '%s', 'EMPLOYEE')
		ON CONFLICT (employee_code) 
		DO UPDATE SET 
			password = EXCLUDED.password,
			role = EXCLUDED.role,
			full_name = EXCLUDED.full_name;
	`, string(adminHash), string(empHash))

	_, err := DB.Exec(ctx, seedQuery)
	if err != nil {
		log.Printf("⚠️ خطا در ست کردن اطلاعات پرسنل پایه: %v\n", err)
	}

	fmt.Println("--------------------------------------------------")
	fmt.Println("🔑 اطلاعات ورود مجاز به سیستم (با هش پیشرفته Bcrypt به‌روزرسانی شد):")
	fmt.Println("   ۱. پورتال مدیر: نام کاربری [ADMIN] | رمز عبور [admin123]")
	fmt.Println("   ۲. پورتال کارمند: نام کاربری [EMP-1001] | رمز عبور [emp123]")
	fmt.Println("--------------------------------------------------")
}