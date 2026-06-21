package attendance

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"shamsi_attendance/internal/database"
	"github.com/yaa110/go-persian-calendar"
)

// GetCurrentShamsiDate تاریخ امروز را به صورت متنی بازمی‌گرداند
func GetCurrentShamsiDate() string {
	now := time.Now()
	pt := ptime.New(now)
	return fmt.Sprintf("%d/%02d/%02d", pt.Year(), int(pt.Month()), pt.Day())
}

// ParseShamsiToTime تبدیل تاریخ شمسی و ساعت به زمان استاندارد سیستم
func ParseShamsiToTime(shamsiDateStr string, timeStr string) (time.Time, error) {
	dateParts := strings.Split(shamsiDateStr, "/")
	timeParts := strings.Split(timeStr, ":")

	if len(dateParts) != 3 || len(timeParts) != 2 {
		return time.Time{}, fmt.Errorf("فرمت تاریخ یا ساعت اشتباه است")
	}

	year, _ := strconv.Atoi(dateParts[0])
	month, _ := strconv.Atoi(dateParts[1])
	day, _ := strconv.Atoi(dateParts[2])
	hour, _ := strconv.Atoi(timeParts[0])
	minute, _ := strconv.Atoi(timeParts[1])

	iranLoc := time.FixedZone("Asia/Tehran", 12600)
	pt := ptime.Date(year, ptime.Month(month), day, hour, minute, 0, 0, iranLoc)
	return pt.Time(), nil
}

// CheckIn ثبت ورود زنده و خودکار مجهز به پایش ورودهای همپوشان و باز گذشته
func CheckIn(employeeCode string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// ترفند معماری: بررسی اینکه آیا این نیرو از قبل تردد ورود باز بدون خروج دارد یا خیر
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM attendance WHERE employee_code = $1 AND check_out IS NULL);`
	err := database.DB.QueryRow(ctx, checkQuery, employeeCode).Scan(&exists)
	if err != nil {
		return err
	}

	// اگر ورود باز یافت شد، سیستم با قاطعیت جلوی خراب شدن داده‌ها را می‌گیرد
	if exists {
		return fmt.Errorf("شما یک ورود باز ثبت‌شده دارید! ابتدا باید خروج زنده خود را ثبت کنید")
	}

	now := time.Now()
	shamsiDate := GetCurrentShamsiDate()

	query := `INSERT INTO attendance (employee_code, check_in, check_out, shamsi_date) VALUES ($1, $2, NULL, $3);`
	_, err = database.DB.Exec(ctx, query, employeeCode, now, shamsiDate)
	return err
}

// CheckOut ثبت خروج زنده و خودکار (دکمه سریع)
func CheckOut(employeeCode string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	query := `
		UPDATE attendance SET check_out = $1
		WHERE id = (
			SELECT id FROM attendance WHERE employee_code = $2 AND check_out IS NULL
			ORDER BY check_in DESC LIMIT 1
		);
	`
	result, err := database.DB.Exec(ctx, query, now, employeeCode)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("هیچ ورود بازی در سیستم برای شما یافت نشد")
	}
	return nil
}

// AddManualAttendance ثبت کاملاً دستی ورود و خروج برای روزهای گذشته یا فراموش شده
func AddManualAttendance(employeeCode string, shamsiDate string, checkInTime string, checkOutTime string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tIn, err := ParseShamsiToTime(shamsiDate, checkInTime)
	if err != nil {
		return fmt.Errorf("خطا در پردازش زمان ورود: %v", err)
	}

	tOut, err := ParseShamsiToTime(shamsiDate, checkOutTime)
	if err != nil {
		return fmt.Errorf("خطا در پردازش زمان خروج: %v", err)
	}

	query := `INSERT INTO attendance (employee_code, check_in, check_out, shamsi_date) VALUES ($1, $2, $3, $4);`
	_, err = database.DB.Exec(ctx, query, employeeCode, tIn, tOut, shamsiDate)
	return err
}