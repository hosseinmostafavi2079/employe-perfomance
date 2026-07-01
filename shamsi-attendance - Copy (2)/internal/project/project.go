package project

import (
	"context"
	"fmt"
	"time"

	"shamsi_attendance/internal/database"
	"github.com/yaa110/go-persian-calendar"
)

// GetCurrentShamsiDate یک تابع کمکی برای دریافت تاریخ امروز به صورت شمسی است
func GetCurrentShamsiDate() string {
	now := time.Now()
	pt := ptime.New(now)
	return fmt.Sprintf("%d/%02d/%02d", pt.Year(), int(pt.Month()), pt.Day())
}

// CreateProject یک پروژه جدید در سیستم تعریف می‌کند
func CreateProject(name string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var projectID int
	query := `
		INSERT INTO projects (name)
		VALUES ($1)
		ON CONFLICT DO NOTHING
		RETURNING id;
	`
	err := database.DB.QueryRow(ctx, query, name).Scan(&projectID)
	if err != nil {
		err = database.DB.QueryRow(ctx, "SELECT id FROM projects WHERE name=$1;", name).Scan(&projectID)
		if err != nil {
			return 0, fmt.Errorf("خطا در تعریف پروژه: %v", err)
		}
	}

	return projectID, nil
}

// DeleteProject وظیفه حذف کامل پروژه و پاکسازی آبشاری کارکردهای متصل به آن را دارد
func DeleteProject(id int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := "DELETE FROM projects WHERE id = $1;"
	_, err := database.DB.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("خطا در حذف پروژه از پایگاه داده: %v", err)
	}
	return nil
}

// LogWorkWithDate وظیفه ثبت کارکرد روزانه (برای امروز یا تاریخ‌های گذشته) را دارد
func LogWorkWithDate(employeeCode string, projectID int, hours float64, description string, shamsiDate string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		INSERT INTO work_logs (employee_code, project_id, hours_spent, description, shamsi_date)
		VALUES ($1, $2, $3, $4, $5);
	`

	_, err := database.DB.Exec(ctx, query, employeeCode, projectID, hours, description, shamsiDate)
	if err != nil {
		return fmt.Errorf("خطا در ثبت کارکرد روزانه: %v", err)
	}

	return nil
}

// UpdateWorkLog وظیفه ویرایش یک گزارش کارکرد از قبل ثبت شده را دارد
func UpdateWorkLog(logID int, projectID int, hours float64, description string, shamsiDate string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		UPDATE work_logs
		SET project_id = $1, hours_spent = $2, description = $3, shamsi_date = $4
		WHERE id = $5;
	`

	// مشکل اینجا بود: shamsiDate جا افتاده بود که در خط زیر به عنوان پارامتر چهارم اضافه شد
	_, err := database.DB.Exec(ctx, query, projectID, hours, description, shamsiDate, logID)
	if err != nil {
		return fmt.Errorf("خطا در ویرایش کارکرد: %v", err)
	}

	return nil
}

// DeleteWorkLog وظیفه حذف یک گزارش کارکرد را دارد
func DeleteWorkLog(logID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := "DELETE FROM work_logs WHERE id = $1;"
	_, err := database.DB.Exec(ctx, query, logID)
	if err != nil {
		return fmt.Errorf("خطا در حذف کارکرد: %v", err)
	}

	return nil
}