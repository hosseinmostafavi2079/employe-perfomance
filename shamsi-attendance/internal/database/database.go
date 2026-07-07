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

// ConnectToDatabase برقراری اتصال امن با دیتابیس
func ConnectToDatabase() {
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

// CreateTables ساخت ساختار پیشرفته پرسنلی و سیستم تیم‌بندی
func CreateTables() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

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
		`CREATE TABLE IF NOT EXISTS employee_profiles (
			id SERIAL PRIMARY KEY,
			employee_code VARCHAR(50) UNIQUE NOT NULL REFERENCES employees(employee_code) ON DELETE CASCADE,
			contract_type VARCHAR(50) NOT NULL DEFAULT 'REGULAR',
			is_married BOOLEAN DEFAULT FALSE,
			child_count INT DEFAULT 0,
			eligible_for_seniority BOOLEAN DEFAULT FALSE,
			custom_overtime_rate BIGINT DEFAULT 0,
			hourly_rate BIGINT DEFAULT 0,
			remaining_leave_hours NUMERIC(6,2) DEFAULT 0.0,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);`,
		`CREATE TABLE IF NOT EXISTS payroll_slips (
			id SERIAL PRIMARY KEY,
			employee_code VARCHAR(50) NOT NULL REFERENCES employees(employee_code) ON DELETE CASCADE,
			year INT NOT NULL,
			month INT NOT NULL,
			expected_work_hours NUMERIC(6,2) NOT NULL,
			actual_work_hours NUMERIC(6,2) NOT NULL,
			base_salary BIGINT NOT NULL,
			bon_allowance BIGINT NOT NULL,
			housing_allowance BIGINT NOT NULL,
			marital_allowance BIGINT NOT NULL,
			child_allowance BIGINT NOT NULL,
			seniority_allowance BIGINT NOT NULL,
			overtime_income BIGINT NOT NULL,
			gross_earnings BIGINT NOT NULL,
			insurance_deduction BIGINT NOT NULL,
			leave_deficit_hours NUMERIC(6,2) NOT NULL,
			total_deductions BIGINT NOT NULL,
			net_payout BIGINT NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);`,
		`CREATE TABLE IF NOT EXISTS user_biometrics (
			id SERIAL PRIMARY KEY,
			employee_code VARCHAR(50) NOT NULL REFERENCES employees(employee_code) ON DELETE CASCADE,
			credential_id BYTEA NOT NULL UNIQUE,
			public_key BYTEA NOT NULL,
			attestation_type VARCHAR(255),
			sign_count BIGINT DEFAULT 0,
			aaguid BYTEA,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);`,

		// ============================================
		// 🚨 جداول جدید برای ماژول تیم‌بندی (GBAC)
		// ============================================
		`CREATE TABLE IF NOT EXISTS teams (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) UNIQUE NOT NULL,
			description TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);`,
		`CREATE TABLE IF NOT EXISTS employee_teams (
			employee_code VARCHAR(50) NOT NULL REFERENCES employees(employee_code) ON DELETE CASCADE,
			team_id INT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
			PRIMARY KEY (employee_code, team_id)
		);`,
		`CREATE TABLE IF NOT EXISTS team_projects (
			team_id INT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
			project_id INT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			PRIMARY KEY (team_id, project_id)
		);`,
	}

	for _, query := range queries {
		_, err := DB.Exec(ctx, query)
		if err != nil {
			log.Fatalf("خطا در ساخت یا ارتقای جداول پایگاه داده: %v\n", err)
		}
	}

	adminUser := os.Getenv("INITIAL_ADMIN_USER")
	if adminUser == "" {
		adminUser = "SUPER_ADMIN"
	}
	adminPass := os.Getenv("INITIAL_ADMIN_PASS")
	if adminPass == "" {
		adminPass = "shamsi_admin_password"
	}
	adminHash, _ := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)

	seedQuery := `
		INSERT INTO employees (employee_code, full_name, password, role)
		VALUES ($1, $2, $3, 'ADMIN')
		ON CONFLICT (employee_code) 
		DO UPDATE SET 
			password = EXCLUDED.password,
			role = EXCLUDED.role,
			full_name = EXCLUDED.full_name;
	`
	_, err := DB.Exec(ctx, seedQuery, adminUser, "مدیر کل سیستم ارشد", string(adminHash))
	if err != nil {
		log.Printf("⚠️ خطا در ست کردن اطلاعات مدیر ارشد پایه: %v\n", err)
	}
}