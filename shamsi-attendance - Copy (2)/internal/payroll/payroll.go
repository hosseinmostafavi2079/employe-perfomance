package payroll

import (
	"context"
	"fmt"
	"math"
	"time"

	"shamsi_attendance/internal/database"
)

// ContractType نوع قرارداد
type ContractType string

const (
	ContractRegular ContractType = "REGULAR"
	ContractHourly  ContractType = "HOURLY"
)

// EmployeeProfile پروفایل مالی
type EmployeeProfile struct {
	ID                   int          `json:"id"`
	EmployeeCode         string       `json:"employee_code"`
	ContractType         ContractType `json:"contract_type"`
	IsMarried            bool         `json:"is_married"`
	ChildCount           int          `json:"child_count"`
	EligibleForSeniority bool         `json:"eligible_for_seniority"`
	CustomOvertimeRate   int64        `json:"custom_overtime_rate"`
	HourlyRate           int64        `json:"hourly_rate"`
	RemainingLeaveHours  float64      `json:"remaining_leave_hours"`
	CreatedAt            time.Time    `json:"created_at"`
}

// PayrollSlip فیش حقوقی صادر شده (با تمامی ریز مبالغ)
type PayrollSlip struct {
	ID                 int          `json:"id"`
	EmployeeCode       string       `json:"employee_code"`
	Year               int          `json:"year"`
	Month              int          `json:"month"`
	ContractType       ContractType `json:"contract_type"`
	ExpectedWorkHours  float64      `json:"expected_work_hours"`
	ActualWorkHours    float64      `json:"actual_work_hours"`
	OvertimeHours      float64      `json:"overtime_hours"`
	BaseSalary         int64        `json:"base_salary"`
	BonAllowance       int64        `json:"bon_allowance"`
	HousingAllowance   int64        `json:"housing_allowance"`
	MaritalAllowance   int64        `json:"marital_allowance"`
	ChildAllowance     int64        `json:"child_allowance"`
	SeniorityAllowance int64        `json:"seniority_allowance"`
	OvertimeIncome     int64        `json:"overtime_income"`
	GrossEarnings      int64        `json:"gross_earnings"`
	InsuranceDeduction int64        `json:"insurance_deduction"`
	LeaveDeficitHours  float64      `json:"leave_deficit_hours"`
	TotalDeductions    int64        `json:"total_deductions"`
	NetPayout          int64        `json:"net_payout"`
	CreatedAt          time.Time    `json:"created_at"`
}

// IssueMonthlyPayroll موتور هسته مرکزی محاسبه مالی خالص (فقط محاسبه می‌کند و دیتابیس را آپدیت نمی‌کند)
func IssueMonthlyPayroll(ctx context.Context, employeeCode string, year, month int, actualHours, expectedHours, overtimeHours float64) (*PayrollSlip, error) {

	// ۱. واکشی مقادیر پایه از تنظیمات سالانه
	var cDailyBase, cDailySeniority, cMonthlyBon, cMonthlyHousing, cMonthlyMarital, cMonthlyChild int64
	var cLeaveAccrual float64

	queryConstants := `
		SELECT daily_base_wage, daily_seniority, monthly_bon, monthly_housing, monthly_marital, monthly_child, monthly_leave_accrual 
		FROM payroll_constants WHERE year = $1;`

	errConst := database.DB.QueryRow(ctx, queryConstants, year).Scan(
		&cDailyBase, &cDailySeniority, &cMonthlyBon, &cMonthlyHousing, &cMonthlyMarital, &cMonthlyChild, &cLeaveAccrual,
	)
	if errConst != nil {
		return nil, fmt.Errorf("مقادیر پایه برای سال %d تعریف نشده است", year)
	}

	// ۲. واکشی پروفایل مالی پرسنل
	var profile EmployeeProfile
	var cTypeStr string
	queryProfile := `
		SELECT contract_type, is_married, child_count, eligible_for_seniority, custom_overtime_rate, hourly_rate 
		FROM employee_profiles WHERE employee_code = $1;`

	errProf := database.DB.QueryRow(ctx, queryProfile, employeeCode).Scan(
		&cTypeStr, &profile.IsMarried, &profile.ChildCount, &profile.EligibleForSeniority, &profile.CustomOvertimeRate, &profile.HourlyRate,
	)
	if errProf != nil {
		return nil, fmt.Errorf("مشخصات مالی نیرو تنظیم نشده است")
	}
	profile.ContractType = ContractType(cTypeStr)

	// ۳. محاسبه کسر کارکرد
	var leaveDeficit float64 = 0
	if profile.ContractType == ContractRegular && actualHours < expectedHours {
		leaveDeficit = expectedHours - actualHours
	}

	// ۴. ساخت پیش‌نویس فیش
	slip := &PayrollSlip{
		EmployeeCode:      employeeCode,
		Year:              year,
		Month:             month,
		ContractType:      profile.ContractType,
		ExpectedWorkHours: expectedHours,
		ActualWorkHours:   actualHours,
		OvertimeHours:     overtimeHours,
		LeaveDeficitHours: leaveDeficit,
	}

	if profile.ContractType == ContractRegular {
		daysInMonth := 30
		if month <= 6 {
			daysInMonth = 31
		} else if month == 12 {
			daysInMonth = 29
		}

		slip.BaseSalary = cDailyBase * int64(daysInMonth)
		slip.BonAllowance = cMonthlyBon
		slip.HousingAllowance = cMonthlyHousing
		if profile.IsMarried {
			slip.MaritalAllowance = cMonthlyMarital
		}
		slip.ChildAllowance = cMonthlyChild * int64(profile.ChildCount)
		if profile.EligibleForSeniority {
			slip.SeniorityAllowance = cDailySeniority * int64(daysInMonth)
		}
		slip.OvertimeIncome = int64(overtimeHours) * profile.CustomOvertimeRate

		slip.GrossEarnings = slip.BaseSalary + slip.BonAllowance + slip.HousingAllowance +
			slip.MaritalAllowance + slip.ChildAllowance + slip.SeniorityAllowance + slip.OvertimeIncome

		insuredEarnings := slip.BaseSalary + slip.BonAllowance + slip.HousingAllowance +
			slip.MaritalAllowance + slip.SeniorityAllowance + slip.OvertimeIncome

		slip.InsuranceDeduction = int64(math.Round(float64(insuredEarnings) * 0.07))
		slip.TotalDeductions = slip.InsuranceDeduction
		slip.NetPayout = slip.GrossEarnings - slip.TotalDeductions

	} else if profile.ContractType == ContractHourly {
		slip.BaseSalary = int64(actualHours) * profile.HourlyRate
		slip.GrossEarnings = slip.BaseSalary
		slip.InsuranceDeduction = 0
		slip.TotalDeductions = 0
		slip.NetPayout = slip.GrossEarnings
	}

	return slip, nil
}