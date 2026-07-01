package attendance

import (
	"context"
	"fmt"
	"time"

	"shamsi_attendance/internal/database"
)

// ProjectReportStruct ساختاری برای نگهداری اطلاعات تفکیکی هر پروژه است
type ProjectReportStruct struct {
	ProjectName string
	TotalHours  float64
}

// GenerateMonthlyShamsiReport گزارش کامل کارکرد یک کارمند را در یک ماه شمسی خاص استخراج می‌کند
// پارامتر yearMonth باید به صورت فرمت "1405/03" وارد شود
func GenerateMonthlyShamsiReport(employeeCode string, yearMonth string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Printf("\n==================================================\n")
	fmt.Printf("گزارش مدیریتی ماهانه شمسی برای کارمند: %s\n", employeeCode)
	fmt.Printf("دوره گزارش: %s\n", yearMonth)
	fmt.Printf("==================================================\n")

	// ۱. استخراج مجموع کل ساعات کارکرد کارمند در این ماه شمسی
	var totalHours float64
	totalQuery := `
		SELECT COALESCE(SUM(hours_spent), 0) 
		FROM work_logs 
		WHERE employee_code = $1 AND shamsi_date LIKE $2;
	`
	// علامت % در انتهای yearMonth یعنی هر چیزی که با این متن شروع می‌شود (مثلا 1405/03/01 تا 1405/03/31)
	err := database.DB.QueryRow(ctx, totalQuery, employeeCode, yearMonth+"%").Scan(&totalHours)
	if err != nil {
		fmt.Printf("خطا در محاسبه مجموع ساعات: %v\n", err)
		return
	}

	fmt.Printf("🔹 مجموع کل کارکرد در این ماه: %.2f ساعت\n", totalHours)
	fmt.Printf("--------------------------------------------------\n")
	fmt.Printf("تفکیک کارکرد به ازای هر پروژه:\n")

	// ۲. استخراج لیست پروژه‌ها و میزان ساعت صرف شده روی هرکدام در این ماه شمسی
	breakdownQuery := `
		SELECT p.name, SUM(w.hours_spent)
		FROM work_logs w
		JOIN projects p ON w.project_id = p.id
		WHERE w.employee_code = $1 AND w.shamsi_date LIKE $2
		GROUP BY p.name;
	`

	rows, err := database.DB.Query(ctx, breakdownQuery, employeeCode, yearMonth+"%")
	if err != nil {
		fmt.Printf("خطا در استخراج تفکیک پروژه‌ها: %v\n", err)
		return
	}
	defer rows.Close()

	hasData := false
	for rows.Next() {
		hasData = true
		var projectName string
		var projectHours float64
		err := rows.Scan(&projectName, &projectHours)
		if err != nil {
			fmt.Printf("خطا در خواندن اطلاعات ردیف: %v\n", err)
			return
		}
		fmt.Printf("  🔸 پروژه: %-25s | کارکرد: %.2f ساعت\n", projectName, projectHours)
	}

	if !hasData {
		fmt.Println("  ⚠️ هیچ اطلاعات کارکردی برای این کارمند در ماه مورد نظر ثبت نشده است.")
	}
	fmt.Printf("==================================================\n")
}