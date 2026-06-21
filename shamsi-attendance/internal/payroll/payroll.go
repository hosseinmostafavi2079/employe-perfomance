package payroll

import (
	"context"
	"fmt"
	"time"

	"shamsi_attendance/internal/database"
)

// SalaryCalculator همان قرارداد رسمی (Interface) ماست.
// هر استراتژی محاسباتی که در آینده اضافه شود، باید این دو رفتار را پیاده‌سازی کند.
type SalaryCalculator interface {
	CalculateSalary(totalHours float64) float64
	GetStrategyName() string
}

// -----------------------------------------------------------------
// ۱. استراتژی اول: حقوق استاندارد مطابق قانون کار (ماهانه ثابت با شرط ساعت موظفی)
type LaborLawStrategy struct {
	BaseMonthlySalary float64 // حقوق پایه مصوب قانون کار برای یک ماه
	RequiredHours     float64 // ساعات موظفی کار در آن ماه (مثلاً ۱۹۲ ساعت)
}

// CalculateSalary پیاده‌سازی فرمول قانون کار: اگر کارمند ساعت موظفی را پر کند حقوق کامل می‌گیرد، در غیر این صورت به نسبت کارکرد
func (l LaborLawStrategy) CalculateSalary(totalHours float64) float64 {
	if totalHours >= l.RequiredHours {
		return l.BaseMonthlySalary
	}
	// محاسبه حقوق به نسبت ساعات کارکرد واقعی
	return (totalHours / l.RequiredHours) * l.BaseMonthlySalary
}

func (l LaborLawStrategy) GetStrategyName() string {
	return "قانون کار استاندارد (ماهانه مصوب)"
}

// -----------------------------------------------------------------
// ۲. استراتژی دوم: قرارداد ساعتی (دستمزد مستقیم بر اساس میزان ساعت کارکرد)
type HourlyStrategy struct {
	RatePerHour float64 // دستمزد مصوب برای هر یک ساعت کار
}

// CalculateSalary پیاده‌سازی فرمول ساعتی
func (h HourlyStrategy) CalculateSalary(totalHours float64) float64 {
	return totalHours * h.RatePerHour
}

func (h HourlyStrategy) GetStrategyName() string {
	return "قرارداد ساعتی پروژه‌ای"
}

// -----------------------------------------------------------------
// تابع اصلی ماژول حقوق و دستمزد برای استخراج ساعات از دیتابیس و محاسبه فیش حقوقی
func RunPayrollCalculation(employeeCode string, yearMonth string, strategy SalaryCalculator) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// استخراج مجموع ساعات کارکرد کارمند در آن ماه شمسی خاص از پایگاه داده
	var totalHours float64
	query := `
		SELECT COALESCE(SUM(hours_spent), 0) 
		FROM work_logs 
		WHERE employee_code = $1 AND shamsi_date LIKE $2;
	`
	err := database.DB.QueryRow(ctx, query, employeeCode, yearMonth+"%").Scan(&totalHours)
	if err != nil {
		fmt.Printf("خطا در بازخوانی کارکرد برای محاسبه حقوق: %v\n", err)
		return
	}

	// استفاده داینامیک از اینترفیس برای محاسبه حقوق (بدون اینکه این تابع بداند فرمول چیست!)
	finalSalary := strategy.CalculateSalary(totalHours)

	// چاپ فیش حقوقی صادر شده بر اساس ماه‌های شمسی
	fmt.Printf("\n==================================================\n")
	fmt.Printf("🧾 فیش حقوقی سیستمی (دوره شمسی: %s)\n", yearMonth)
	fmt.Printf("کارمند: %s\n", employeeCode)
	fmt.Printf("نوع قرارداد محاسباتی: %s\n", strategy.GetStrategyName())
	fmt.Printf("کل کارکرد ثبت شده: %.2f ساعت\n", totalHours)
	fmt.Printf("💰 مبلغ قابل پرداخت: %.0f تومان\n", finalSalary)
	fmt.Printf("==================================================\n")
}