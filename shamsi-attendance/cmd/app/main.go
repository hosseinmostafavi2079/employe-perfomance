package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"shamsi_attendance/internal/attendance"
	"shamsi_attendance/internal/database"
	"shamsi_attendance/internal/payroll"
	"shamsi_attendance/internal/project"

	"github.com/xuri/excelize/v2"
	"golang.org/x/crypto/bcrypt"
)

const (
	DailyBaseWage1405      int64   = 5541850
	DailySeniority1405     int64   = 166667
	MonthlyBon1405         int64   = 22000000
	MonthlyHousing1405     int64   = 30000000
	MonthlyMarital1405     int64   = 5000000
	MonthlyChildPerOne1405 int64   = 16625550
	MonthlyLeaveAccrual    float64 = 20.0
)

// --- توابع کمکی برای تبدیل زمان دقیق بسیار ایمن ---
func ParseDuration(input string) float64 {
	input = strings.TrimSpace(input)
	if input == "" {
		return 0
	}
	if strings.Contains(input, ":") {
		parts := strings.Split(input, ":")
		if len(parts) == 2 {
			h, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			m, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			return h + (m / 60.0)
		}
	}
	f, _ := strconv.ParseFloat(input, 64)
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

// ----------------------------------------

type WorkLogView struct {
	ID             int
	EmployeeCode   string
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
	NationalCode         string
	PhoneNumber          string
	BankCardNumber       string
	ShebaNumber          string
}

type LiveStatusView struct {
	EmployeeCode string
	FullName     string
	IsPresent    bool
	CheckInTime  string
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
	
	// دیتای اختصاصی داشبورد آنالیز
	LiveStatuses      []LiveStatusView
	PieLabelsJSON     template.JS
	PieDataJSON       template.JS
	BarLabelsJSON     template.JS
	BarDataJSON       template.JS
	RecentTasks       []WorkLogView
	ActiveProjects    []string
	SelectedMonthsMap map[string]bool
}

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
	dc := &http.Cookie{Name: "flash_message", Value: "", Path: "/", HttpOnly: true, MaxAge: -1}
	http.SetCookie(w, dc)
	decodedMessage, err := url.QueryUnescape(cookie.Value)
	if err != nil {
		return cookie.Value
	}
	return decodedMessage
}

func main() {
	fmt.Println("==================================================")
	fmt.Println("در حال راه‌اندازی وب‌سرور حرفه‌ای و تجاری سامانه...")
	fmt.Println("==================================================")

	database.ConnectToDatabase()
	if database.DB != nil {
		defer database.DB.Close()
	}

	ctx := context.Background()
	_, _ = database.DB.Exec(ctx, `ALTER TABLE employee_profiles ADD COLUMN IF NOT EXISTS national_code VARCHAR(10) DEFAULT '';`)
	_, _ = database.DB.Exec(ctx, `ALTER TABLE employee_profiles ADD COLUMN IF NOT EXISTS phone_number VARCHAR(11) DEFAULT '';`)
	_, _ = database.DB.Exec(ctx, `ALTER TABLE employee_profiles ADD COLUMN IF NOT EXISTS bank_card_number VARCHAR(16) DEFAULT '';`)
	_, _ = database.DB.Exec(ctx, `ALTER TABLE employee_profiles ADD COLUMN IF NOT EXISTS sheba_number VARCHAR(26) DEFAULT '';`)

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

	http.HandleFunc("/admin/payroll/save-profile", handleSavePayrollProfile)
	http.HandleFunc("/admin/payroll/issue", handleIssuePayroll)
	http.HandleFunc("/admin/add-employee", handleAddEmployee)
	http.HandleFunc("/admin/edit-employee", handleEditEmployee)
	http.HandleFunc("/admin/delete-employee", handleDeleteEmployee)
	http.HandleFunc("/admin/create-project", handleCreateProject)
	http.HandleFunc("/admin/edit-project", handleEditProject)
	http.HandleFunc("/admin/delete-project", handleDeleteProject)

	fmt.Println("🚀 وب‌سرور امن تفکیک‌شده با موفقیت روی پورت 8085 روشن شد!")
	fmt.Println("🌐 آدرس ورود به سامانه: http://localhost:8085")
	fmt.Println("==================================================")
	log.Fatal(http.ListenAndServe(":8085", nil))
}

func getAuthenticatedUser(r *http.Request) (string, string) {
	cookie, err := r.Cookie("session_user")
	if err != nil {
		return "", ""
	}
	username, err := url.QueryUnescape(cookie.Value)
	if err != nil {
		username = cookie.Value
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

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	username, role := getAuthenticatedUser(r)
	flashMsg := getFlashMessage(w, r)

	if username == "" {
		tmpl, err := template.ParseFiles("templates/login.html")
		if err != nil {
			http.Error(w, "Error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, PageData{IsLoggedIn: false, Message: flashMsg, CurrentDate: attendance.GetCurrentShamsiDate()})
		return
	}

	var currentFullName string
	_ = database.DB.QueryRow(ctx, "SELECT full_name FROM employees WHERE employee_code = $1;", username).Scan(&currentFullName)
	if currentFullName == "" {
		currentFullName = username
	}

	r.ParseForm()
	editIDParam := r.FormValue("edit_id")
	editAttIDParam := r.FormValue("edit_attendance_id")
	editFinancialCode := strings.ToUpper(strings.TrimSpace(r.FormValue("edit_financial_code")))
	filterEmployee := strings.ToUpper(strings.TrimSpace(r.FormValue("filter_employee")))
	filterProject := r.FormValue("filter_project")
	filterMonth := r.FormValue("filter_month") // Single select fallback
	filterMonths := r.Form["filter_months"]    // Multi-select array
	tabParam := r.FormValue("tab")

	// Fallback برای زمانی که کاربر به جای انتخاب چندتایی، فقط یک ماه را زده باشد
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
	} else if tabParam == "dashboard" && role == "ADMIN" {
		currentTab = "dashboard"
	} else if tabParam == "worklog" {
		currentTab = "worklog"
	} else if tabParam == "management" && role == "ADMIN" {
		currentTab = "management"
	} else if tabParam == "projects" && role == "ADMIN" {
		currentTab = "projects"
	} else if tabParam == "project_report" && role == "ADMIN" {
		currentTab = "project_report"
	} else {
		currentTab = tabParam
	}

	// --- استخراج داده‌های هوشمند داشبورد مدیریت ---
	var liveStatuses []LiveStatusView
	pieLabels := []string{}
	pieData := []float64{}
	barLabels := []string{}
	barData := []float64{}
	var recentTasks []WorkLogView
	activeProjects := []string{}
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
		// ۱. وضعیت زنده (شامل تمامی مدیرها و پرسنل)
		lsRows, _ := database.DB.Query(ctx, `
			SELECT e.employee_code, e.full_name,
				   (SELECT check_in FROM attendance a WHERE a.employee_code = e.employee_code AND a.shamsi_date = $1 ORDER BY id DESC LIMIT 1),
				   (SELECT check_out FROM attendance a WHERE a.employee_code = e.employee_code AND a.shamsi_date = $1 ORDER BY id DESC LIMIT 1)
			FROM employees e
			ORDER BY e.role ASC, e.full_name ASC
		`, todayStrDate)
		if lsRows != nil {
			defer lsRows.Close()
			for lsRows.Next() {
				var lsv LiveStatusView
				var tIn, tOut *time.Time
				lsRows.Scan(&lsv.EmployeeCode, &lsv.FullName, &tIn, &tOut)
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

		// ساخت کوئری‌های داینامیک بر اساس ماه و شخص
		var args []interface{}
		args = append(args, yearPrefix)
		argCounter := 2
		
		empCondition := ""
		if filterEmployee != "" {
			empCondition = fmt.Sprintf("AND w.employee_code = $%d", argCounter)
			args = append(args, filterEmployee)
			argCounter++
		}
		
		monthCondition := ""
		if len(filterMonths) > 0 {
			placeholders := []string{}
			for _, m := range filterMonths {
				placeholders = append(placeholders, fmt.Sprintf("$%d", argCounter))
				args = append(args, m)
				argCounter++
			}
			monthCondition = fmt.Sprintf("AND split_part(w.shamsi_date, '/', 2) IN (%s)", strings.Join(placeholders, ","))
		}

		// ۲. نمودار دایره‌ای (سهم پروژه‌ها از کارکرد)
		pieQ := fmt.Sprintf(`
			SELECT p.name, COALESCE(SUM(w.hours_spent), 0)
			FROM work_logs w
			JOIN projects p ON w.project_id = p.id
			WHERE w.shamsi_date LIKE $1 || '%%' %s %s
			GROUP BY p.name
		`, empCondition, monthCondition)
		
		pRows, err := database.DB.Query(ctx, pieQ, args...)
		if err == nil {
			defer pRows.Close()
			for pRows.Next() {
				var pName string
				var h float64
				pRows.Scan(&pName, &h)
				pieLabels = append(pieLabels, pName)
				pieData = append(pieData, math.Round(h*100)/100)
			}
		}

		// ۳. نمودار میله‌ای (مقایسه ماه‌ها)
		barQ := fmt.Sprintf(`
			SELECT split_part(w.shamsi_date, '/', 2) as month, COALESCE(SUM(w.hours_spent), 0)
			FROM work_logs w
			WHERE w.shamsi_date LIKE $1 || '%%' %s %s
			GROUP BY month
			ORDER BY month ASC
		`, empCondition, monthCondition)
		
		bRows, err2 := database.DB.Query(ctx, barQ, args...)
		if err2 == nil {
			defer bRows.Close()
			for bRows.Next() {
				var m string
				var h float64
				bRows.Scan(&m, &h)
				barLabels = append(barLabels, "ماه "+m)
				barData = append(barData, math.Round(h*100)/100)
			}
		}

		// ۴. خلاصه وضعیت فرد (فقط اگر یک شخص خاص انتخاب شده باشد)
		if filterEmployee != "" {
			taskQ := fmt.Sprintf(`
				SELECT w.id, w.employee_code, w.project_id, p.name, COALESCE(w.hours_spent, 0), COALESCE(w.description, ''), w.shamsi_date 
				FROM work_logs w JOIN projects p ON w.project_id = p.id 
				WHERE w.shamsi_date LIKE $1 || '%%' %s %s
				ORDER BY w.shamsi_date DESC, w.id DESC LIMIT 8
			`, empCondition, monthCondition)
			
			tRows, err3 := database.DB.Query(ctx, taskQ, args...)
			if err3 == nil {
				defer tRows.Close()
				projMap := make(map[string]bool)
				for tRows.Next() {
					var wl WorkLogView
					tRows.Scan(&wl.ID, &wl.EmployeeCode, &wl.ProjectID, &wl.ProjectName, &wl.HoursSpent, &wl.Description, &wl.ShamsiDate)
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

	plJSON, _ := json.Marshal(pieLabels)
	pdJSON, _ := json.Marshal(pieData)
	blJSON, _ := json.Marshal(barLabels)
	bdJSON, _ := json.Marshal(barData)
	// --------------------------------------------------

	var selectedProfile *FinancialProfileView = nil
	if editFinancialCode != "" && role == "ADMIN" {
		var pf FinancialProfileView
		pf.EmployeeCode = editFinancialCode
		queryProfile := `
			SELECT contract_type, is_married, child_count, eligible_for_seniority, 
			       custom_overtime_rate, hourly_rate, remaining_leave_hours,
			       COALESCE(national_code, ''), COALESCE(phone_number, ''), 
			       COALESCE(bank_card_number, ''), COALESCE(sheba_number, '')
			FROM employee_profiles WHERE employee_code = $1;`
		err := database.DB.QueryRow(ctx, queryProfile, editFinancialCode).Scan(
			&pf.ContractType, &pf.IsMarried, &pf.ChildCount, &pf.EligibleForSeniority,
			&pf.CustomOvertimeRate, &pf.HourlyRate, &pf.RemainingLeaveHours,
			&pf.NationalCode, &pf.PhoneNumber, &pf.BankCardNumber, &pf.ShebaNumber,
		)
		if err == nil {
			selectedProfile = &pf
		} else {
			selectedProfile = &FinancialProfileView{EmployeeCode: editFinancialCode, ContractType: "REGULAR"}
		}
		currentTab = "management"
	}

	var editLog *WorkLogView = nil
	if editIDParam != "" {
		eID, _ := strconv.Atoi(editIDParam)
		var ev WorkLogView
		err := database.DB.QueryRow(ctx, "SELECT id, employee_code, project_id, COALESCE(hours_spent, 0), COALESCE(description, ''), shamsi_date FROM work_logs WHERE id=$1;", eID).
			Scan(&ev.ID, &ev.EmployeeCode, &ev.ProjectID, &ev.HoursSpent, &ev.Description, &ev.ShamsiDate)
		if err == nil {
			ev.FormattedHours = FormatDuration(ev.HoursSpent)
			editLog = &ev
			currentTab = "worklog"
		}
	}

	var editAttLog *AttendanceView = nil
	if editAttIDParam != "" {
		aID, _ := strconv.Atoi(editAttIDParam)
		var av AttendanceView
		var tIn, tOut *time.Time
		err := database.DB.QueryRow(ctx, "SELECT id, employee_code, check_in, check_out, shamsi_date FROM attendance WHERE id=$1;", aID).
			Scan(&av.ID, &av.EmployeeCode, &tIn, &tOut, &av.ShamsiDate)
		if err == nil {
			if tIn != nil { av.CheckIn = tIn.In(time.Local).Format("15:04") }
			if tOut != nil { av.CheckOut = tOut.In(time.Local).Format("15:04") }
			editAttLog = &av
			currentTab = "attendance"
		}
	}

	targetFilterUser := filterEmployee
	if role != "ADMIN" {
		targetFilterUser = username
	}

	var workLogs []WorkLogView
	logQuery := `SELECT w.id, w.employee_code, w.project_id, p.name, COALESCE(w.hours_spent, 0), COALESCE(w.description, ''), w.shamsi_date 
                 FROM work_logs w JOIN projects p ON w.project_id = p.id WHERE 1=1 `
	var logArgs []interface{}
	argIdx := 1

	if role != "ADMIN" || targetFilterUser != "" {
		logQuery += fmt.Sprintf("AND w.employee_code = $%d ", argIdx)
		logArgs = append(logArgs, targetFilterUser)
		argIdx++
	}
	if filterProject != "" {
		pID, _ := strconv.Atoi(filterProject)
		logQuery += fmt.Sprintf("AND w.project_id = $%d ", argIdx)
		logArgs = append(logArgs, pID)
		argIdx++
	}
	if filterMonth != "" {
		logQuery += fmt.Sprintf("AND split_part(w.shamsi_date, '/', 2) = $%d ", argIdx)
		logArgs = append(logArgs, filterMonth)
		argIdx++
	}
	logQuery += "ORDER BY w.shamsi_date DESC, w.id DESC;"

	rows, err := database.DB.Query(ctx, logQuery, logArgs...)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var wl WorkLogView
			if err := rows.Scan(&wl.ID, &wl.EmployeeCode, &wl.ProjectID, &wl.ProjectName, &wl.HoursSpent, &wl.Description, &wl.ShamsiDate); err == nil {
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
		pID, _ := strconv.Atoi(filterProject)
		sumQuery += fmt.Sprintf("AND project_id = $%d ", sumArgIdx)
		sumArgs = append(sumArgs, pID)
		sumArgIdx++
	}
	if filterMonth != "" {
		sumQuery += fmt.Sprintf("AND split_part(shamsi_date, '/', 2) = $%d ", sumArgIdx)
		sumArgs = append(sumArgs, filterMonth)
		sumArgIdx++
	}
	_ = database.DB.QueryRow(ctx, sumQuery, sumArgs...).Scan(&totalHours)

	var attLogs []AttendanceView
	var attQuery string
	var attArgs []interface{}
	attArgIdx := 1

	if role == "ADMIN" && filterEmployee == "" {
		attQuery = "SELECT id, employee_code, check_in, check_out, shamsi_date FROM attendance WHERE 1=1 "
	} else {
		attQuery = "SELECT id, employee_code, check_in, check_out, shamsi_date FROM attendance WHERE employee_code = $1 "
		attArgs = append(attArgs, targetFilterUser)
		attArgIdx++
	}

	if filterMonth != "" {
		attQuery += fmt.Sprintf("AND split_part(shamsi_date, '/', 2) = $%d ", attArgIdx)
		attArgs = append(attArgs, filterMonth)
	}
	attQuery += "ORDER BY id DESC;"

	aRows, err := database.DB.Query(ctx, attQuery, attArgs...)
	if err == nil {
		defer aRows.Close()
		for aRows.Next() {
			var av AttendanceView
			var tIn, tOut *time.Time
			if err := aRows.Scan(&av.ID, &av.EmployeeCode, &tIn, &tOut, &av.ShamsiDate); err == nil {
				if tIn != nil { av.CheckIn = tIn.In(time.Local).Format("15:04:05") }
				if tOut != nil {
					av.CheckOut = tOut.In(time.Local).Format("15:04:05")
					diff := tOut.Sub(*tIn)
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
		eRows, _ := database.DB.Query(ctx, "SELECT id, employee_code, full_name, role FROM employees ORDER BY id DESC;")
		if eRows != nil {
			defer eRows.Close()
			for eRows.Next() {
				var ev EmployeeView
				if err := eRows.Scan(&ev.ID, &ev.EmployeeCode, &ev.FullName, &ev.Role); err == nil {
					employees = append(employees, ev)
				}
			}
		}
	}

	var projects []ProjectView
	pRows, _ := database.DB.Query(ctx, "SELECT id, name FROM projects ORDER BY id ASC;")
	if pRows != nil {
		defer pRows.Close()
		for pRows.Next() {
			var pv ProjectView
			if err := pRows.Scan(&pv.ID, &pv.Name); err == nil {
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
		TotalAttendanceMonthStr: FormatDuration(totalHours) + " ساعت",
		TotalAttendanceDayStr:   todayStr,
		SelectedProfile:         selectedProfile,
		LiveStatuses:            liveStatuses,
		PieLabelsJSON:           template.JS(plJSON),
		PieDataJSON:             template.JS(pdJSON),
		BarLabelsJSON:           template.JS(blJSON),
		BarDataJSON:             template.JS(bdJSON),
		RecentTasks:             recentTasks,
		ActiveProjects:          activeProjects,
		SelectedMonthsMap:       selectedMonthsMap,
	}

	var targetTemplateFile string = "templates/employee.html"
	if role == "ADMIN" {
		targetTemplateFile = "templates/admin.html"
	}

	tmpl, err := template.ParseFiles(targetTemplateFile)
	if err != nil {
		http.Error(w, "خطای قالب: "+err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, data)
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
	http.SetCookie(w, &http.Cookie{Name: "session_user", Value: "", Expires: time.Now().Add(-1 * time.Hour), HttpOnly: true, Path: "/"})
	setFlashMessage(w, "از سیستم خارج شدید.")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleCheckIn(w http.ResponseWriter, r *http.Request) {
	username, _ := getAuthenticatedUser(r)
	tab := r.FormValue("tab")
	if tab == "" { tab = "attendance" }
	if username != "" {
		err := attendance.CheckIn(username)
		if err != nil { setFlashMessage(w, "⚠️ خطا: "+err.Error()) } else { setFlashMessage(w, "ورود زنده شما با موفقیت ثبت شد.") }
	}
	http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
}

func handleCheckOut(w http.ResponseWriter, r *http.Request) {
	username, _ := getAuthenticatedUser(r)
	tab := r.FormValue("tab")
	if tab == "" { tab = "attendance" }
	if username != "" {
		err := attendance.CheckOut(username)
		if err != nil { setFlashMessage(w, "⚠️ خطا: "+err.Error()) } else { setFlashMessage(w, "خروج زنده با موفقیت ثبت شد.") }
	}
	http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
}

func handleLogWork(w http.ResponseWriter, r *http.Request) {
	username, _ := getAuthenticatedUser(r)
	tab := r.FormValue("tab")
	if tab == "" { tab = "worklog" }
	if username != "" {
		pID, _ := strconv.Atoi(r.FormValue("project_id"))
		hours := ParseDuration(r.FormValue("hours"))
		desc := r.FormValue("description")
		sDate := r.FormValue("shamsi_date")

		err := project.LogWorkWithDate(username, pID, hours, desc, sDate)
		if err != nil { setFlashMessage(w, "❌ خطا: "+err.Error()) } else { setFlashMessage(w, "گزارش کارکرد با موفقیت ثبت شد.") }
	}
	http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
}

func handleManualAttendance(w http.ResponseWriter, r *http.Request) {
	username, role := getAuthenticatedUser(r)
	tab := r.FormValue("tab")
	if tab == "" { tab = "attendance" }
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
			attID, _ := strconv.Atoi(attIDStr)
			_, _ = database.DB.Exec(context.Background(), "DELETE FROM attendance WHERE id = $1;", attID)
		}

		err := attendance.AddManualAttendance(target, sDate, tIn, tOut)
		if err != nil { setFlashMessage(w, "خطا: "+err.Error()) } else {
			if attIDStr != "" { setFlashMessage(w, "تردد با موفقیت ویرایش و جایگزین شد.") } else { setFlashMessage(w, "تردد اصلاحی با موفقیت ثبت شد.") }
		}
	}
	http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
}

func handleEditWorkLog(w http.ResponseWriter, r *http.Request) {
	username, _ := getAuthenticatedUser(r)
	tab := r.FormValue("tab")
	if tab == "" { tab = "worklog" }
	if username != "" {
		logID, _ := strconv.Atoi(r.FormValue("log_id"))
		pID, _ := strconv.Atoi(r.FormValue("project_id"))
		hours := ParseDuration(r.FormValue("hours"))
		desc := r.FormValue("description")
		sDate := r.FormValue("shamsi_date")

		err := project.UpdateWorkLog(logID, pID, hours, desc, sDate)
		if err != nil { setFlashMessage(w, "❌ خطا: "+err.Error()) } else { setFlashMessage(w, "گزارش کارکرد ویرایش شد.") }
	}
	http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
}

func handleDeleteWorkLog(w http.ResponseWriter, r *http.Request) {
	username, _ := getAuthenticatedUser(r)
	tab := r.URL.Query().Get("tab")
	if tab == "" { tab = "worklog" }
	if username != "" {
		logID, _ := strconv.Atoi(r.URL.Query().Get("id"))
		_ = project.DeleteWorkLog(logID)
		setFlashMessage(w, "رکورد کارکرد حذف شد.")
	}
	http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
}

func handleDeleteAttendance(w http.ResponseWriter, r *http.Request) {
	username, _ := getAuthenticatedUser(r)
	tab := r.URL.Query().Get("tab")
	if tab == "" { tab = "attendance" }
	if username != "" {
		attID, _ := strconv.Atoi(r.URL.Query().Get("id"))
		_, _ = database.DB.Exec(context.Background(), "DELETE FROM attendance WHERE id = $1;", attID)
		setFlashMessage(w, "تردد مورد نظر با موفقیت حذف گردید.")
	}
	http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
}

func handleAddEmployee(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" { return }
	if r.Method == http.MethodPost {
		empCode := strings.ToUpper(strings.TrimSpace(r.FormValue("emp_code")))
		if empCode == "" { empCode = strings.ToUpper(strings.TrimSpace(r.FormValue("employee_code"))) }
		password := r.FormValue("password")
		fullName := strings.TrimSpace(r.FormValue("full_name"))
		uRole := r.FormValue("role")

		hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		_, err := database.DB.Exec(context.Background(), "INSERT INTO employees (employee_code, full_name, password, role) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING;", empCode, fullName, string(hash), uRole)
		if err != nil { setFlashMessage(w, "خطا: "+err.Error()) } else { setFlashMessage(w, "نیروی جدید استخدام شد.") }
	}
	http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
}

func handleEditEmployee(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" { return }
	if r.Method == http.MethodPost {
		ctx := context.Background()
		origCode := strings.ToUpper(strings.TrimSpace(r.FormValue("original_emp_code")))
		newCode := strings.ToUpper(strings.TrimSpace(r.FormValue("emp_code")))
		if newCode == "" { newCode = origCode }

		var existingFullName string
		_ = database.DB.QueryRow(ctx, "SELECT full_name FROM employees WHERE employee_code = $1;", origCode).Scan(&existingFullName)

		fullName := r.FormValue("full_name")
		if fullName == "" { fullName = existingFullName }
		uRole := r.FormValue("role")
		password := r.FormValue("password")

		if newCode != origCode && origCode != "" {
			_, errUpdate := database.DB.Exec(ctx, "UPDATE employees SET employee_code = $1 WHERE employee_code = $2;", newCode, origCode)
			if errUpdate == nil {
				database.DB.Exec(ctx, "UPDATE work_logs SET employee_code = $1 WHERE employee_code = $2;", newCode, origCode)
				database.DB.Exec(ctx, "UPDATE attendance SET employee_code = $1 WHERE employee_code = $2;", newCode, origCode)
				database.DB.Exec(ctx, "UPDATE employee_profiles SET employee_code = $1 WHERE employee_code = $2;", newCode, origCode)
			}
		}

		if password != "" {
			hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			_, err := database.DB.Exec(ctx, "UPDATE employees SET full_name = $1, role = $2, password = $3 WHERE employee_code = $4;", fullName, uRole, string(hash), newCode)
			if err != nil { setFlashMessage(w, "خطا: "+err.Error()) } else { setFlashMessage(w, "مشخصات و رمز عبور پرسنل بروزرسانی شد.") }
		} else {
			_, err := database.DB.Exec(ctx, "UPDATE employees SET full_name = $1, role = $2 WHERE employee_code = $3;", fullName, uRole, newCode)
			if err != nil { setFlashMessage(w, "خطا: "+err.Error()) } else { setFlashMessage(w, "مشخصات پرسنل با موفقیت بروزرسانی شد.") }
		}
	}
	http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
}

func handleDeleteEmployee(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" { return }
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	_, _ = database.DB.Exec(context.Background(), "DELETE FROM employees WHERE id = $1;", id)
	setFlashMessage(w, "نیرو حذف گردید.")
	http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
}

func handleCreateProject(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" { return }
	if r.Method == http.MethodPost {
		name := strings.TrimSpace(r.FormValue("project_name"))
		if name == "" { name = strings.TrimSpace(r.FormValue("name")) }
		if name != "" {
			_, err := database.DB.Exec(context.Background(), "INSERT INTO projects (name) VALUES ($1) ON CONFLICT DO NOTHING;", name)
			if err != nil { setFlashMessage(w, "خطا: "+err.Error()) } else { setFlashMessage(w, "✅ پروژه جدید با موفقیت ایجاد شد.") }
		}
	}
	http.Redirect(w, r, "/?tab=projects", http.StatusSeeOther)
}

func handleEditProject(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" { return }
	if r.Method == http.MethodPost {
		id, _ := strconv.Atoi(r.FormValue("project_id"))
		newName := r.FormValue("new_project_name")
		_, _ = database.DB.Exec(context.Background(), "UPDATE projects SET name = $1 WHERE id = $2;", newName, id)
		setFlashMessage(w, "نام پروژه اصلاح شد.")
	}
	http.Redirect(w, r, "/?tab=projects", http.StatusSeeOther)
}

func handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" { return }
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	_, _ = database.DB.Exec(context.Background(), "DELETE FROM projects WHERE id = $1;", id)
	setFlashMessage(w, "پروژه حذف شد.")
	http.Redirect(w, r, "/?tab=projects", http.StatusSeeOther)
}

func handleSavePayrollProfile(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" { return }
	if r.Method == http.MethodPost {
		empCode := strings.ToUpper(strings.TrimSpace(r.FormValue("employee_code")))
		cType := r.FormValue("contract_type")
		isMarried := r.FormValue("is_married") == "true"
		childCount, _ := strconv.Atoi(r.FormValue("child_count"))
		eligibleSeniority := r.FormValue("eligible_for_seniority") == "true"
		overtimeRate, _ := strconv.ParseInt(r.FormValue("custom_overtime_rate"), 10, 64)
		hourlyRate, _ := strconv.ParseInt(r.FormValue("hourly_rate"), 10, 64)
		leaveHours, _ := strconv.ParseFloat(r.FormValue("remaining_leave_hours"), 64)
		nationalCode := strings.TrimSpace(r.FormValue("national_code"))
		phoneNumber := strings.TrimSpace(r.FormValue("phone_number"))
		bankCard := strings.TrimSpace(r.FormValue("bank_card"))
		sheba := strings.TrimSpace(r.FormValue("sheba"))

		query := `INSERT INTO employee_profiles (
					employee_code, contract_type, is_married, child_count, eligible_for_seniority, 
					custom_overtime_rate, hourly_rate, remaining_leave_hours, national_code, phone_number, bank_card_number, sheba_number
				  ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) 
				  ON CONFLICT (employee_code) DO UPDATE SET 
					contract_type=EXCLUDED.contract_type, is_married=EXCLUDED.is_married, child_count=EXCLUDED.child_count, 
					eligible_for_seniority=EXCLUDED.eligible_for_seniority, custom_overtime_rate=EXCLUDED.custom_overtime_rate,
					hourly_rate=EXCLUDED.hourly_rate, remaining_leave_hours=EXCLUDED.remaining_leave_hours,
					national_code=EXCLUDED.national_code, phone_number=EXCLUDED.phone_number, bank_card_number=EXCLUDED.bank_card_number, sheba_number=EXCLUDED.sheba_number;`

		_, err := database.DB.Exec(context.Background(), query, empCode, cType, isMarried, childCount, eligibleSeniority, overtimeRate, hourlyRate, leaveHours, nationalCode, phoneNumber, bankCard, sheba)
		if err != nil { setFlashMessage(w, "❌ خطا در ذخیره پروفایل مالی: "+err.Error()) } else { setFlashMessage(w, "✅ پروفایل حقوقی و اطلاعات بانکی پرسنل با موفقیت ذخیره شد.") }
	}
	http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
}

func handleIssuePayroll(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" { return }
	if r.Method == http.MethodPost {
		empCode := strings.ToUpper(strings.TrimSpace(r.FormValue("employee_code")))
		year, _ := strconv.Atoi(r.FormValue("year"))
		month, _ := strconv.Atoi(r.FormValue("month"))
		actualHours := ParseDuration(r.FormValue("actual_hours"))
		expectedHours := ParseDuration(r.FormValue("expected_hours"))
		overtimeHours := ParseDuration(r.FormValue("overtime_hours"))
		slip, err := payroll.IssueMonthlyPayroll(context.Background(), empCode, year, month, actualHours, expectedHours, overtimeHours)
		if err != nil { setFlashMessage(w, "❌ خطا: "+err.Error()) } else { setFlashMessage(w, fmt.Sprintf("فیش حقوقی صادر شد. خالص دریافتی: %d", slip.NetPayout)) }
	}
	http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
}

func handleExportExcel(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	username, role := getAuthenticatedUser(r)
	if username == "" { return }
	filterEmp := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("employee_code")))
	filterProj := r.URL.Query().Get("project_id")
	filterMonth := r.URL.Query().Get("filter_month")
	targetExcelUser := filterEmp
	if role != "ADMIN" { targetExcelUser = username }

	logQuery := `SELECT w.employee_code, w.shamsi_date, p.name, COALESCE(w.hours_spent, 0), COALESCE(w.description, '') FROM work_logs w JOIN projects p ON w.project_id = p.id WHERE 1=1 `
	var args []interface{}
	argIdx := 1
	if role != "ADMIN" || targetExcelUser != "" {
		logQuery += fmt.Sprintf("AND w.employee_code = $%d ", argIdx)
		args = append(args, targetExcelUser)
		argIdx++
	}
	if filterProj != "" {
		pID, _ := strconv.Atoi(filterProj)
		logQuery += fmt.Sprintf("AND w.project_id = $%d ", argIdx)
		args = append(args, pID)
		argIdx++
	}
	if filterMonth != "" {
		logQuery += fmt.Sprintf("AND split_part(w.shamsi_date, '/', 2) = $%d ", argIdx)
		args = append(args, filterMonth)
	}
	logQuery += "ORDER BY w.shamsi_date DESC;"
	rows, err := database.DB.Query(ctx, logQuery, args...)
	if err != nil { return }
	defer rows.Close()

	f := excelize.NewFile()
	sheetName := "گزارش ماتریسی کارکرد"
	f.SetSheetName("Sheet1", sheetName)
	f.SetCellValue(sheetName, "A1", "کد کارمند")
	f.SetCellValue(sheetName, "B1", "تاریخ شمسی")
	f.SetCellValue(sheetName, "C1", "نام پروژه")
	f.SetCellValue(sheetName, "D1", "مدت زمان")
	f.SetCellValue(sheetName, "E1", "شرح")

	rowIdx := 2
	for rows.Next() {
		var emp, sDate, pName, desc string
		var hours float64
		if err := rows.Scan(&emp, &sDate, &pName, &hours, &desc); err == nil {
			f.SetCellValue(sheetName, fmt.Sprintf("A%d", rowIdx), emp)
			f.SetCellValue(sheetName, fmt.Sprintf("B%d", rowIdx), sDate)
			f.SetCellValue(sheetName, fmt.Sprintf("C%d", rowIdx), pName)
			f.SetCellValue(sheetName, fmt.Sprintf("D%d", rowIdx), FormatDuration(hours))
			f.SetCellValue(sheetName, fmt.Sprintf("E%d", rowIdx), desc)
			rowIdx++
		}
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=shamsi_matrix_report.xlsx")
	f.WriteTo(w)
}