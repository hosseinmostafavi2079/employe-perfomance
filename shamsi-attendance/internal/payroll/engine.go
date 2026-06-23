package payroll

import (
	"errors"
	"math"
)

// PayrollEngine ساختار موتور محاسباتی حقوق
type PayrollEngine struct {
	repo *PayrollRepository
}

// NewPayrollEngine نمونه‌سازی از موتور حقوق و دستمزد
func NewPayrollEngine(r *PayrollRepository) *PayrollEngine {
	return &PayrollEngine{repo: r}
}

// IssueMonthlyPayroll فیش حقوقی یک کارمند را محاسبه، مرخصی را بروزرسانی و فیش را صادر می‌کند
func (e *PayrollEngine) IssueMonthlyPayroll(employeeID int, year int, month int, actualHours float64, expectedHours float64, overtimeHours float64) (*PayrollSlip, error) {
	
	// ۱. دریافت پروفایل کارمند از دیتابیس
	profile, err := e.repo.GetProfileByEmployeeID(employeeID)
	if err != nil {
		return nil, errors.New("پروفایل کارمند یافت نشد")
	}

	// ۲. سیستم خودکار مدیریت مرخصی و کسر کارکرد
	// ابتدا ۲۰ ساعت مرخصی ماه جدید به کارمند اضافه می‌شود
	updatedLeaveBalance := profile.RemainingLeaveHours + MonthlyLeaveAccrual
	var leaveDeficit float64 = 0

	// اگر کارکرد واقعی کمتر از ساعت موظفی ماه باشد (مثال خرداد: ۱۷۰ به جای ۱۷۸)
	if actualHours < expectedHours {
		leaveDeficit = expectedHours - actualHours
		// کسر اتوماتیک از موجودی مرخصی کارمند
		updatedLeaveBalance = updatedLeaveBalance - leaveDeficit
	}

	// بروزرسانی مانده مرخصی کارمند در دیتابیس
	err = e.repo.UpdateLeaveBalance(employeeID, updatedLeaveBalance)
	if err != nil {
		return nil, errors.New("خطا در بروزرسانی مانده مرخصی")
	}

	// ۳. شروع محاسبات مالی بر اساس نوع کارمند
	slip := &PayrollSlip{
		EmployeeID:         employeeID,
		Year:               year,
		Month:              month,
		ExpectedWorkHours:  expectedHours,
		ActualWorkHours:    actualHours,
		LeaveDeficitHours:  leaveDeficit,
	}

	if profile.Type == ContractRegular {
		// --- حالت اول: نیروی رسمی و مشمول بیمه قانون کار ---
		
		// تعداد روزهای ماه بر اساس تقویم شمسی برای ضریب حقوق پایه
		daysInMonth := 30
		if month <= 6 {
			daysInMonth = 31 // نیمه اول سال ۳۱ روزه است
		} else if month == 12 {
			daysInMonth = 29 // اسفند ماه (بدون در نظر گرفتن کبیسه فرض ساده ۲۹ روز)
		}

		// محاسبه درآمدها
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

		// محاسبه اضافه‌کاری بر اساس نرخ اختصاصی تعیین شده توسط مدیر
		slip.OvertimeIncome = int64(overtimeHours) * profile.CustomOvertimeRate

		// جمع کل درآمد ناخالص
		slip.GrossEarnings = slip.BaseSalary + slip.BonAllowance + slip.HousingAllowance + 
			slip.MaritalAllowance + slip.ChildAllowance + slip.SeniorityAllowance + slip.OvertimeIncome

		// محاسبه بیمه ۷٪ سهم کارگر (حق اولاد مشمول بیمه نیست)
		insuredEarnings := slip.BaseSalary + slip.BonAllowance + slip.HousingAllowance + 
			slip.MaritalAllowance + slip.SeniorityAllowance + slip.OvertimeIncome
		
		slip.InsuranceDeduction = int64(math.Round(float64(insuredEarnings) * 0.07))
		slip.TotalDeductions = slip.InsuranceDeduction

		// خالص دریافتی نهایی
		slip.NetPayout = slip.GrossEarnings - slip.TotalDeductions

	} else if profile.Type == ContractHourly {
		// --- حالت دوم: نیروی ساعتی توافقی ---
		// حقوق بر اساس کل ساعت حضور ضرب در نرخ توافقی ثبت شده در پروفایل
		slip.BaseSalary = int64(actualHours) * profile.HourlyRate
		
		// نیروهای ساعتی شامل مزایای ثابت قانون کار و بیمه اجباری کارگاه در این فرمول نمی‌شوند
		slip.GrossEarnings = slip.BaseSalary
		slip.InsuranceDeduction = 0
		slip.TotalDeductions = 0
		slip.NetPayout = slip.GrossEarnings
	}

	// ۴. ذخیره فیش حقوقی نهایی در دیتابیس برای آرشیو مدیریت
	err = e.repo.SavePayrollSlip(slip)
	if err != nil {
		return nil, errors.New("خطا در ذخیره فیش حقوقی صادر شده")
	}

	return slip, nil
}