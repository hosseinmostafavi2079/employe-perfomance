package payroll

import (
	"time"
)

// ContractType نوع قرارداد نیرو را مشخص می‌کند
type ContractType string

const (
	ContractRegular ContractType = "REGULAR" // رسمی و مشمول بیمه
	ContractHourly  ContractType = "HOURLY"  // ساعتی توافقی
)

// EmployeeProfile پروفایل کامل کارمند که توسط مدیر یک‌بار تنظیم می‌شود
type EmployeeProfile struct {
	ID                   int          `json:"id"`
	FullName             string       `json:"full_name"`
	Type                 ContractType `json:"type"`
	IsMarried            bool         `json:"is_married"`
	ChildCount           int          `json:"child_count"`
	EligibleForSeniority bool         `json:"eligible_for_seniority"` // مشمول پایه سنوات
	CustomOvertimeRate   int64        `json:"custom_overtime_rate"`   // نرخ ساعتی اضافه‌کاری توافقی (ریال)
	HourlyRate           int64        `json:"hourly_rate"`            // نرخ کل ساعت کارکرد برای نیروی ساعتی (ریال)
	RemainingLeaveHours  float64      `json:"remaining_leave_hours"`  // باقیمانده کل مرخصی کارمند به ساعت
	CreatedAt            time.Time    `json:"created_at"`
}

// PayrollSlip ساختار نهایی فیش حقوقی صادر شده برای هر ماه
type PayrollSlip struct {
	ID                 int       `json:"id"`
	EmployeeID         int       `json:"employee_id"`
	Year               int       `json:"year"`  // سال شمسی (مثلاً ۱۴۰۵)
	Month              int       `json:"month"` // ماه شمسی (۱ تا ۱۲)
	ExpectedWorkHours  float64   `json:"expected_work_hours"`
	ActualWorkHours    float64   `json:"actual_work_hours"`
	
	// اقلام درآمدی
	BaseSalary         int64     `json:"base_salary"`          // حقوق پایه کارکرد
	BonAllowance       int64     `json:"bon_allowance"`         // بن کارگری
	HousingAllowance   int64     `json:"housing_allowance"`     // حق مسکن
	MaritalAllowance   int64     `json:"marital_allowance"`     // حق تاهل
	ChildAllowance     int64     `json:"child_allowance"`       // حق اولاد
	SeniorityAllowance int64     `json:"seniority_allowance"`   // پایه سنوات
	OvertimeIncome     int64     `json:"overtime_income"`       // درآمد حاصل از اضافه‌کاری
	GrossEarnings      int64     `json:"gross_earnings"`        // جمع ناخالص درآمد
	
	// اقلام کسورات
	InsuranceDeduction int64     `json:"insurance_deduction"`   // سهم بیمه ۷ درصد کارگر
	LeaveDeficitHours  float64   `json:"leave_deficit_hours"`   // ساعت کسر کارکرد که از مرخصی کسر شده
	TotalDeductions    int64     `json:"total_deductions"`      // جمع کسورات
	
	NetPayout          int64     `json:"net_payout"`            // خالص دریافتی (قابل پرداخت)
	CreatedAt          time.Time `json:"created_at"`
}

// Constants values based on 1405 labor laws (from context)
const (
	DailyBaseWage1405       int64 = 5541850   // مزد روزانه قانون کار
	DailySeniority1405      int64 = 166667    // پایه سنوات روزانه
	MonthlyBon1405          int64 = 22000000  // بن کارگری ثابت
	MonthlyHousing1405      int64 = 30000000  // حق مسکن ثابت
	MonthlyMarital1405      int64 = 5000000   // حق تاهل ثابت
	MonthlyChildPerOne1405  int64 = 16625550  // حق اولاد ثابت به ازای هر فرزند
	MonthlyLeaveAccrual     float64 = 20.0    // مرخصی استحقاقی ماهیانه (۲۰ ساعت یا ۲.۵ روز)
)