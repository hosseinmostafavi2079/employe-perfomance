package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"shamsi_attendance/internal/attendance"
	"shamsi_attendance/internal/backup"
	"shamsi_attendance/internal/biometric"
	"shamsi_attendance/internal/database"
	"shamsi_attendance/internal/payroll"
	"shamsi_attendance/internal/project"

	"github.com/xuri/excelize/v2"
	"golang.org/x/crypto/bcrypt"
)

// ثابت مرخصی پیش‌فرض (در صورتی که در دیتابیس یافت نشود)
const MonthlyLeaveAccrual float64 = 20.0

// ==========================================
// 1. Helper Functions (توابع کمکی و فرمت‌بندی)
// ==========================================

func ParseDuration(input string) float64 {
	input = strings.TrimSpace(input)
	if input == "" {
		return 0
	}
	
	// پشتیبانی از اعداد منفی برای اصلاحات مرخصی
	isNegative := false
	if strings.HasPrefix(input, "-") {
		isNegative = true
		input = strings.TrimPrefix(input, "-")
	}

	if strings.Contains(input, ":") {
		parts := strings.Split(input, ":")
		if len(parts) == 2 {
			h, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			m, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			val := h + (m / 60.0)
			if isNegative {
				return -val
			}
			return val
		}
	}

	f, _ := strconv.ParseFloat(input, 64)
	if isNegative && f > 0 {
		return -f
	}
	return f
}

func FormatDuration(hours float64) string {
	if math.IsNaN(hours) || math.IsInf(hours, 0) {
		return "00:00"
	}
	sign := ""
	if hours < 0 {
		sign = "-"
		hours = -hours
	}
	h := int(hours)
	m := int(math.Round((hours - float64(h)) * 60))
	if m >= 60 {
		h += 1
		m -= 60
	}
	if m < 0 {
		m = 0
	}
	return fmt.Sprintf("%s%02d:%02d", sign, h, m)
}

func FormatCurrency(amount int64) string {
	str := strconv.FormatInt(amount, 10)
	n := len(str)
	if n <= 3 {
		return str
	}
	var result []byte
	for i, c := range str {
		if i > 0 && (n-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}

func getMonthName(m int) string {
	names := []string{"", "فروردین", "اردیبهشت", "خرداد", "تیر", "مرداد", "شهریور", "مهر", "آبان", "آذر", "دی", "بهمن", "اسفند"}
	if m >= 1 && m <= 12 {
		return names[m]
	}
	return "نامشخص"
}

// ==========================================
// 2. Data Structures (ساختارهای داده‌ای ویوها)
// ==========================================

type WorkLogView struct {
	ID             int
	EmployeeCode   string
	EmployeeName   string
	ProjectID      int
	ProjectName    string
	HoursSpent     float64
	FormattedHours string
	Description    string
	ShamsiDate     string
}

type AttendanceView struct {
	ID           int
	EmployeeCode string
	EmployeeName string
	CheckIn      string
	CheckOut     string
	Duration     string
	ShamsiDate   string
}

type EmployeeView struct {
	ID           int
	EmployeeCode string
	FullName     string
	Role         string
}

type ProjectView struct {
	ID   int
	Name string
}

type FinancialProfileView struct {
	EmployeeCode         string
	ContractType         string
	IsMarried            bool
	ChildCount           int
	EligibleForSeniority bool
	CustomOvertimeRate   int64
	HourlyRate           int64
	RemainingLeaveHours  float64
	FormattedLeave       string
	NationalCode         string
	PhoneNumber          string
	BankCardNumber       string
	ShebaNumber          string
	AvatarPath           string
}

type LiveStatusView struct {
	EmployeeCode string
	FullName     string
	IsPresent    bool
	CheckInTime  string
	AvatarPath   string
}

type PayslipView struct {
	ID                 int
	EmployeeCode       string
	EmployeeName       string
	Year               int
	Month              int
	MonthName          string
	ContractType       string
	ActualHours        string
	ExpectedHours      string
	OvertimeHours      string
	LeaveDeficitHours  string
	BaseSalary         string
	BonAllowance       string
	HousingAllowance   string
	MaritalAllowance   string
	ChildAllowance     string
	SeniorityAllowance string
	OvertimeIncome     string
	GrossEarnings      string
	InsuranceDeduction string
	TotalDeductions    string
	NetPayout          string
	LeaveBalance       string
	IsPublished        bool
	IsLeaveDeducted    bool
}

type ExpectedHourView struct {
	Year          int
	Month         int
	MonthName     string
	ExpectedHours string
}

type PayrollConstantView struct {
	Year                int
	DailyBaseWage       string
	DailySeniority      string
	MonthlyBon          string
	MonthlyHousing      string
	MonthlyMarital      string
	MonthlyChild        string
	MonthlyLeaveAccrual string
}

type ProjectAccessView struct {
	EmployeeCode string
	EmployeeName string
	ProjectNames string
}

type PageData struct {
	IsLoggedIn              bool
	CurrentDate             string
	TotalHours              float64
	TotalHoursFormatted     string
	Message                 string
	CurrentUser             string
	CurrentFullName         string
	CurrentRole             string
	CurrentAvatar           string
	SelectedFilter          string
	SelectedProjectFilter   string
	SelectedMonthFilter     string
	CurrentTab              string
	WorkLogs                []WorkLogView
	AttendanceLogs          []AttendanceView
	Employees               []EmployeeView
	Projects                []ProjectView
	EditLog                 *WorkLogView
	EditAttendanceLog       *AttendanceView
	TotalAttendanceMonthStr string
	TotalAttendanceDayStr   string
	SelectedProfile         *FinancialProfileView
	MyProfile               *FinancialProfileView
	LiveStatuses            []LiveStatusView
	PieLabelsJSON           template.JS
	PieDataJSON             template.JS
	BarLabelsJSON           template.JS
	BarDataJSON             template.JS
	RecentTasks             []WorkLogView
	ActiveProjects          []string
	SelectedMonthsMap       map[string]bool
	MyPayslips              []PayslipView
	AllPayslips             []PayslipView
	ExpectedHoursList       []ExpectedHourView
	PayrollConstants        []PayrollConstantView
	ProjectAccessList       []ProjectAccessView
}

// ==========================================
// 3. Session & Auth Management
// ==========================================

func setFlashMessage(w http.ResponseWriter, message string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "flash_message",
		Value:    url.QueryEscape(message),
		Path:     "/",
		HttpOnly: true,
		Expires:  time.Now().Add(10 * time.Second),
	})
}

func getFlashMessage(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie("flash_message")
	if err != nil {
		return ""
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "flash_message",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	decoded, _ := url.QueryUnescape(cookie.Value)
	return decoded
}

func getAuthenticatedUser(r *http.Request) (string, string) {
	cookie, err := r.Cookie("session_user")
	if err != nil {
		return "", ""
	}

	username, err := url.QueryUnescape(cookie.Value)
	if err != nil {
		return "", ""
	}
	username = strings.ToUpper(strings.TrimSpace(username))

	ctx := context.Background()
	var role string
	err = database.DB.QueryRow(ctx, "SELECT role FROM employees WHERE employee_code = $1;", username).Scan(&role)
	if err != nil {
		return "", ""
	}

	return username, role
}

// ==========================================
// 4. Server Initialization & Routes
// ==========================================

func main() {
	fmt.Println("==================================================")
	fmt.Println("راه‌اندازی سیستم جامع (ERP) منابع انسانی و دستمزد...")
	fmt.Println("==================================================")

	database.ConnectToDatabase()
	if database.DB != nil {
		defer database.DB.Close()
	}

	// 🚨 اجرای ماژول بکاپ اتوماتیک در پس‌زمینه بدون مسدود کردن سرور (Zero Downtime)
	backup.StartScheduledBackups()

	ctx := context.Background()

	err := os.MkdirAll("static/uploads/profiles", os.ModePerm)
	if err != nil {
		log.Printf("Warning: Failed to create uploads directory: %v", err)
	}

	errBio := biometric.InitWebAuthn()
	if errBio != nil {
		log.Printf("⚠️ هشدار: ماژول بیومتریک لود نشد: %v", errBio)
	}

	// بروزرسانی جداول پایگاه داده
	_, _ = database.DB.Exec(ctx, `ALTER TABLE employee_profiles ADD COLUMN IF NOT EXISTS national_code VARCHAR(10) DEFAULT '';`)
	_, _ = database.DB.Exec(ctx, `ALTER TABLE employee_profiles ADD COLUMN IF NOT EXISTS phone_number VARCHAR(11) DEFAULT '';`)
	_, _ = database.DB.Exec(ctx, `ALTER TABLE employee_profiles ADD COLUMN IF NOT EXISTS bank_card_number VARCHAR(16) DEFAULT '';`)
	_, _ = database.DB.Exec(ctx, `ALTER TABLE employee_profiles ADD COLUMN IF NOT EXISTS sheba_number VARCHAR(26) DEFAULT '';`)
	_, _ = database.DB.Exec(ctx, `ALTER TABLE employee_profiles ADD COLUMN IF NOT EXISTS avatar_path VARCHAR(255) DEFAULT '';`)

	_, _ = database.DB.Exec(ctx, `CREATE TABLE IF NOT EXISTS monthly_expected_hours (
		year INT, month INT, expected_hours FLOAT,
		PRIMARY KEY (year, month)
	);`)

	_, _ = database.DB.Exec(ctx, `CREATE TABLE IF NOT EXISTS payslips (
		id SERIAL PRIMARY KEY, employee_code VARCHAR(50), year INT, month INT, contract_type VARCHAR(20),
		actual_hours FLOAT, expected_hours FLOAT, overtime_hours FLOAT, 
		base_salary BIGINT, bon_allowance BIGINT, housing_allowance BIGINT, marital_allowance BIGINT, child_allowance BIGINT, 
		seniority_allowance BIGINT, overtime_income BIGINT, gross_earnings BIGINT, insurance_deduction BIGINT, 
		leave_deficit_hours FLOAT, total_deductions BIGINT, net_payout BIGINT, leave_balance FLOAT,
		is_published BOOLEAN, is_leave_deducted BOOLEAN DEFAULT false, created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(employee_code, year, month)
	);`)
	_, _ = database.DB.Exec(ctx, `ALTER TABLE payslips ADD COLUMN IF NOT EXISTS is_leave_deducted BOOLEAN DEFAULT false;`)

	_, _ = database.DB.Exec(ctx, `CREATE TABLE IF NOT EXISTS payroll_constants (
		year INT PRIMARY KEY,
		daily_base_wage BIGINT,
		daily_seniority BIGINT,
		monthly_bon BIGINT,
		monthly_housing BIGINT,
		monthly_marital BIGINT,
		monthly_child BIGINT,
		monthly_leave_accrual FLOAT
	);`)

	_, _ = database.DB.Exec(ctx, `CREATE TABLE IF NOT EXISTS employee_projects (
		employee_code VARCHAR(50),
		project_id INT,
		PRIMARY KEY (employee_code, project_id)
	);`)

	_, _ = database.DB.Exec(ctx, `INSERT INTO payroll_constants 
		(year, daily_base_wage, daily_seniority, monthly_bon, monthly_housing, monthly_marital, monthly_child, monthly_leave_accrual) 
		VALUES (1405, 5541850, 166667, 22000000, 30000000, 5000000, 16625550, 20.0) 
		ON CONFLICT DO NOTHING;`)

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	http.HandleFunc("/", handleDashboard)
	http.HandleFunc("/login", handleLogin)
	http.HandleFunc("/logout", handleLogout)
	http.HandleFunc("/checkin", handleCheckIn)
	http.HandleFunc("/checkout", handleCheckOut)
	http.HandleFunc("/logwork", handleLogWork)
	http.HandleFunc("/manual-attendance", handleManualAttendance)
	http.HandleFunc("/edit-worklog", handleEditWorkLog)
	http.HandleFunc("/delete-worklog", handleDeleteWorkLog)
	http.HandleFunc("/delete-attendance", handleDeleteAttendance)
	http.HandleFunc("/export", handleExportExcel)

	http.HandleFunc("/update-my-profile", handleUpdateMyProfile)

	http.HandleFunc("/admin/save-payroll-constants", handleSavePayrollConstants)
	http.HandleFunc("/admin/set-expected-hours", handleSetExpectedHours)
	http.HandleFunc("/admin/payroll/save-profile", handleSavePayrollProfile)
	http.HandleFunc("/admin/payroll/issue", handleIssuePayroll)
	http.HandleFunc("/admin/payroll/finalize", handleFinalizePayslip)
	http.HandleFunc("/admin/payroll/delete-payslip", handleDeletePayslip)
	http.HandleFunc("/admin/assign-projects", handleAssignProjects)

	http.HandleFunc("/admin/add-employee", handleAddEmployee)
	http.HandleFunc("/admin/edit-employee", handleEditEmployee)
	http.HandleFunc("/admin/delete-employee", handleDeleteEmployee)
	http.HandleFunc("/admin/create-project", handleCreateProject)
	http.HandleFunc("/admin/edit-project", handleEditProject)
	http.HandleFunc("/admin/delete-project", handleDeleteProject)

	http.HandleFunc("/biometric/register/begin", biometric.HandleRegisterBegin)
	http.HandleFunc("/biometric/register/finish", biometric.HandleRegisterFinish)
	http.HandleFunc("/biometric/login/begin", biometric.HandleLoginBegin)
	http.HandleFunc("/biometric/login/finish", biometric.HandleLoginFinish)

	// <--- روت مربوط به بکاپ دستی اضافه شد
	http.HandleFunc("/admin/backup/manual", handleManualBackup)

	fmt.Println("🚀 وب‌سرور هوشمند با موفقیت روی پورت 8085 روشن شد!")
	log.Fatal(http.ListenAndServe(":8085", nil))
}

// ==========================================
// 5. Application Handlers 
// ==========================================

func handleAssignProjects(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" {
		return
	}

	if r.Method == http.MethodPost {
		ctx := context.Background()
		err := r.ParseForm()
		if err != nil {
			setFlashMessage(w, "❌ خطا در پردازش اطلاعات فرم")
			http.Redirect(w, r, "/?tab=projects", http.StatusSeeOther)
			return
		}

		empCode := strings.ToUpper(strings.TrimSpace(r.FormValue("employee_code")))
		projectIDs := r.Form["project_ids"]

		if empCode != "" {
			_, errDelete := database.DB.Exec(ctx, "DELETE FROM employee_projects WHERE employee_code = $1;", empCode)
			if errDelete != nil {
				setFlashMessage(w, "❌ خطا در بروزرسانی دسترسی‌ها")
				http.Redirect(w, r, "/?tab=projects", http.StatusSeeOther)
				return
			}

			for _, pidStr := range projectIDs {
				pid, errParse := strconv.Atoi(pidStr)
				if errParse == nil {
					database.DB.Exec(ctx, "INSERT INTO employee_projects (employee_code, project_id) VALUES ($1, $2);", empCode, pid)
				}
			}
			setFlashMessage(w, "✅ دسترسی پروژه‌ها با موفقیت بروزرسانی شد.")
		}
	}
	http.Redirect(w, r, "/?tab=projects", http.StatusSeeOther)
}

func handleIssuePayroll(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" {
		return
	}

	if r.Method == http.MethodPost {
		ctx := context.Background()
		empCode := strings.ToUpper(strings.TrimSpace(r.FormValue("employee_code")))
		year, _ := strconv.Atoi(r.FormValue("year"))
		month, _ := strconv.Atoi(r.FormValue("month"))
		overtimeHours := ParseDuration(r.FormValue("overtime_hours"))

		isPublished := r.FormValue("is_published") == "true"

		var currentIsPublished bool
		errCheck := database.DB.QueryRow(ctx, "SELECT is_published FROM payslips WHERE employee_code=$1 AND year=$2 AND month=$3", empCode, year, month).Scan(&currentIsPublished)
		if errCheck == nil && currentIsPublished {
			setFlashMessage(w, "❌ این فیش قبلاً تایید نهایی شده است. در صورت نیاز به ویرایش، ابتدا از جدول آرشیو در پایین صفحه آن را ابطال کنید.")
			http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
			return
		}

		var expectedHours float64
		errExp := database.DB.QueryRow(ctx, "SELECT expected_hours FROM monthly_expected_hours WHERE year=$1 AND month=$2", year, month).Scan(&expectedHours)
		if errExp != nil {
			setFlashMessage(w, "❌ خطا: لطفاً ابتدا در پنل مدیریت، ساعت موظفی این ماه را تعریف کنید.")
			http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
			return
		}

		var actualHours float64
		monthStr := fmt.Sprintf("%04d/%02d", year, month)
		errAct := database.DB.QueryRow(ctx, "SELECT COALESCE(SUM(hours_spent), 0) FROM work_logs WHERE employee_code=$1 AND shamsi_date LIKE $2", empCode, monthStr+"%").Scan(&actualHours)
		if errAct != nil {
			actualHours = 0
		}

		slip, err := payroll.IssueMonthlyPayroll(ctx, empCode, year, month, actualHours, expectedHours, overtimeHours)
		if err != nil {
			setFlashMessage(w, "❌ خطا: "+err.Error())
			http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
			return
		}

		var currentLeave float64
		errLeave := database.DB.QueryRow(ctx, "SELECT remaining_leave_hours FROM employee_profiles WHERE employee_code = $1", empCode).Scan(&currentLeave)
		if errLeave != nil {
			currentLeave = 0
		}

		var leaveMsg string
		newBalance := currentLeave
		isLeaveDeducted := false

		if isPublished && slip.ContractType == "REGULAR" {
			var cLeaveAccrual float64
			errAcc := database.DB.QueryRow(ctx, "SELECT monthly_leave_accrual FROM payroll_constants WHERE year=$1", year).Scan(&cLeaveAccrual)
			if errAcc != nil {
				cLeaveAccrual = MonthlyLeaveAccrual
			}

			newBalance = currentLeave + cLeaveAccrual - slip.LeaveDeficitHours
			database.DB.Exec(ctx, "UPDATE employee_profiles SET remaining_leave_hours=$1 WHERE employee_code=$2", newBalance, empCode)
			isLeaveDeducted = true

			if slip.LeaveDeficitHours > 0 {
				leaveMsg = fmt.Sprintf(" | 🏖️ کسر غیبت: %s (مانده جدید: %s)", FormatDuration(slip.LeaveDeficitHours), FormatDuration(newBalance))
			} else {
				leaveMsg = fmt.Sprintf(" | 🏖️ ذخیره مرخصی (مانده جدید: %s)", FormatDuration(newBalance))
			}
		}

		querySave := `INSERT INTO payslips (
			employee_code, year, month, contract_type, actual_hours, expected_hours, overtime_hours,
			base_salary, bon_allowance, housing_allowance, marital_allowance, child_allowance,
			seniority_allowance, overtime_income, gross_earnings, insurance_deduction,
			leave_deficit_hours, total_deductions, net_payout, leave_balance, is_published, is_leave_deducted
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
		ON CONFLICT (employee_code, year, month) DO UPDATE SET 
			contract_type=EXCLUDED.contract_type, actual_hours=EXCLUDED.actual_hours, expected_hours=EXCLUDED.expected_hours, 
			overtime_hours=EXCLUDED.overtime_hours, base_salary=EXCLUDED.base_salary, bon_allowance=EXCLUDED.bon_allowance, 
			housing_allowance=EXCLUDED.housing_allowance, marital_allowance=EXCLUDED.marital_allowance, child_allowance=EXCLUDED.child_allowance, 
			seniority_allowance=EXCLUDED.seniority_allowance, overtime_income=EXCLUDED.overtime_income, gross_earnings=EXCLUDED.gross_earnings, 
			insurance_deduction=EXCLUDED.insurance_deduction, leave_deficit_hours=EXCLUDED.leave_deficit_hours, total_deductions=EXCLUDED.total_deductions, 
			net_payout=EXCLUDED.net_payout, leave_balance=EXCLUDED.leave_balance, is_published=EXCLUDED.is_published, is_leave_deducted=EXCLUDED.is_leave_deducted;`

		_, errSave := database.DB.Exec(ctx, querySave,
			empCode, year, month, string(slip.ContractType), slip.ActualWorkHours, slip.ExpectedWorkHours, slip.OvertimeHours,
			slip.BaseSalary, slip.BonAllowance, slip.HousingAllowance, slip.MaritalAllowance, slip.ChildAllowance,
			slip.SeniorityAllowance, slip.OvertimeIncome, slip.GrossEarnings, slip.InsuranceDeduction,
			slip.LeaveDeficitHours, slip.TotalDeductions, slip.NetPayout, newBalance, isPublished, isLeaveDeducted)

		if errSave != nil {
			setFlashMessage(w, "❌ خطا در ذخیره فیش: "+errSave.Error())
		} else {
			pubStatus := "پیش‌نویس فیش ثبت شد (جهت تایید نهایی به آرشیو مراجعه کنید)"
			if isPublished {
				pubStatus = "فیش نهایی صادر و مرخصی‌ها با موفقیت اعمال گردید"
			}
			setFlashMessage(w, fmt.Sprintf("✅ %s. خالص پرداختی: %s ریال %s", pubStatus, FormatCurrency(slip.NetPayout), leaveMsg))
		}
	}
	http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
}

func handleFinalizePayslip(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" {
		return
	}
	payslipID := r.URL.Query().Get("id")
	ctx := context.Background()

	var empCode, contractType string
	var leaveDeficit float64
	var isDeducted bool
	var year int
	err := database.DB.QueryRow(ctx, "SELECT employee_code, contract_type, leave_deficit_hours, is_leave_deducted, year FROM payslips WHERE id=$1", payslipID).Scan(&empCode, &contractType, &leaveDeficit, &isDeducted, &year)

	if err == nil && !isDeducted {
		var newBalance float64 = 0
		if contractType == "REGULAR" {
			var currentLeave float64
			errLeave := database.DB.QueryRow(ctx, "SELECT remaining_leave_hours FROM employee_profiles WHERE employee_code=$1", empCode).Scan(&currentLeave)
			if errLeave != nil {
				currentLeave = 0
			}

			var cLeaveAccrual float64
			errAccrual := database.DB.QueryRow(ctx, "SELECT monthly_leave_accrual FROM payroll_constants WHERE year=$1", year).Scan(&cLeaveAccrual)
			if errAccrual != nil {
				cLeaveAccrual = MonthlyLeaveAccrual
			}

			newBalance = currentLeave + cLeaveAccrual - leaveDeficit
			_, errUpdateProf := database.DB.Exec(ctx, "UPDATE employee_profiles SET remaining_leave_hours=$1 WHERE employee_code=$2", newBalance, empCode)
			if errUpdateProf != nil {
				setFlashMessage(w, "❌ خطا در بروزرسانی مرخصی")
				http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
				return
			}
		}

		_, errUpdateSlip := database.DB.Exec(ctx, "UPDATE payslips SET is_published=true, is_leave_deducted=true, leave_balance=$1 WHERE id=$2", newBalance, payslipID)
		if errUpdateSlip == nil {
			setFlashMessage(w, "✅ فیش تایید نهایی شد، مرخصی‌ها با موفقیت اعمال گردید.")
		} else {
			setFlashMessage(w, "❌ خطا در تایید فیش")
		}
	} else if err != nil {
		setFlashMessage(w, "❌ فیش یافت نشد")
	} else {
		setFlashMessage(w, "❌ این فیش قبلاً اعمال شده است")
	}
	http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
}

func handleDeletePayslip(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" {
		return
	}

	payslipID := r.URL.Query().Get("id")
	ctx := context.Background()

	var empCode, contractType string
	var leaveDeficit float64
	var isDeducted bool
	var year int

	err := database.DB.QueryRow(ctx, "SELECT employee_code, contract_type, leave_deficit_hours, is_leave_deducted, year FROM payslips WHERE id=$1", payslipID).Scan(&empCode, &contractType, &leaveDeficit, &isDeducted, &year)

	if err == nil {
		if isDeducted && contractType == "REGULAR" {
			var cLeaveAccrual float64
			errAccrual := database.DB.QueryRow(ctx, "SELECT monthly_leave_accrual FROM payroll_constants WHERE year=$1", year).Scan(&cLeaveAccrual)
			if errAccrual != nil {
				cLeaveAccrual = MonthlyLeaveAccrual
			}

			_, errRev := database.DB.Exec(ctx, "UPDATE employee_profiles SET remaining_leave_hours = remaining_leave_hours - $1 + $2 WHERE employee_code=$3", cLeaveAccrual, leaveDeficit, empCode)
			if errRev != nil {
				setFlashMessage(w, "❌ خطا در بازگردانی مرخصی")
				http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
				return
			}
		}

		_, errDel := database.DB.Exec(ctx, "DELETE FROM payslips WHERE id=$1", payslipID)
		if errDel == nil {
			setFlashMessage(w, "✅ فیش ابطال گردید و حساب مرخصی پرسنل به حالت قبل بازگشت.")
		} else {
			setFlashMessage(w, "❌ خطا در حذف فیش")
		}
	} else {
		setFlashMessage(w, "❌ فیش یافت نشد")
	}

	http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
}

func handleSavePayrollConstants(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" {
		return
	}

	if r.Method == http.MethodPost {
		year, errYear := strconv.Atoi(r.FormValue("year"))
		dBase, errBase := strconv.ParseInt(r.FormValue("daily_base_wage"), 10, 64)
		dSen, errSen := strconv.ParseInt(r.FormValue("daily_seniority"), 10, 64)
		mBon, errBon := strconv.ParseInt(r.FormValue("monthly_bon"), 10, 64)
		mHous, errHous := strconv.ParseInt(r.FormValue("monthly_housing"), 10, 64)
		mMar, errMar := strconv.ParseInt(r.FormValue("monthly_marital"), 10, 64)
		mChild, errChild := strconv.ParseInt(r.FormValue("monthly_child"), 10, 64)
		mLeave, errLeave := strconv.ParseFloat(r.FormValue("monthly_leave_accrual"), 64)

		if errYear != nil || errBase != nil || errSen != nil || errBon != nil || errHous != nil || errMar != nil || errChild != nil || errLeave != nil {
			setFlashMessage(w, "❌ خطا در فرمت مقادیر وارد شده")
			http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
			return
		}

		query := `INSERT INTO payroll_constants 
			(year, daily_base_wage, daily_seniority, monthly_bon, monthly_housing, monthly_marital, monthly_child, monthly_leave_accrual) 
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (year) DO UPDATE SET 
			daily_base_wage=EXCLUDED.daily_base_wage, daily_seniority=EXCLUDED.daily_seniority, monthly_bon=EXCLUDED.monthly_bon, 
			monthly_housing=EXCLUDED.monthly_housing, monthly_marital=EXCLUDED.monthly_marital, monthly_child=EXCLUDED.monthly_child, 
			monthly_leave_accrual=EXCLUDED.monthly_leave_accrual;`

		_, err := database.DB.Exec(context.Background(), query, year, dBase, dSen, mBon, mHous, mMar, mChild, mLeave)
		if err != nil {
			setFlashMessage(w, "❌ خطا در ذخیره مقادیر پایه: "+err.Error())
		} else {
			setFlashMessage(w, "✅ مقادیر پایه حقوق و دستمزد در سیستم ذخیره شد.")
		}
	}
	http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
}

func handleSetExpectedHours(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" {
		return
	}

	if r.Method == http.MethodPost {
		year, errYear := strconv.Atoi(r.FormValue("year"))
		month, errMonth := strconv.Atoi(r.FormValue("month"))
		hours := ParseDuration(r.FormValue("expected_hours"))

		if errYear != nil || errMonth != nil {
			setFlashMessage(w, "❌ مقادیر سال و ماه نامعتبر است")
			http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
			return
		}

		query := `INSERT INTO monthly_expected_hours (year, month, expected_hours) VALUES ($1, $2, $3) 
		          ON CONFLICT (year, month) DO UPDATE SET expected_hours = EXCLUDED.expected_hours;`
		_, err := database.DB.Exec(context.Background(), query, year, month, hours)
		if err != nil {
			setFlashMessage(w, "❌ خطا در ثبت ساعت موظفی")
		} else {
			setFlashMessage(w, "✅ ساعت موظفی با موفقیت در تقویم سازمانی ثبت شد.")
		}
	}
	http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
}

func handleUpdateMyProfile(w http.ResponseWriter, r *http.Request) {
	username, _ := getAuthenticatedUser(r)
	if username == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if r.Method == http.MethodPost {
		ctx := context.Background()

		err := r.ParseMultipartForm(2 << 20)
		if err != nil {
			setFlashMessage(w, "❌ خطا: حجم فایل آپلودی بیش از ۲ مگابایت است.")
			http.Redirect(w, r, "/?tab=profile", http.StatusSeeOther)
			return
		}

		fullName := strings.TrimSpace(r.FormValue("full_name"))
		password := r.FormValue("password")

		if fullName != "" {
			_, errName := database.DB.Exec(ctx, "UPDATE employees SET full_name = $1 WHERE employee_code = $2;", fullName, username)
			if errName != nil {
				log.Printf("Error updating name: %v", errName)
			}
		}

		if password != "" {
			hash, errHash := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if errHash == nil {
				_, errPw := database.DB.Exec(ctx, "UPDATE employees SET password = $1 WHERE employee_code = $2;", string(hash), username)
				if errPw != nil {
					log.Printf("Error updating password: %v", errPw)
				}
			}
		}

		file, handler, errFile := r.FormFile("avatar_file")
		if errFile == nil {
			defer file.Close()
			ext := strings.ToLower(filepath.Ext(handler.Filename))

			if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
				fileName := username + ext
				filePath := filepath.Join("static/uploads/profiles", fileName)
				dst, errCreate := os.Create(filePath)

				if errCreate == nil {
					io.Copy(dst, file)
					dst.Close()

					dbPath := "/" + strings.ReplaceAll(filePath, "\\", "/")
					res, errDb := database.DB.Exec(ctx, "UPDATE employee_profiles SET avatar_path = $1 WHERE employee_code = $2;", dbPath, username)

					if errDb == nil {
						if res.RowsAffected() == 0 {
							database.DB.Exec(ctx, "INSERT INTO employee_profiles (employee_code, contract_type, avatar_path) VALUES ($1, 'REGULAR', $2) ON CONFLICT DO NOTHING;", username, dbPath)
						}
					}
				}
			} else {
				setFlashMessage(w, "❌ فقط فرمت‌های JPG و PNG مجاز هستند.")
				http.Redirect(w, r, "/?tab=profile", http.StatusSeeOther)
				return
			}
		}

		setFlashMessage(w, "✅ اطلاعات حساب کاربری شما با موفقیت بروزرسانی شد.")
	}
	http.Redirect(w, r, "/?tab=profile", http.StatusSeeOther)
}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	username, role := getAuthenticatedUser(r)
	flashMsg := getFlashMessage(w, r)

	if username == "" {
		tmpl, err := template.ParseFiles("templates/login.html")
		if err != nil {
			http.Error(w, "Error parsing login template", http.StatusInternalServerError)
			return
		}
		errExec := tmpl.Execute(w, PageData{IsLoggedIn: false, Message: flashMsg, CurrentDate: attendance.GetCurrentShamsiDate()})
		if errExec != nil {
			log.Printf("Template execution error: %v", errExec)
		}
		return
	}

	var currentFullName string
	errName := database.DB.QueryRow(ctx, "SELECT full_name FROM employees WHERE employee_code = $1;", username).Scan(&currentFullName)
	if errName != nil || currentFullName == "" {
		currentFullName = username
	}

	var currentAvatar string
	errAvatar := database.DB.QueryRow(ctx, "SELECT avatar_path FROM employee_profiles WHERE employee_code = $1;", username).Scan(&currentAvatar)
	if errAvatar != nil {
		currentAvatar = ""
	}

	var myProfile *FinancialProfileView = nil
	var mypf FinancialProfileView
	myQuery := `SELECT contract_type, is_married, child_count, eligible_for_seniority, custom_overtime_rate, hourly_rate, remaining_leave_hours, COALESCE(national_code, ''), COALESCE(phone_number, ''), COALESCE(bank_card_number, ''), COALESCE(sheba_number, ''), COALESCE(avatar_path, '') FROM employee_profiles WHERE employee_code = $1;`
	errMyPf := database.DB.QueryRow(ctx, myQuery, username).Scan(
		&mypf.ContractType, &mypf.IsMarried, &mypf.ChildCount, &mypf.EligibleForSeniority, 
		&mypf.CustomOvertimeRate, &mypf.HourlyRate, &mypf.RemainingLeaveHours, 
		&mypf.NationalCode, &mypf.PhoneNumber, &mypf.BankCardNumber, &mypf.ShebaNumber, &mypf.AvatarPath,
	)

	if errMyPf == nil {
		mypf.FormattedLeave = FormatDuration(mypf.RemainingLeaveHours)
		myProfile = &mypf
	} else {
		myProfile = &FinancialProfileView{EmployeeCode: username, ContractType: "REGULAR", FormattedLeave: "00:00"}
	}

	psQuery := `
		SELECT p.id, p.employee_code, e.full_name, p.year, p.month, p.contract_type, p.actual_hours, p.expected_hours, p.overtime_hours, p.leave_deficit_hours,
		       p.base_salary, p.bon_allowance, p.housing_allowance, p.marital_allowance, p.child_allowance, p.seniority_allowance,
		       p.overtime_income, p.gross_earnings, p.insurance_deduction, p.total_deductions, p.net_payout, p.leave_balance, p.is_published, p.is_leave_deducted
		FROM payslips p
		JOIN employees e ON p.employee_code = e.employee_code`

	var myPayslips []PayslipView
	myPsRows, errMyPs := database.DB.Query(ctx, psQuery+" WHERE p.employee_code = $1 AND p.is_published = true ORDER BY p.year DESC, p.month DESC", username)
	if errMyPs == nil && myPsRows != nil {
		defer myPsRows.Close()
		for myPsRows.Next() {
			var pv PayslipView
			var a, e, o, ld, l float64
			var bs, bon, hs, ma, ch, sen, oi, gr, ins, td, net int64

			errScan := myPsRows.Scan(
				&pv.ID, &pv.EmployeeCode, &pv.EmployeeName, &pv.Year, &pv.Month, &pv.ContractType, &a, &e, &o, &ld,
				&bs, &bon, &hs, &ma, &ch, &sen, &oi, &gr, &ins, &td, &net, &l, &pv.IsPublished, &pv.IsLeaveDeducted,
			)

			if errScan == nil {
				pv.MonthName = getMonthName(pv.Month)
				pv.ActualHours = FormatDuration(a)
				pv.ExpectedHours = FormatDuration(e)
				pv.OvertimeHours = FormatDuration(o)
				pv.LeaveDeficitHours = FormatDuration(ld)
				pv.BaseSalary = FormatCurrency(bs)
				pv.BonAllowance = FormatCurrency(bon)
				pv.HousingAllowance = FormatCurrency(hs)
				pv.MaritalAllowance = FormatCurrency(ma)
				pv.ChildAllowance = FormatCurrency(ch)
				pv.SeniorityAllowance = FormatCurrency(sen)
				pv.OvertimeIncome = FormatCurrency(oi)
				pv.GrossEarnings = FormatCurrency(gr)
				pv.InsuranceDeduction = FormatCurrency(ins)
				pv.TotalDeductions = FormatCurrency(td)
				pv.NetPayout = FormatCurrency(net)
				pv.LeaveBalance = FormatDuration(l)
				myPayslips = append(myPayslips, pv)
			}
		}
	}

	var allPayslips []PayslipView
	if role == "ADMIN" {
		allPsRows, errAllPs := database.DB.Query(ctx, psQuery+" ORDER BY p.year DESC, p.month DESC, p.id DESC")
		if errAllPs == nil && allPsRows != nil {
			defer allPsRows.Close()
			for allPsRows.Next() {
				var pv PayslipView
				var a, e, o, ld, l float64
				var bs, bon, hs, ma, ch, sen, oi, gr, ins, td, net int64

				errScan := allPsRows.Scan(
					&pv.ID, &pv.EmployeeCode, &pv.EmployeeName, &pv.Year, &pv.Month, &pv.ContractType, &a, &e, &o, &ld,
					&bs, &bon, &hs, &ma, &ch, &sen, &oi, &gr, &ins, &td, &net, &l, &pv.IsPublished, &pv.IsLeaveDeducted,
				)

				if errScan == nil {
					pv.MonthName = getMonthName(pv.Month)
					pv.ActualHours = FormatDuration(a)
					pv.ExpectedHours = FormatDuration(e)
					pv.OvertimeHours = FormatDuration(o)
					pv.LeaveDeficitHours = FormatDuration(ld)
					pv.BaseSalary = FormatCurrency(bs)
					pv.BonAllowance = FormatCurrency(bon)
					pv.HousingAllowance = FormatCurrency(hs)
					pv.MaritalAllowance = FormatCurrency(ma)
					pv.ChildAllowance = FormatCurrency(ch)
					pv.SeniorityAllowance = FormatCurrency(sen)
					pv.OvertimeIncome = FormatCurrency(oi)
					pv.GrossEarnings = FormatCurrency(gr)
					pv.InsuranceDeduction = FormatCurrency(ins)
					pv.TotalDeductions = FormatCurrency(td)
					pv.NetPayout = FormatCurrency(net)
					pv.LeaveBalance = FormatDuration(l)
					allPayslips = append(allPayslips, pv)
				}
			}
		}
	}

	var expectedHoursList []ExpectedHourView
	var payrollConstants []PayrollConstantView
	var projectAccessList []ProjectAccessView

	if role == "ADMIN" {
		ehRows, errEh := database.DB.Query(ctx, "SELECT year, month, expected_hours FROM monthly_expected_hours ORDER BY year DESC, month DESC")
		if errEh == nil && ehRows != nil {
			defer ehRows.Close()
			for ehRows.Next() {
				var ev ExpectedHourView
				var h float64
				errScan := ehRows.Scan(&ev.Year, &ev.Month, &h)
				if errScan == nil {
					ev.MonthName = getMonthName(ev.Month)
					ev.ExpectedHours = FormatDuration(h)
					expectedHoursList = append(expectedHoursList, ev)
				}
			}
		}

		cRows, errC := database.DB.Query(ctx, "SELECT year, daily_base_wage, daily_seniority, monthly_bon, monthly_housing, monthly_marital, monthly_child, monthly_leave_accrual FROM payroll_constants ORDER BY year DESC")
		if errC == nil && cRows != nil {
			defer cRows.Close()
			for cRows.Next() {
				var cv PayrollConstantView
				var dBase, dSen, mBon, mHous, mMar, mChild int64
				var mLeave float64
				errScan := cRows.Scan(&cv.Year, &dBase, &dSen, &mBon, &mHous, &mMar, &mChild, &mLeave)

				if errScan == nil {
					cv.DailyBaseWage = FormatCurrency(dBase)
					cv.DailySeniority = FormatCurrency(dSen)
					cv.MonthlyBon = FormatCurrency(mBon)
					cv.MonthlyHousing = FormatCurrency(mHous)
					cv.MonthlyMarital = FormatCurrency(mMar)
					cv.MonthlyChild = FormatCurrency(mChild)
					cv.MonthlyLeaveAccrual = fmt.Sprintf("%.1f", mLeave)

					payrollConstants = append(payrollConstants, cv)
				}
			}
		}

		paQ := `
			SELECT e.employee_code, e.full_name, COALESCE(STRING_AGG(p.name, '، '), 'بدون دسترسی')
			FROM employees e
			LEFT JOIN employee_projects ep ON e.employee_code = ep.employee_code
			LEFT JOIN projects p ON ep.project_id = p.id
			WHERE e.role != 'ADMIN'
			GROUP BY e.employee_code, e.full_name
			ORDER BY e.full_name ASC;`
		paRows, errPa := database.DB.Query(ctx, paQ)
		if errPa == nil && paRows != nil {
			defer paRows.Close()
			for paRows.Next() {
				var pav ProjectAccessView
				errScan := paRows.Scan(&pav.EmployeeCode, &pav.EmployeeName, &pav.ProjectNames)
				if errScan == nil {
					projectAccessList = append(projectAccessList, pav)
				}
			}
		}
	}

	errForm := r.ParseForm()
	if errForm != nil {
		log.Printf("Form parse error: %v", errForm)
	}

	editIDParam := r.FormValue("edit_id")
	editAttIDParam := r.FormValue("edit_attendance_id")
	editFinancialCode := strings.ToUpper(strings.TrimSpace(r.FormValue("edit_financial_code")))
	filterEmployee := strings.ToUpper(strings.TrimSpace(r.FormValue("filter_employee")))
	filterProject := r.FormValue("filter_project")
	filterMonth := r.FormValue("filter_month")
	filterMonths := r.Form["filter_months"]
	tabParam := r.FormValue("tab")

	if len(filterMonths) == 0 && filterMonth != "" {
		filterMonths = append(filterMonths, filterMonth)
	}

	currentTab := "attendance"
	if tabParam == "" {
		if role == "ADMIN" {
			currentTab = "dashboard"
		} else {
			currentTab = "attendance"
		}
	} else {
		currentTab = tabParam
	}

	var liveStatuses []LiveStatusView
	pieLabels, barLabels, activeProjects := []string{}, []string{}, []string{}
	pieData, barData := []float64{}, []float64{}
	var recentTasks []WorkLogView

	selectedMonthsMap := make(map[string]bool)
	for _, m := range filterMonths {
		selectedMonthsMap[m] = true
	}

	todayStrDate := attendance.GetCurrentShamsiDate()
	yearPrefix := ""
	if len(todayStrDate) >= 4 {
		yearPrefix = todayStrDate[0:4]
	}

	if role == "ADMIN" && currentTab == "dashboard" {
		lsQuery := `
			SELECT e.employee_code, e.full_name, 
			       (SELECT check_in FROM attendance a WHERE a.employee_code = e.employee_code AND a.shamsi_date = $1 ORDER BY id DESC LIMIT 1), 
			       (SELECT check_out FROM attendance a WHERE a.employee_code = e.employee_code AND a.shamsi_date = $1 ORDER BY id DESC LIMIT 1), 
			       COALESCE(p.avatar_path, '') 
			FROM employees e 
			LEFT JOIN employee_profiles p ON e.employee_code = p.employee_code 
			ORDER BY e.role ASC, e.full_name ASC`

		lsRows, errLs := database.DB.Query(ctx, lsQuery, todayStrDate)
		if errLs == nil && lsRows != nil {
			defer lsRows.Close()
			for lsRows.Next() {
				var lsv LiveStatusView
				var tIn, tOut *time.Time
				errScan := lsRows.Scan(&lsv.EmployeeCode, &lsv.FullName, &tIn, &tOut, &lsv.AvatarPath)

				if errScan == nil {
					if tIn != nil && tOut == nil {
						lsv.IsPresent = true
						lsv.CheckInTime = tIn.In(time.Local).Format("15:04")
					} else {
						lsv.IsPresent = false
						lsv.CheckInTime = "--:--"
					}
					liveStatuses = append(liveStatuses, lsv)
				}
			}
		}

		var args []interface{}
		args = append(args, yearPrefix)
		argCounter := 2

		empCondition, monthCondition := "", ""

		if filterEmployee != "" {
			empCondition = fmt.Sprintf("AND w.employee_code = $%d", argCounter)
			args = append(args, filterEmployee)
			argCounter++
		}

		if len(filterMonths) > 0 {
			placeholders := []string{}
			for _, m := range filterMonths {
				placeholders = append(placeholders, fmt.Sprintf("$%d", argCounter))
				args = append(args, m)
				argCounter++
			}
			monthCondition = fmt.Sprintf("AND split_part(w.shamsi_date, '/', 2) IN (%s)", strings.Join(placeholders, ","))
		}

		pieQ := fmt.Sprintf(`SELECT p.name, COALESCE(SUM(w.hours_spent), 0) FROM work_logs w JOIN projects p ON w.project_id = p.id WHERE w.shamsi_date LIKE $1 || '%%' %s %s GROUP BY p.name`, empCondition, monthCondition)
		pRows, errP := database.DB.Query(ctx, pieQ, args...)
		if errP == nil && pRows != nil {
			defer pRows.Close()
			for pRows.Next() {
				var pName string
				var h float64
				errScan := pRows.Scan(&pName, &h)
				if errScan == nil {
					pieLabels = append(pieLabels, pName)
					pieData = append(pieData, math.Round(h*100)/100)
				}
			}
		}

		barQ := fmt.Sprintf(`SELECT split_part(w.shamsi_date, '/', 2) as month, COALESCE(SUM(w.hours_spent), 0) FROM work_logs w WHERE w.shamsi_date LIKE $1 || '%%' %s %s GROUP BY month ORDER BY month ASC`, empCondition, monthCondition)
		bRows, errB := database.DB.Query(ctx, barQ, args...)
		if errB == nil && bRows != nil {
			defer bRows.Close()
			for bRows.Next() {
				var m string
				var h float64
				errScan := bRows.Scan(&m, &h)
				if errScan == nil {
					barLabels = append(barLabels, "ماه "+m)
					barData = append(barData, math.Round(h*100)/100)
				}
			}
		}

		if filterEmployee != "" {
			taskQ := fmt.Sprintf(`SELECT w.id, w.employee_code, e.full_name, w.project_id, p.name, COALESCE(w.hours_spent, 0), COALESCE(w.description, ''), w.shamsi_date FROM work_logs w JOIN projects p ON w.project_id = p.id JOIN employees e ON w.employee_code = e.employee_code WHERE w.shamsi_date LIKE $1 || '%%' %s %s ORDER BY w.shamsi_date DESC, w.id DESC LIMIT 8`, empCondition, monthCondition)
			tRows, errT := database.DB.Query(ctx, taskQ, args...)
			if errT == nil && tRows != nil {
				defer tRows.Close()
				projMap := make(map[string]bool)
				for tRows.Next() {
					var wl WorkLogView
					errScan := tRows.Scan(&wl.ID, &wl.EmployeeCode, &wl.EmployeeName, &wl.ProjectID, &wl.ProjectName, &wl.HoursSpent, &wl.Description, &wl.ShamsiDate)
					if errScan == nil {
						wl.FormattedHours = FormatDuration(wl.HoursSpent)
						recentTasks = append(recentTasks, wl)

						if !projMap[wl.ProjectName] {
							projMap[wl.ProjectName] = true
							activeProjects = append(activeProjects, wl.ProjectName)
						}
					}
				}
			}
		}
	}

	plJSON, _ := json.Marshal(pieLabels)
	pdJSON, _ := json.Marshal(pieData)
	blJSON, _ := json.Marshal(barLabels)
	bdJSON, _ := json.Marshal(barData)

	var selectedProfile *FinancialProfileView = nil
	if editFinancialCode != "" && role == "ADMIN" {
		var pf FinancialProfileView
		pf.EmployeeCode = editFinancialCode
		queryProfile := `SELECT contract_type, is_married, child_count, eligible_for_seniority, custom_overtime_rate, hourly_rate, remaining_leave_hours, COALESCE(national_code, ''), COALESCE(phone_number, ''), COALESCE(bank_card_number, ''), COALESCE(sheba_number, ''), COALESCE(avatar_path, '') FROM employee_profiles WHERE employee_code = $1;`
		errAdminPf := database.DB.QueryRow(ctx, queryProfile, editFinancialCode).Scan(&pf.ContractType, &pf.IsMarried, &pf.ChildCount, &pf.EligibleForSeniority, &pf.CustomOvertimeRate, &pf.HourlyRate, &pf.RemainingLeaveHours, &pf.NationalCode, &pf.PhoneNumber, &pf.BankCardNumber, &pf.ShebaNumber, &pf.AvatarPath)

		if errAdminPf == nil {
			pf.FormattedLeave = FormatDuration(pf.RemainingLeaveHours)
			selectedProfile = &pf
		} else {
			selectedProfile = &FinancialProfileView{EmployeeCode: editFinancialCode, ContractType: "REGULAR", FormattedLeave: "00:00"}
		}
		currentTab = "management"
	}

	var editLog *WorkLogView = nil
	if editIDParam != "" {
		eID, errConv := strconv.Atoi(editIDParam)
		if errConv == nil {
			var ev WorkLogView
			editLogQuery := "SELECT w.id, w.employee_code, e.full_name, w.project_id, COALESCE(w.hours_spent, 0), COALESCE(w.description, ''), w.shamsi_date FROM work_logs w JOIN employees e ON w.employee_code = e.employee_code WHERE w.id=$1;"
			errLog := database.DB.QueryRow(ctx, editLogQuery, eID).Scan(&ev.ID, &ev.EmployeeCode, &ev.EmployeeName, &ev.ProjectID, &ev.HoursSpent, &ev.Description, &ev.ShamsiDate)
			if errLog == nil {
				ev.FormattedHours = FormatDuration(ev.HoursSpent)
				editLog = &ev
				currentTab = "worklog"
			}
		}
	}

	var editAttLog *AttendanceView = nil
	if editAttIDParam != "" {
		aID, errConv := strconv.Atoi(editAttIDParam)
		if errConv == nil {
			var av AttendanceView
			var tIn, tOut *time.Time
			editAttQuery := "SELECT a.id, a.employee_code, e.full_name, a.check_in, a.check_out, a.shamsi_date FROM attendance a JOIN employees e ON a.employee_code = e.employee_code WHERE a.id=$1;"
			errAtt := database.DB.QueryRow(ctx, editAttQuery, aID).Scan(&av.ID, &av.EmployeeCode, &av.EmployeeName, &tIn, &tOut, &av.ShamsiDate)
			if errAtt == nil {
				if tIn != nil {
					av.CheckIn = tIn.In(time.Local).Format("15:04")
				}
				if tOut != nil {
					av.CheckOut = tOut.In(time.Local).Format("15:04")
				}
				editAttLog = &av
				currentTab = "attendance"
			}
		}
	}

	targetFilterUser := filterEmployee
	if role != "ADMIN" {
		targetFilterUser = username
	}

	var workLogs []WorkLogView
	logQuery := `SELECT w.id, w.employee_code, e.full_name, w.project_id, p.name, COALESCE(w.hours_spent, 0), COALESCE(w.description, ''), w.shamsi_date FROM work_logs w JOIN projects p ON w.project_id = p.id JOIN employees e ON w.employee_code = e.employee_code WHERE 1=1 `
	var logArgs []interface{}
	argIdx := 1

	if role != "ADMIN" || targetFilterUser != "" {
		logQuery += fmt.Sprintf("AND w.employee_code = $%d ", argIdx)
		logArgs = append(logArgs, targetFilterUser)
		argIdx++
	}
	if filterProject != "" {
		pID, errConv := strconv.Atoi(filterProject)
		if errConv == nil {
			logQuery += fmt.Sprintf("AND w.project_id = $%d ", argIdx)
			logArgs = append(logArgs, pID)
			argIdx++
		}
	}
	if filterMonth != "" {
		logQuery += fmt.Sprintf("AND split_part(w.shamsi_date, '/', 2) = $%d ", argIdx)
		logArgs = append(logArgs, filterMonth)
		argIdx++
	}
	logQuery += "ORDER BY w.shamsi_date DESC, w.id DESC;"

	wlRows, errWl := database.DB.Query(ctx, logQuery, logArgs...)
	if errWl == nil && wlRows != nil {
		defer wlRows.Close()
		for wlRows.Next() {
			var wl WorkLogView
			errScan := wlRows.Scan(&wl.ID, &wl.EmployeeCode, &wl.EmployeeName, &wl.ProjectID, &wl.ProjectName, &wl.HoursSpent, &wl.Description, &wl.ShamsiDate)
			if errScan == nil {
				wl.FormattedHours = FormatDuration(wl.HoursSpent)
				workLogs = append(workLogs, wl)
			}
		}
	}

	var totalHours float64
	sumQuery := "SELECT COALESCE(SUM(hours_spent), 0) FROM work_logs WHERE 1=1 "
	var sumArgs []interface{}
	sumArgIdx := 1

	if role != "ADMIN" || targetFilterUser != "" {
		sumQuery += fmt.Sprintf("AND employee_code = $%d ", sumArgIdx)
		sumArgs = append(sumArgs, targetFilterUser)
		sumArgIdx++
	}
	if filterProject != "" {
		pID, errConv := strconv.Atoi(filterProject)
		if errConv == nil {
			sumQuery += fmt.Sprintf("AND project_id = $%d ", sumArgIdx)
			sumArgs = append(sumArgs, pID)
			sumArgIdx++
		}
	}
	if filterMonth != "" {
		sumQuery += fmt.Sprintf("AND split_part(shamsi_date, '/', 2) = $%d ", sumArgIdx)
		sumArgs = append(sumArgs, filterMonth)
		sumArgIdx++
	}
	errSum := database.DB.QueryRow(ctx, sumQuery, sumArgs...).Scan(&totalHours)
	if errSum != nil {
		totalHours = 0
	}

	var attLogs []AttendanceView
	var attQuery string
	var attArgs []interface{}
	var totalAttendanceHours float64 = 0 
	attArgIdx := 1

	if role == "ADMIN" && filterEmployee == "" {
		attQuery = "SELECT a.id, a.employee_code, e.full_name, a.check_in, a.check_out, a.shamsi_date FROM attendance a JOIN employees e ON a.employee_code = e.employee_code WHERE 1=1 "
	} else {
		attQuery = "SELECT a.id, a.employee_code, e.full_name, a.check_in, a.check_out, a.shamsi_date FROM attendance a JOIN employees e ON a.employee_code = e.employee_code WHERE a.employee_code = $1 "
		attArgs = append(attArgs, targetFilterUser)
		attArgIdx++
	}

	if filterMonth != "" {
		attQuery += fmt.Sprintf("AND split_part(a.shamsi_date, '/', 2) = $%d ", attArgIdx)
		attArgs = append(attArgs, filterMonth)
	}
	attQuery += "ORDER BY a.id DESC;"

	aRows, errA := database.DB.Query(ctx, attQuery, attArgs...)
	if errA == nil && aRows != nil {
		defer aRows.Close()
		for aRows.Next() {
			var av AttendanceView
			var tIn, tOut *time.Time
			errScan := aRows.Scan(&av.ID, &av.EmployeeCode, &av.EmployeeName, &tIn, &tOut, &av.ShamsiDate)

			if errScan == nil {
				if tIn != nil {
					av.CheckIn = tIn.In(time.Local).Format("15:04:05")
				}
				if tOut != nil {
					av.CheckOut = tOut.In(time.Local).Format("15:04:05")
					diff := tOut.Sub(*tIn)

					totalAttendanceHours += diff.Hours()

					av.Duration = fmt.Sprintf("%d ساعت و %d دقیقه", int(diff.Hours()), int(diff.Minutes())%60)
				} else {
					av.Duration = "حضور زنده فعال"
				}
				attLogs = append(attLogs, av)
			}
		}
	}

	var employees []EmployeeView
	if role == "ADMIN" {
		eRows, errE := database.DB.Query(ctx, "SELECT id, employee_code, full_name, role FROM employees ORDER BY id DESC;")
		if errE == nil && eRows != nil {
			defer eRows.Close()
			for eRows.Next() {
				var ev EmployeeView
				errScan := eRows.Scan(&ev.ID, &ev.EmployeeCode, &ev.FullName, &ev.Role)
				if errScan == nil {
					employees = append(employees, ev)
				}
			}
		}
	}

	var projects []ProjectView
	var pQuery string
	var pArgs []interface{}

	if role == "ADMIN" {
		pQuery = "SELECT id, name FROM projects ORDER BY id ASC;"
	} else {
		pQuery = "SELECT p.id, p.name FROM projects p JOIN employee_projects ep ON p.id = ep.project_id WHERE ep.employee_code = $1 ORDER BY p.id ASC;"
		pArgs = append(pArgs, username)
	}

	pRows, errPrj := database.DB.Query(ctx, pQuery, pArgs...)
	if errPrj == nil && pRows != nil {
		defer pRows.Close()
		for pRows.Next() {
			var pv ProjectView
			errScan := pRows.Scan(&pv.ID, &pv.Name)
			if errScan == nil {
				projects = append(projects, pv)
			}
		}
	}

	todayStr := "--"
	if len(attLogs) > 0 && attLogs[0].ShamsiDate == attendance.GetCurrentShamsiDate() {
		todayStr = attLogs[0].Duration
	}

	data := PageData{
		IsLoggedIn:              true,
		CurrentDate:             attendance.GetCurrentShamsiDate(),
		TotalHours:              totalHours,
		TotalHoursFormatted:     FormatDuration(totalHours),
		Message:                 flashMsg,
		CurrentUser:             username,
		CurrentFullName:         currentFullName,
		CurrentAvatar:           currentAvatar,
		CurrentRole:             role,
		SelectedFilter:          filterEmployee,
		SelectedProjectFilter:   filterProject,
		SelectedMonthFilter:     filterMonth,
		CurrentTab:              currentTab,
		WorkLogs:                workLogs,
		AttendanceLogs:          attLogs,
		Employees:               employees,
		Projects:                projects,
		EditLog:                 editLog,
		EditAttendanceLog:       editAttLog,
		TotalAttendanceMonthStr: FormatDuration(totalAttendanceHours),
		TotalAttendanceDayStr:   todayStr,
		SelectedProfile:         selectedProfile,
		MyProfile:               myProfile,
		LiveStatuses:            liveStatuses,
		PieLabelsJSON:           template.JS(plJSON),
		PieDataJSON:             template.JS(pdJSON),
		BarLabelsJSON:           template.JS(blJSON),
		BarDataJSON:             template.JS(bdJSON),
		RecentTasks:             recentTasks,
		ActiveProjects:          activeProjects,
		SelectedMonthsMap:       selectedMonthsMap,
		MyPayslips:              myPayslips,
		AllPayslips:             allPayslips,
		ExpectedHoursList:       expectedHoursList,
		PayrollConstants:        payrollConstants,
		ProjectAccessList:       projectAccessList,
	}

	var targetTemplateFile string = "templates/employee.html"
	if role == "ADMIN" {
		targetTemplateFile = "templates/admin.html"
	}

	tmpl, errTmpl := template.ParseFiles(targetTemplateFile)
	if errTmpl != nil {
		http.Error(w, "Error parsing template", http.StatusInternalServerError)
		return
	}
	errExec := tmpl.Execute(w, data)
	if errExec != nil {
		log.Printf("Template execution error: %v", errExec)
	}
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		username := strings.ToUpper(strings.TrimSpace(r.FormValue("username")))
		password := r.FormValue("password")
		ctx := context.Background()

		var dbUser, hashedPassword string
		err := database.DB.QueryRow(ctx, "SELECT employee_code, password FROM employees WHERE employee_code = $1;", username).Scan(&dbUser, &hashedPassword)

		if err == nil && bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password)) == nil {
			http.SetCookie(w, &http.Cookie{
				Name:     "session_user",
				Value:    url.QueryEscape(username),
				Expires:  time.Now().Add(24 * time.Hour),
				HttpOnly: true,
				Path:     "/",
			})
			setFlashMessage(w, "ورود با موفقیت انجام شد.")
		} else {
			setFlashMessage(w, "خطا: نام کاربری یا کلمه عبور اشتباه است!")
		}
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session_user",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HttpOnly: true,
		Path:     "/",
	})
	setFlashMessage(w, "از سیستم خارج شدید.")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleCheckIn(w http.ResponseWriter, r *http.Request) {
	username, _ := getAuthenticatedUser(r)
	tab := r.FormValue("tab")
	if tab == "" {
		tab = "attendance"
	}

	if username != "" {
		err := attendance.CheckIn(username)
		if err != nil {
			setFlashMessage(w, "⚠️ خطا: "+err.Error())
		} else {
			setFlashMessage(w, "ورود زنده شما با موفقیت ثبت شد.")
		}
	}
	http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
}

func handleCheckOut(w http.ResponseWriter, r *http.Request) {
	username, _ := getAuthenticatedUser(r)
	tab := r.FormValue("tab")
	if tab == "" {
		tab = "attendance"
	}

	if username != "" {
		err := attendance.CheckOut(username)
		if err != nil {
			setFlashMessage(w, "⚠️ خطا: "+err.Error())
		} else {
			setFlashMessage(w, "خروج زنده با موفقیت ثبت شد.")
		}
	}
	http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
}

func handleLogWork(w http.ResponseWriter, r *http.Request) {
	username, _ := getAuthenticatedUser(r)
	tab := r.FormValue("tab")
	if tab == "" {
		tab = "worklog"
	}

	if username != "" {
		pID, errPid := strconv.Atoi(r.FormValue("project_id"))
		if errPid != nil {
			setFlashMessage(w, "❌ خطا: لطفاً یک پروژه معتبر انتخاب کنید.")
			http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
			return
		}

		hours := ParseDuration(r.FormValue("hours"))
		desc := r.FormValue("description")
		sDate := r.FormValue("shamsi_date")

		err := project.LogWorkWithDate(username, pID, hours, desc, sDate)
		if err != nil {
			setFlashMessage(w, "❌ خطا: "+err.Error())
		} else {
			setFlashMessage(w, "گزارش کارکرد با موفقیت ثبت شد.")
		}
	}
	http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
}

func handleManualAttendance(w http.ResponseWriter, r *http.Request) {
	username, role := getAuthenticatedUser(r)
	tab := r.FormValue("tab")
	if tab == "" {
		tab = "attendance"
	}

	if username != "" {
		sDate := r.FormValue("shamsi_date")
		tIn := r.FormValue("check_in_time")
		tOut := r.FormValue("check_out_time")
		attIDStr := r.FormValue("attendance_id")

		target := username
		if role == "ADMIN" && r.FormValue("target_employee") != "" {
			target = strings.ToUpper(strings.TrimSpace(r.FormValue("target_employee")))
		}

		if attIDStr != "" {
			attID, errConv := strconv.Atoi(attIDStr)
			if errConv == nil {
				_, errDel := database.DB.Exec(context.Background(), "DELETE FROM attendance WHERE id = $1;", attID)
				if errDel != nil {
					log.Printf("Error deleting old attendance: %v", errDel)
				}
			}
		}

		err := attendance.AddManualAttendance(target, sDate, tIn, tOut)
		if err != nil {
			setFlashMessage(w, "خطا: "+err.Error())
		} else {
			if attIDStr != "" {
				setFlashMessage(w, "تردد با موفقیت ویرایش و جایگزین شد.")
			} else {
				setFlashMessage(w, "تردد اصلاحی با موفقیت ثبت شد.")
			}
		}
	}
	http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
}

func handleEditWorkLog(w http.ResponseWriter, r *http.Request) {
	username, _ := getAuthenticatedUser(r)
	tab := r.FormValue("tab")
	if tab == "" {
		tab = "worklog"
	}

	if username != "" {
		logID, errLid := strconv.Atoi(r.FormValue("log_id"))
		pID, errPid := strconv.Atoi(r.FormValue("project_id"))

		if errLid != nil || errPid != nil {
			setFlashMessage(w, "❌ خطا در مقادیر ورودی")
			http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
			return
		}

		hours := ParseDuration(r.FormValue("hours"))
		desc := r.FormValue("description")
		sDate := r.FormValue("shamsi_date")

		err := project.UpdateWorkLog(logID, pID, hours, desc, sDate)
		if err != nil {
			setFlashMessage(w, "❌ خطا: "+err.Error())
		} else {
			setFlashMessage(w, "گزارش کارکرد ویرایش شد.")
		}
	}
	http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
}

func handleDeleteWorkLog(w http.ResponseWriter, r *http.Request) {
	username, _ := getAuthenticatedUser(r)
	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "worklog"
	}

	if username != "" {
		logID, errConv := strconv.Atoi(r.URL.Query().Get("id"))
		if errConv == nil {
			errDel := project.DeleteWorkLog(logID)
			if errDel == nil {
				setFlashMessage(w, "رکورد کارکرد حذف شد.")
			} else {
				setFlashMessage(w, "❌ خطا در حذف رکورد")
			}
		}
	}
	http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
}

func handleDeleteAttendance(w http.ResponseWriter, r *http.Request) {
	username, _ := getAuthenticatedUser(r)
	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "attendance"
	}

	if username != "" {
		attID, errConv := strconv.Atoi(r.URL.Query().Get("id"))
		if errConv == nil {
			_, errDel := database.DB.Exec(context.Background(), "DELETE FROM attendance WHERE id = $1;", attID)
			if errDel == nil {
				setFlashMessage(w, "تردد مورد نظر با موفقیت حذف گردید.")
			} else {
				setFlashMessage(w, "❌ خطا در حذف تردد")
			}
		}
	}
	http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
}

func handleAddEmployee(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" {
		return
	}

	if r.Method == http.MethodPost {
		empCode := strings.ToUpper(strings.TrimSpace(r.FormValue("emp_code")))
		if empCode == "" {
			empCode = strings.ToUpper(strings.TrimSpace(r.FormValue("employee_code")))
		}

		password := r.FormValue("password")
		fullName := strings.TrimSpace(r.FormValue("full_name"))
		uRole := r.FormValue("role")

		hash, errHash := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if errHash != nil {
			setFlashMessage(w, "❌ خطا در سیستم رمزنگاری")
			http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
			return
		}

		query := "INSERT INTO employees (employee_code, full_name, password, role) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING;"
		_, err := database.DB.Exec(context.Background(), query, empCode, fullName, string(hash), uRole)

		if err != nil {
			setFlashMessage(w, "خطا: "+err.Error())
		} else {
			setFlashMessage(w, "نیروی جدید استخدام شد.")
		}
	}
	http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
}

func handleEditEmployee(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" {
		return
	}

	if r.Method == http.MethodPost {
		ctx := context.Background()
		origCode := strings.ToUpper(strings.TrimSpace(r.FormValue("original_emp_code")))
		newCode := strings.ToUpper(strings.TrimSpace(r.FormValue("emp_code")))
		if newCode == "" {
			newCode = origCode
		}

		var existingFullName string
		errEx := database.DB.QueryRow(ctx, "SELECT full_name FROM employees WHERE employee_code = $1;", origCode).Scan(&existingFullName)
		if errEx != nil {
			setFlashMessage(w, "❌ پرسنل مورد نظر یافت نشد")
			http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
			return
		}

		fullName := r.FormValue("full_name")
		if fullName == "" {
			fullName = existingFullName
		}

		uRole := r.FormValue("role")
		password := r.FormValue("password")

		if newCode != origCode && origCode != "" {
			_, errUpdate := database.DB.Exec(ctx, "UPDATE employees SET employee_code = $1 WHERE employee_code = $2;", newCode, origCode)
			if errUpdate == nil {
				database.DB.Exec(ctx, "UPDATE work_logs SET employee_code = $1 WHERE employee_code = $2;", newCode, origCode)
				database.DB.Exec(ctx, "UPDATE attendance SET employee_code = $1 WHERE employee_code = $2;", newCode, origCode)
				database.DB.Exec(ctx, "UPDATE employee_profiles SET employee_code = $1 WHERE employee_code = $2;", newCode, origCode)
				database.DB.Exec(ctx, "UPDATE payslips SET employee_code = $1 WHERE employee_code = $2;", newCode, origCode)
				database.DB.Exec(ctx, "UPDATE employee_projects SET employee_code = $1 WHERE employee_code = $2;", newCode, origCode)
			}
		}

		if password != "" {
			hash, errHash := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if errHash == nil {
				_, err := database.DB.Exec(ctx, "UPDATE employees SET full_name = $1, role = $2, password = $3 WHERE employee_code = $4;", fullName, uRole, string(hash), newCode)
				if err != nil {
					setFlashMessage(w, "خطا: "+err.Error())
				} else {
					setFlashMessage(w, "مشخصات و رمز عبور پرسنل بروزرسانی شد.")
				}
			}
		} else {
			_, err := database.DB.Exec(ctx, "UPDATE employees SET full_name = $1, role = $2 WHERE employee_code = $3;", fullName, uRole, newCode)
			if err != nil {
				setFlashMessage(w, "خطا: "+err.Error())
			} else {
				setFlashMessage(w, "مشخصات پرسنل با موفقیت بروزرسانی شد.")
			}
		}
	}
	http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
}

func handleDeleteEmployee(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" {
		return
	}

	id, errConv := strconv.Atoi(r.URL.Query().Get("id"))
	if errConv == nil {
		_, errDel := database.DB.Exec(context.Background(), "DELETE FROM employees WHERE id = $1;", id)
		if errDel == nil {
			setFlashMessage(w, "نیرو حذف گردید.")
		} else {
			setFlashMessage(w, "❌ خطا در حذف نیرو")
		}
	}

	http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
}

func handleCreateProject(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" {
		return
	}

	if r.Method == http.MethodPost {
		name := strings.TrimSpace(r.FormValue("project_name"))
		if name == "" {
			name = strings.TrimSpace(r.FormValue("name"))
		}

		if name != "" {
			_, err := database.DB.Exec(context.Background(), "INSERT INTO projects (name) VALUES ($1) ON CONFLICT DO NOTHING;", name)
			if err != nil {
				setFlashMessage(w, "خطا: "+err.Error())
			} else {
				setFlashMessage(w, "✅ پروژه جدید با موفقیت ایجاد شد.")
			}
		}
	}
	http.Redirect(w, r, "/?tab=projects", http.StatusSeeOther)
}

func handleEditProject(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" {
		return
	}

	if r.Method == http.MethodPost {
		id, errConv := strconv.Atoi(r.FormValue("project_id"))
		newName := strings.TrimSpace(r.FormValue("new_project_name"))

		if errConv == nil && newName != "" {
			_, errUpd := database.DB.Exec(context.Background(), "UPDATE projects SET name = $1 WHERE id = $2;", newName, id)
			if errUpd == nil {
				setFlashMessage(w, "نام پروژه اصلاح شد.")
			} else {
				setFlashMessage(w, "❌ خطا در ویرایش پروژه")
			}
		}
	}
	http.Redirect(w, r, "/?tab=projects", http.StatusSeeOther)
}

func handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" {
		return
	}

	id, errConv := strconv.Atoi(r.URL.Query().Get("id"))
	if errConv == nil {
		_, errDel := database.DB.Exec(context.Background(), "DELETE FROM projects WHERE id = $1;", id)
		if errDel == nil {
			setFlashMessage(w, "پروژه حذف شد.")
		} else {
			setFlashMessage(w, "❌ خطا در حذف پروژه")
		}
	}

	http.Redirect(w, r, "/?tab=projects", http.StatusSeeOther)
}

func handleSavePayrollProfile(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" {
		return
	}

	if r.Method == http.MethodPost {
		empCode := strings.ToUpper(strings.TrimSpace(r.FormValue("employee_code")))
		cType := r.FormValue("contract_type")
		isMarried := r.FormValue("is_married") == "true"
		childCount, _ := strconv.Atoi(r.FormValue("child_count"))
		eligibleSeniority := r.FormValue("eligible_for_seniority") == "true"
		overtimeRate, _ := strconv.ParseInt(r.FormValue("custom_overtime_rate"), 10, 64)
		hourlyRate, _ := strconv.ParseInt(r.FormValue("hourly_rate"), 10, 64)
		leaveHours := ParseDuration(r.FormValue("remaining_leave_hours"))
		nationalCode := strings.TrimSpace(r.FormValue("national_code"))
		phoneNumber := strings.TrimSpace(r.FormValue("phone_number"))
		bankCard := strings.TrimSpace(r.FormValue("bank_card"))
		sheba := strings.TrimSpace(r.FormValue("sheba"))

		query := `INSERT INTO employee_profiles (
					employee_code, contract_type, is_married, child_count, eligible_for_seniority, 
					custom_overtime_rate, hourly_rate, remaining_leave_hours, 
					national_code, phone_number, bank_card_number, sheba_number
				  ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) 
				  ON CONFLICT (employee_code) DO UPDATE SET 
					contract_type=EXCLUDED.contract_type, is_married=EXCLUDED.is_married, 
					child_count=EXCLUDED.child_count, eligible_for_seniority=EXCLUDED.eligible_for_seniority, 
					custom_overtime_rate=EXCLUDED.custom_overtime_rate, hourly_rate=EXCLUDED.hourly_rate, 
					remaining_leave_hours=EXCLUDED.remaining_leave_hours, national_code=EXCLUDED.national_code, 
					phone_number=EXCLUDED.phone_number, bank_card_number=EXCLUDED.bank_card_number, 
					sheba_number=EXCLUDED.sheba_number;`

		_, err := database.DB.Exec(context.Background(), query, empCode, cType, isMarried, childCount, eligibleSeniority, overtimeRate, hourlyRate, leaveHours, nationalCode, phoneNumber, bankCard, sheba)

		if err != nil {
			setFlashMessage(w, "❌ خطا در ذخیره پروفایل مالی: "+err.Error())
		} else {
			setFlashMessage(w, "✅ پروفایل حقوقی و اطلاعات بانکی پرسنل با موفقیت ذخیره شد.")
		}
	}
	http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
}

func handleExportExcel(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	username, role := getAuthenticatedUser(r)
	if username == "" {
		return
	}

	filterEmp := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("employee_code")))
	filterProj := r.URL.Query().Get("project_id")
	filterMonth := r.URL.Query().Get("filter_month")
	targetExcelUser := filterEmp

	if role != "ADMIN" {
		targetExcelUser = username
	}

	logQuery := `SELECT w.employee_code, e.full_name, w.shamsi_date, p.name, COALESCE(w.hours_spent, 0), COALESCE(w.description, '') 
                 FROM work_logs w 
				 JOIN projects p ON w.project_id = p.id 
				 JOIN employees e ON w.employee_code = e.employee_code 
				 WHERE 1=1 `

	var args []interface{}
	argIdx := 1

	if role != "ADMIN" || targetExcelUser != "" {
		logQuery += fmt.Sprintf("AND w.employee_code = $%d ", argIdx)
		args = append(args, targetExcelUser)
		argIdx++
	}
	if filterProj != "" {
		pID, errConv := strconv.Atoi(filterProj)
		if errConv == nil {
			logQuery += fmt.Sprintf("AND w.project_id = $%d ", argIdx)
			args = append(args, pID)
			argIdx++
		}
	}
	if filterMonth != "" {
		logQuery += fmt.Sprintf("AND split_part(w.shamsi_date, '/', 2) = $%d ", argIdx)
		args = append(args, filterMonth)
	}
	logQuery += "ORDER BY w.shamsi_date DESC;"

	rows, err := database.DB.Query(ctx, logQuery, args...)
	if err != nil {
		return
	}
	defer rows.Close()

	f := excelize.NewFile()
	sheetName := "گزارش ماتریسی کارکرد"
	f.SetSheetName("Sheet1", sheetName)
	f.SetCellValue(sheetName, "A1", "کد کارمند")
	f.SetCellValue(sheetName, "B1", "نام پرسنل")
	f.SetCellValue(sheetName, "C1", "تاریخ شمسی")
	f.SetCellValue(sheetName, "D1", "نام پروژه")
	f.SetCellValue(sheetName, "E1", "مدت زمان")
	f.SetCellValue(sheetName, "F1", "شرح")

	rowIdx := 2
	for rows.Next() {
		var empCode, empName, sDate, pName, desc string
		var hours float64
		errScan := rows.Scan(&empCode, &empName, &sDate, &pName, &hours, &desc)
		if errScan == nil {
			f.SetCellValue(sheetName, fmt.Sprintf("A%d", rowIdx), empCode)
			f.SetCellValue(sheetName, fmt.Sprintf("B%d", rowIdx), empName)
			f.SetCellValue(sheetName, fmt.Sprintf("C%d", rowIdx), sDate)
			f.SetCellValue(sheetName, fmt.Sprintf("D%d", rowIdx), pName)
			f.SetCellValue(sheetName, fmt.Sprintf("E%d", rowIdx), FormatDuration(hours))
			f.SetCellValue(sheetName, fmt.Sprintf("F%d", rowIdx), desc)
			rowIdx++
		}
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=shamsi_matrix_report.xlsx")
	errWrite := f.Write(w)
	if errWrite != nil {
		log.Printf("Error writing excel: %v", errWrite)
	}
}

// ----------------------------------------------------
// 6. ماژول‌های پشتیبان‌گیری (Backup Handlers)
// ----------------------------------------------------
func handleManualBackup(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	
	// بررسی امنیتی برای جلوگیری از اجرای غیرمجاز
	if role != "ADMIN" {
		setFlashMessage(w, "❌ دسترسی غیرمجاز: فقط مدیر سیستم مجاز به تهیه فایل پشتیبان است.")
		http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
		return
	}

	fileName, err := backup.RunManualBackup()
	if err != nil {
		setFlashMessage(w, "❌ خطا در فرآیند پشتیبان‌گیری: "+err.Error())
	} else {
		setFlashMessage(w, "✅ عملیات موفق: نسخه پشتیبان با نام ("+fileName+") در پوشه backups ذخیره شد.")
	}
	
	http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
}