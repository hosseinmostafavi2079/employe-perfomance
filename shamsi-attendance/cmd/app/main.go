package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url" 
	"strings" 
	"strconv"
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

type WorkLogView struct {
	ID           int
	EmployeeCode string
	ProjectID    int
	ProjectName  string
	HoursSpent   float64
	Description  string
	ShamsiDate   string
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

type PageData struct {
	IsLoggedIn              bool
	CurrentDate           string
	TotalHours            float64
	Message               string
	CurrentUser           string
	CurrentRole           string
	SelectedFilter        string
	SelectedProjectFilter string
	SelectedMonthFilter   string
	CurrentTab            string
	WorkLogs              []WorkLogView
	AttendanceLogs        []AttendanceView
	Employees             []EmployeeView 
	Projects              []ProjectView  
	EditLog               *WorkLogView
	EditAttendanceLog     *AttendanceView 
	TotalAttendanceMonthStr string          
	TotalAttendanceDayStr   string          
	SelectedProfile         *FinancialProfileView
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

	fmt.Println("🚀 وب‌سرور امن تفکیک‌شده با موفقیت روشن شد!")
	fmt.Println("🌐 آدرس ورود به سامانه: http://localhost:8080")
	fmt.Println("==================================================")
	log.Fatal(http.ListenAndServe(":8080", nil))
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

	log.Printf("[LOG-SYSTEM] Request received. User: %s | Role: %s | Tab: %s", username, role, r.URL.Query().Get("tab"))

	if username == "" {
		tmpl, err := template.ParseFiles("templates/login.html")
		if err != nil {
			http.Error(w, "Error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, PageData{IsLoggedIn: false, Message: flashMsg, CurrentDate: attendance.GetCurrentShamsiDate()})
		return
	}

	editIDParam := r.URL.Query().Get("edit_id")
	editAttIDParam := r.URL.Query().Get("edit_attendance_id")
	editFinancialCode := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("edit_financial_code")))
	filterEmployee := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("filter_employee")))
	filterProject := r.URL.Query().Get("filter_project")
	filterMonth := r.URL.Query().Get("filter_month")
	tabParam := r.URL.Query().Get("tab")

	currentTab := "attendance"
	if tabParam == "worklog" {
		currentTab = "worklog"
	} else if tabParam == "management" && role == "ADMIN" {
		currentTab = "management"
	} else if tabParam == "projects" && role == "ADMIN" {
		currentTab = "projects" 
	} else if tabParam == "project_report" && role == "ADMIN" {
		currentTab = "project_report"
	}

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
		err := database.DB.QueryRow(ctx, "SELECT id, employee_code, project_id, hours_spent, description, shamsi_date FROM work_logs WHERE id=$1;", eID).
			Scan(&ev.ID, &ev.EmployeeCode, &ev.ProjectID, &ev.HoursSpent, &ev.Description, &ev.ShamsiDate)
		if err == nil {
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
	logQuery := `SELECT w.id, w.employee_code, w.project_id, p.name, w.hours_spent, w.description, w.shamsi_date 
                 FROM work_logs w JOIN projects p ON w.project_id = p.id WHERE 1=1 `
	var args []interface{}
	argIdx := 1

	if role != "ADMIN" || targetFilterUser != "" {
		logQuery += fmt.Sprintf("AND w.employee_code = $%d ", argIdx)
		args = append(args, targetFilterUser)
		argIdx++
	}
	if filterProject != "" {
		pID, _ := strconv.Atoi(filterProject)
		logQuery += fmt.Sprintf("AND w.project_id = $%d ", argIdx)
		args = append(args, pID)
		argIdx++
	}
	if filterMonth != "" {
		logQuery += fmt.Sprintf("AND split_part(w.shamsi_date, '/', 2) = $%d ", argIdx)
		args = append(args, filterMonth)
		argIdx++
	}
	logQuery += "ORDER BY w.shamsi_date DESC, w.id DESC;"

	rows, err := database.DB.Query(ctx, logQuery, args...)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var wl WorkLogView
			if err := rows.Scan(&wl.ID, &wl.EmployeeCode, &wl.ProjectID, &wl.ProjectName, &wl.HoursSpent, &wl.Description, &wl.ShamsiDate); err == nil {
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
		IsLoggedIn:            true,
		CurrentDate:           attendance.GetCurrentShamsiDate(),
		TotalHours:            totalHours,
		Message:               flashMsg,
		CurrentUser:           username,
		CurrentRole:           role,
		SelectedFilter:        filterEmployee,
		SelectedProjectFilter: filterProject,
		SelectedMonthFilter:   filterMonth,
		CurrentTab:            currentTab,
		WorkLogs:              workLogs,
		AttendanceLogs:        attLogs,
		Employees:             employees, 
		Projects:              projects,  
		EditLog:               editLog,
		EditAttendanceLog:     editAttLog,
		TotalAttendanceMonthStr: fmt.Sprintf("%.2f ساعت", totalHours),
		TotalAttendanceDayStr:   todayStr,
		SelectedProfile:       selectedProfile,
	}

	var targetTemplateFile string = "templates/employee.html"
	if role == "ADMIN" { targetTemplateFile = "templates/admin.html" }

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
		hours, _ := strconv.ParseFloat(r.FormValue("hours"), 64)
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

		target := username
		if role == "ADMIN" && r.FormValue("target_employee") != "" {
			target = strings.ToUpper(strings.TrimSpace(r.FormValue("target_employee")))
		}

		err := attendance.AddManualAttendance(target, sDate, tIn, tOut)
		if err != nil { setFlashMessage(w, "خطا: " + err.Error()) } else { setFlashMessage(w, "تردد اصلاحی با موفقیت ثبت شد.") }
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
		hours, _ := strconv.ParseFloat(r.FormValue("hours"), 64)
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
		empCode := strings.ToUpper(strings.TrimSpace(r.FormValue("emp_code")))
		fullName := r.FormValue("full_name")
		uRole := r.FormValue("role")
		_, err := database.DB.Exec(context.Background(), "UPDATE employees SET full_name = $1, role = $2 WHERE employee_code = $3;", fullName, uRole, empCode)
		if err != nil { setFlashMessage(w, "خطا: "+err.Error()) } else { setFlashMessage(w, "مشخصات پرسنل بروزرسانی شد.") }
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
			if err != nil {
				setFlashMessage(w, "خطا: "+err.Error())
			} else {
				setFlashMessage(w, "✅ پروژه جدید با موفقیت ایجاد شد.")
				log.Printf("[LOG-SYSTEM] Project created in database: %s", name)
			}
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
					custom_overtime_rate, hourly_rate, remaining_leave_hours, 
					national_code, phone_number, bank_card_number, sheba_number
				  ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) 
				  ON CONFLICT (employee_code) DO UPDATE SET 
					contract_type=EXCLUDED.contract_type, 
					is_married=EXCLUDED.is_married, 
					child_count=EXCLUDED.child_count, 
					eligible_for_seniority=EXCLUDED.eligible_for_seniority,
					custom_overtime_rate=EXCLUDED.custom_overtime_rate,
					hourly_rate=EXCLUDED.hourly_rate,
					remaining_leave_hours=EXCLUDED.remaining_leave_hours,
					national_code=EXCLUDED.national_code,
					phone_number=EXCLUDED.phone_number,
					bank_card_number=EXCLUDED.bank_card_number,
					sheba_number=EXCLUDED.sheba_number;`
		
		_, err := database.DB.Exec(context.Background(), query, empCode, cType, isMarried, childCount, eligibleSeniority, overtimeRate, hourlyRate, leaveHours, nationalCode, phoneNumber, bankCard, sheba)
		if err != nil {
			setFlashMessage(w, "❌ خطا در ذخیره پروفایل مالی: " + err.Error())
		} else {
			setFlashMessage(w, "✅ پروفایل حقوقی و اطلاعات بانکی پرسنل با موفقیت ذخیره شد.")
		}
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
		actualHours, _ := strconv.ParseFloat(r.FormValue("actual_hours"), 64)
		expectedHours, _ := strconv.ParseFloat(r.FormValue("expected_hours"), 64)
		overtimeHours, _ := strconv.ParseFloat(r.FormValue("overtime_hours"), 64)
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

	logQuery := `SELECT w.employee_code, w.shamsi_date, p.name, w.hours_spent, w.description FROM work_logs w JOIN projects p ON w.project_id = p.id WHERE 1=1 `
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
			f.SetCellValue(sheetName, fmt.Sprintf("D%d", rowIdx), hours)
			f.SetCellValue(sheetName, fmt.Sprintf("E%d", rowIdx), desc)
			rowIdx++
		}
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=shamsi_matrix_report.xlsx")
	f.WriteTo(w)
}