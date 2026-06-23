package payroll

import (
	"context"
	"fmt"
	"math"
	"shamsi_attendance/internal/database"
)

// IssueMonthlyPayroll هسته مرکزی لایه دامین برای پردازش موتور مرخصی و محاسبات مالی حقوق قانون کار ۱۴۰۵
func IssueMonthlyPayroll(ctx context.Context, employeeCode string, year, month int, actualHours, expectedHours, overtimeHours float64) (*PayrollSlip, error) {
	
	// ۱. واکشی پروفایل مالی و مدیریتی پرسنل از پایگاه داده
	var profile EmployeeProfile
	queryProfile := `
		SELECT contract_type, is_married, child_count, eligible_for_seniority, 
		       custom_overtime_rate, hourly_rate, remaining_leave_hours 
		FROM employee_profiles 
		WHERE employee_code = $1;`

	err := database.DB.QueryRow(ctx, queryProfile, employeeCode).Scan(
		&profile.ContractType, &profile.IsMarried, &profile.ChildCount, &profile.EligibleForSeniority,
		&profile.CustomOvertimeRate, &profile.HourlyRate, &profile.RemainingLeaveHours,
	)
	if err != nil {
		return nil, fmt.Errorf("ابتدا باید مشخصات مالی این نیرو را در بخش مدیریت تنظیم نمایید")
	}

	// ۲. سیستم خودکار مدیریت موتور مرخصی و محاسبه کسر کارکرد
	// تزریق ۲۰ ساعت مرخصی استحقاقی ماه جدید مطابق قانون کار
	updatedLeaveBalance := profile.RemainingLeaveHours + MonthlyLeaveAccrual
	var leaveDeficit float64 = 0

	// پایش کسر کارکرد: اگر کارکرد واقعی کمتر از موظفی باشد، مابه‌التفاوت از مرخصی کسر می‌شود
	if actualHours < expectedHours {
		leaveDeficit = expectedHours - actualHours
		updatedLeaveBalance = updatedLeaveBalance - leaveDeficit
	}

	// بروزرسانی آنی و امن مانده مرخصی کارمند در دیتابیس داکر
	_, err = database.DB.Exec(ctx, "UPDATE employee_profiles SET remaining_leave_hours = $1 WHERE employee_code = $2;", updatedLeaveBalance, employeeCode)
	if err != nil {
		return nil, fmt.Errorf("خطا در بروزرسانی خودکار لایه مرخصی: %w", err)
	}

	// ۳. ساخت آبجکت فیش حقوقی نهایی و شروع محاسبات دقیق لایه مالی
	slip := &PayrollSlip{
		EmployeeCode:      employeeCode,
		Year:              year,
		Month:             month,
		ExpectedWorkHours: expectedHours,
		ActualWorkHours:   actualHours,
		LeaveDeficitHours: leaveDeficit,
	}

	if profile.ContractType == ContractRegular {
		// --- حالت اول: نیروی رسمی مشمول بیمه و مزایای ثابت قانون کار ---
		daysInMonth := 30
		if month <= 6 {
			daysInMonth = 31 // نیمه اول سال ۳۱ روزه
		} else if month == 12 {
			daysInMonth = 29 // اسفند ماه قانون کار
		}

		// محاسبه اقلام درآمدی بر پایه ریال
		slip.BaseSalary = DailyBaseWage1405 * int64(daysInMonth)
		slip.BonAllowance = MonthlyBon1405
		slip.HousingAllowance = MonthlyHousing1405
		
		if profile.IsMarried {
			slip.MaritalAllowance = MonthlyMarital1405
		}
		
		slip.ChildAllowance = MonthlyChildPerOne1405 * int64(profile.ChildCount)
		
		if profile.EligibleForSeniority {
			slip.SeniorityAllowance = DailySeniority1405 * int64(daysInMonth)
		}

		// محاسبه درآمد اضافه‌کاری بر اساس نرخ توافقی ثبت شده توسط مدیر
		slip.OvertimeIncome = int64(overtimeHours) * profile.CustomOvertimeRate

		// جمع کل درآمد ناخالص
		slip.GrossEarnings = slip.BaseSalary + slip.BonAllowance + slip.HousingAllowance + 
			slip.MaritalAllowance + slip.ChildAllowance + slip.SeniorityAllowance + slip.OvertimeIncome

		// محاسبه فرمول بیمه ۷٪ سهم کارگر (حق اولاد طبق قانون مشمول بیمه نیست)
		insuredEarnings := slip.BaseSalary + slip.BonAllowance + slip.HousingAllowance + 
			slip.MaritalAllowance + slip.SeniorityAllowance + slip.OvertimeIncome
		
		slip.InsuranceDeduction = int64(math.Round(float64(insuredEarnings) * 0.07))
		slip.TotalDeductions = slip.InsuranceDeduction

		// خالص دریافتی قابل پرداخت
		slip.NetPayout = slip.GrossEarnings - slip.TotalDeductions

	} else if profile.ContractType == ContractHourly {
		// --- حالت دوم: نیروی ساعتی توافقی پروژه‌ای ---
		slip.BaseSalary = int64(actualHours) * profile.HourlyRate
		slip.GrossEarnings = slip.BaseSalary
		slip.InsuranceDeduction = 0
		slip.TotalDeductions = 0
		slip.NetPayout = slip.GrossEarnings
	}

	// ۴. ذخیره و آرشیو فیش حقوقی صادر شده در جدول مربوطه
	insertQuery := `
		INSERT INTO payroll_slips (
			employee_code, year, month, expected_work_hours, actual_work_hours,
			base_salary, bon_allowance, housing_allowance, marital_allowance, child_allowance,
			seniority_allowance, overtime_income, gross_earnings, insurance_deduction,
			leave_deficit_hours, total_deductions, net_payout
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17);`

	_, err = database.DB.Exec(ctx, insertQuery,
		slip.EmployeeCode, slip.Year, slip.Month, slip.ExpectedWorkHours, slip.ActualWorkHours,
		slip.BaseSalary, slip.BonAllowance, slip.HousingAllowance, slip.MaritalAllowance, slip.ChildAllowance,
		slip.SeniorityAllowance, slip.OvertimeIncome, slip.GrossEarnings, slip.InsuranceDeduction,
		slip.LeaveDeficitHours, slip.TotalDeductions, slip.NetPayout,
	)
	if err != nil {
		return nil, fmt.Errorf("خطا در آرشیو نهایی فیش حقوقی در پایگاه داده: %w", err)
	}

	return slip, nil
}