package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"shamsi_attendance/internal/attendance"
	"shamsi_attendance/internal/database"
	"shamsi_attendance/internal/project"

	"github.com/xuri/excelize/v2"
	"golang.org/x/crypto/bcrypt"
)

// ثوابت قانون کار سال ۱۴۰۵ و مرخصی ماهانه
const (
	DailyBaseWage1405      int64   = 5541850
	DailySeniority1405     int64   = 166667
	MonthlyBon1405         int64   = 22000000
	MonthlyHousing1405     int64   = 30000000
	MonthlyMarital1405     int64   = 5000000
	MonthlyChildPerOne1405 int64   = 16625550
	MonthlyLeaveAccrual    float64 = 20.0 // ۲۰ ساعت مرخصی ماهانه
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

// ساختارهای جدید جهت نمایش لیست‌ها در ادمین
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

type PageData struct {
	IsLoggedIn            bool
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
	Employees             []EmployeeView // ⭐️ تزریق به قالب
	Projects              []ProjectView  // ⭐️ تزریق به قالب
	EditLog               *WorkLogView
}

var systemMessage string = ""

func main() {
	fmt.Println("==================================================")
	fmt.Println("در حال راه‌اندازی وب‌سرور حرفه‌ای و تجاری سامانه...")
	fmt.Println("==================================================")

	database.ConnectToDatabase()
	if database.DB != nil {
		defer database.DB.Close()
	}

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

	// روت‌های ماژول حقوق و دستمزد اختصاصی مدیریت
	http.HandleFunc("/admin/payroll/save-profile", handleSavePayrollProfile)
	http.HandleFunc("/admin/payroll/issue", handleIssuePayroll)

	// ⭐️ روت‌های جدید عملیات مدیریت، ویرایش و حذف پرسنل و پروژه‌ها
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
	ctx := context.Background()
	var role string
	err = database.DB.QueryRow(ctx, "SELECT role FROM employees WHERE employee_code = $1;", cookie.Value).Scan(&role)
	if err != nil {
		return "", ""
	}
	return cookie.Value, role
}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	username, role := getAuthenticatedUser(r)

	if username == "" {
		tmpl, err := template.ParseFiles("templates/login.html")
		if err != nil {
			http.Error(w, "خطا در بارگذاری قالب ورود: "+err.Error(), http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, PageData{IsLoggedIn: false, Message: systemMessage, CurrentDate: attendance.GetCurrentShamsiDate()})
		systemMessage = ""
		return
	}

	editIDParam := r.URL.Query().Get("edit_id")
	filterEmployee := r.URL.Query().Get("filter_employee")
	filterProject := r.URL.Query().Get("filter_project")
	filterMonth := r.URL.Query().Get("filter_month")
	tabParam := r.URL.Query().Get("tab")

	currentTab := "attendance"
	if tabParam == "worklog" {
		currentTab = "worklog"
	} else if tabParam == "management" && role == "ADMIN" {
		currentTab = "management"
	} else if tabParam == "projects" && role == "ADMIN" {
		currentTab = "projects" // ⭐️ اصلاح منطق کلیک روی تب مدیریت پروژه‌ها
	} else if tabParam == "project_report" && role == "ADMIN" {
		currentTab = "project_report"
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
	}
	_ = database.DB.QueryRow(ctx, sumQuery, sumArgs...).Scan(&totalHours)

	var attLogs []AttendanceView
	var attQuery string
	var attArgs []interface{}
	attArgIdx := 1

	if role == "ADMIN" && filterEmployee == "" {
		attQuery = "SELECT id, employee_code, check_in, check_out, shamsi_date FROM attendance WHERE 1=1 "
	} else if role == "ADMIN" && filterEmployee != "" {
		attQuery = "SELECT id, employee_code, check_in, check_out, shamsi_date FROM attendance WHERE employee_code = $1 "
		attArgs = append(attArgs, filterEmployee)
		attArgIdx++
	} else {
		attQuery = "SELECT id, employee_code, check_in, check_out, shamsi_date FROM attendance WHERE employee_code = $1 "
		attArgs = append(attArgs, username)
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
				if tIn != nil {
					av.CheckIn = tIn.In(time.Local).Format("15:04:05")
				}
				if tOut != nil {
					av.CheckOut = tOut.In(time.Local).Format("15:04:05")
					diff := tOut.Sub(*tIn)
					h := int(diff.Hours())
					m := int(diff.Minutes()) % 60
					av.Duration = fmt.Sprintf("%d ساعت و %d دقیقه", h, m)
				} else {
					av.Duration = "حضور زنده فعال"
				}
				attLogs = append(attLogs, av)
			}
		}
	}

	// ⭐️ واکشی داینامیک کل پرسنل ارگان از دیتابیس برای پنل ادمین
	var employees []EmployeeView
	if role == "ADMIN" {
		eRows, err := database.DB.Query(ctx, "SELECT id, employee_code, full_name, role FROM employees ORDER BY id DESC;")
		if err == nil {
			defer eRows.Close()
			for eRows.Next() {
				var ev EmployeeView
				if err := eRows.Scan(&ev.ID, &ev.EmployeeCode, &ev.FullName, &ev.Role); err == nil {
					employees = append(employees, ev)
				}
			}
		}
	}

	// ⭐️ واکشی داینامیک کل پروژه‌های فعال جهت رندر روی فرم‌ها و جدول پروژه
	var projects []ProjectView
	pRows, err := database.DB.Query(ctx, "SELECT id, name FROM projects ORDER BY id ASC;")
	if err == nil {
		defer pRows.Close()
		for pRows.Next() {
			var pv ProjectView
			if err := pRows.Scan(&pv.ID, &pv.Name); err == nil {
				projects = append(projects, pv)
			}
		}
	}

	data := PageData{
		IsLoggedIn:            true,
		CurrentDate:           attendance.GetCurrentShamsiDate(),
		TotalHours:            totalHours,
		Message:               systemMessage,
		CurrentUser:           username,
		CurrentRole:           role,
		SelectedFilter:        filterEmployee,
		SelectedProjectFilter: filterProject,
		SelectedMonthFilter:   filterMonth,
		CurrentTab:            currentTab,
		WorkLogs:              workLogs,
		AttendanceLogs:        attLogs,
		Employees:             employees, // تزریق به تمپلیت ادمین
		Projects:              projects,  // تزریق به تمپلیت ادمین
		EditLog:               editLog,
	}
	systemMessage = ""

	var targetTemplateFile string = "templates/employee.html"
	if role == "ADMIN" {
		targetTemplateFile = "templates/admin.html"
	}

	tmpl, err := template.ParseFiles(targetTemplateFile)
	if err != nil {
		http.Error(w, "خطای رندر تمپلیت: "+err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, data)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		username := r.FormValue("username")
		password := r.FormValue("password")
		ctx := context.Background()

		var dbUser, hashedPassword string
		err := database.DB.QueryRow(ctx, "SELECT employee_code, password FROM employees WHERE employee_code = $1;", username).Scan(&dbUser, &hashedPassword)
		
		// بررسی تطابق رمز عبور با متد ایمن Bcrypt
		if err == nil && bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password)) == nil {
			http.SetCookie(w, &http.Cookie{
				Name:     "session_user",
				Value:    username,
				Expires:  time.Now().Add(24 * time.Hour),
				HttpOnly: true,
				Path:     "/",
			})
			systemMessage = "ورود با موفقیت انجام شد."
		} else {
			systemMessage = "خطا: نام کاربری یا کلمه عبور اشتباه است!"
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
	systemMessage = "از سیستم خارج شدید."
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
			systemMessage = "⚠️ خطا: " + err.Error()
		} else {
			systemMessage = "ورود زنده شما با موفقیت ثبت شد."
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
			systemMessage = "⚠️ خطا: " + err.Error()
		} else {
			systemMessage = "خروج زنده با موفقیت ثبت شد."
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
		pID, _ := strconv.Atoi(r.FormValue("project_id"))
		hours, _ := strconv.ParseFloat(r.FormValue("hours"), 64)
		desc := r.FormValue("description")
		sDate := r.FormValue("shamsi_date")

		_ = project.LogWorkWithDate(username, pID, hours, desc, sDate)
		systemMessage = "گزارش کارکرد با موفقیت ثبت شد."
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

		target := username
		if role == "ADMIN" && r.FormValue("target_employee") != "" {
			target = r.FormValue("target_employee")
		}

		err := attendance.AddManualAttendance(target, sDate, tIn, tOut)
		if err != nil {
			systemMessage = "خطا در تنظیم زمان دستی: " + err.Error()
		} else {
			systemMessage = "تردد اصلاحی با موفقیت ثبت شد."
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
		logID, _ := strconv.Atoi(r.FormValue("log_id"))
		pID, _ := strconv.Atoi(r.FormValue("project_id"))
		hours, _ := strconv.ParseFloat(r.FormValue("hours"), 64)
		desc := r.FormValue("description")
		sDate := r.FormValue("shamsi_date")

		_ = project.UpdateWorkLog(logID, pID, hours, desc, sDate)
		systemMessage = "رکورد کارکرد روزانه شما با موفقیت به روزرسانی شد."
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
		logID, _ := strconv.Atoi(r.URL.Query().Get("id"))
		_ = project.DeleteWorkLog(logID)
		systemMessage = "رکورد کارکرد حذف شد."
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
		attID, _ := strconv.Atoi(r.URL.Query().Get("id"))
		ctx := context.Background()
		_, err := database.DB.Exec(ctx, "DELETE FROM attendance WHERE id = $1;", attID)
		if err != nil {
			systemMessage = "خطا در حذف تردد: " + err.Error()
		} else {
			systemMessage = "تردد مورد نظر با موفقیت حذف گردید."
		}
	}
	http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
}

func handleAddEmployee(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" {
		http.Error(w, "عدم دسترسی کاربری مجاز", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodPost {
		empCode := r.FormValue("emp_code")
		password := r.FormValue("password")
		fullName := r.FormValue("full_name")
		uRole := r.FormValue("role")

		// هش کردن گذرواژه کاربر جدید به روش Bcrypt
		hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

		ctx := context.Background()
		query := "INSERT INTO employees (employee_code, full_name, password, role) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING;"
		_, err := database.DB.Exec(ctx, query, empCode, fullName, string(hash), uRole)
		if err != nil {
			systemMessage = "خطا در ثبت پرسنل جدید: " + err.Error()
		} else {
			systemMessage = fmt.Sprintf("موفقیت: نیروی جدید با نام «%s» استخدام شد.", fullName)
		}
	}
	http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
}

// ⭐️ هندلر جدید: ویرایش مشخصات پرسنل همراه با مدیریت تغییر فیلد پسورد به روش Bcrypt
func handleEditEmployee(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodPost {
		empCode := r.FormValue("emp_code")
		fullName := r.FormValue("full_name")
		password := r.FormValue("password")
		uRole := r.FormValue("role")
		ctx := context.Background()

		var err error
		if password != "" {
			hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			_, err = database.DB.Exec(ctx, "UPDATE employees SET full_name = $1, password = $2, role = $3 WHERE employee_code = $4;", fullName, string(hash), uRole, empCode)
		} else {
			_, err = database.DB.Exec(ctx, "UPDATE employees SET full_name = $1, role = $2 WHERE employee_code = $3;", fullName, uRole, empCode)
		}

		if err != nil {
			systemMessage = "❌ خطا در اصلاح اطلاعات کارمند: " + err.Error()
		} else {
			systemMessage = "✅ مشخصات و ساختار هویتی کارمند با موفقیت به روزرسانی شد."
		}
	}
	http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
}

// ⭐️ هندلر جدید: حذف کارمند از تشکیلات
func handleDeleteEmployee(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	ctx := context.Background()
	_, err := database.DB.Exec(ctx, "DELETE FROM employees WHERE id = $1;", id)
	if err != nil {
		systemMessage = "خطا در حذف نیرو: " + err.Error()
	} else {
		systemMessage = "✅ نیرو با موفقیت حذف گردید."
	}
	http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
}

// ⭐️ هندلر جدید: ساخت پروژه جدید
func handleCreateProject(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodPost {
		name := r.FormValue("project_name")
		ctx := context.Background()
		_, err := database.DB.Exec(ctx, "INSERT INTO projects (name) VALUES ($1) ON CONFLICT DO NOTHING;", name)
		if err != nil {
			systemMessage = "خطا در ایجاد پروژه: " + err.Error()
		} else {
			systemMessage = "✅ پروژه جدید با موفقیت در تشکیلات ایجاد شد."
		}
	}
	http.Redirect(w, r, "/?tab=projects", http.StatusSeeOther)
}

// ⭐️ هندلر جدید: ویرایش نام پروژه موجود
func handleEditProject(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodPost {
		id, _ := strconv.Atoi(r.FormValue("project_id"))
		newName := r.FormValue("new_project_name")
		ctx := context.Background()
		_, err := database.DB.Exec(ctx, "UPDATE projects SET name = $1 WHERE id = $2;", newName, id)
		if err != nil {
			systemMessage = "خطا در ویرایش عنوان پروژه: " + err.Error()
		} else {
			systemMessage = "✅ نام پروژه با موفقیت اصلاح گردید."
		}
	}
	http.Redirect(w, r, "/?tab=projects", http.StatusSeeOther)
}

// ⭐️ هندلر جدید: حذف کامل پروژه و ریز کارکردها
func handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	ctx := context.Background()
	_, err := database.DB.Exec(ctx, "DELETE FROM projects WHERE id = $1;", id)
	if err != nil {
		systemMessage = "خطا در تخریب پروژه: " + err.Error()
	} else {
		systemMessage = "✅ پروژه مورد نظر همراه با کل ریز گزارش کارکردهای متصل به آن حذف شد."
	}
	http.Redirect(w, r, "/?tab=projects", http.StatusSeeOther)
}

// هندلر تنظیم مشخصات فیش حقوقی در پروفایل مالی
func handleSavePayrollProfile(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodPost {
		empCode := r.FormValue("employee_code")
		cType := r.FormValue("contract_type")
		isMarried := r.FormValue("is_married") == "true"
		childCount, _ := strconv.Atoi(r.FormValue("child_count"))
		eligibleSeniority := r.FormValue("eligible_for_seniority") == "true"
		overtimeRate, _ := strconv.ParseInt(r.FormValue("custom_overtime_rate"), 10, 64)
		hourlyRate, _ := strconv.ParseInt(r.FormValue("hourly_rate"), 10, 64)
		leaveHours, _ := strconv.ParseFloat(r.FormValue("remaining_leave_hours"), 64)

		ctx := context.Background()
		query := `
			INSERT INTO employee_profiles (employee_code, contract_type, is_married, child_count, eligible_for_seniority, custom_overtime_rate, hourly_rate, remaining_leave_hours)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (employee_code) 
			DO UPDATE SET 
				contract_type = EXCLUDED.contract_type,
				is_married = EXCLUDED.is_married,
				child_count = EXCLUDED.child_count,
				eligible_for_seniority = EXCLUDED.eligible_for_seniority,
				custom_overtime_rate = EXCLUDED.custom_overtime_rate,
				hourly_rate = EXCLUDED.hourly_rate,
				remaining_leave_hours = EXCLUDED.remaining_leave_hours;`

		_, err := database.DB.Exec(ctx, query, empCode, cType, isMarried, childCount, eligibleSeniority, overtimeRate, hourlyRate, leaveHours)
		if err != nil {
			systemMessage = "❌ خطا در ذخیره پروفایل مالی: " + err.Error()
		} else {
			systemMessage = fmt.Sprintf("✅ مشخصات مالی و مرخصی پرسنل (%s) با موفقیت در دیتابیس داکر ثبت شد.", empCode)
		}
	}
	http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
}

// هندلر اصلی پردازش خودکار موتور مرخصی و محاسبات حقوق ۱۴۰۵
func handleIssuePayroll(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	if role != "ADMIN" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodPost {
		empCode := r.FormValue("employee_code")
		year, _ := strconv.Atoi(r.FormValue("year"))
		month, _ := strconv.Atoi(r.FormValue("month"))
		actualHours, _ := strconv.ParseFloat(r.FormValue("actual_hours"), 64)
		expectedHours, _ := strconv.ParseFloat(r.FormValue("expected_hours"), 64)
		overtimeHours, _ := strconv.ParseFloat(r.FormValue("overtime_hours"), 64)

		ctx := context.Background()

		var cType string
		var isMarried, eligibleSeniority bool
		var childCount int
		var overtimeRate, hourlyRate int64
		var remainingLeave float64

		err := database.DB.QueryRow(ctx,
			`SELECT contract_type, is_married, child_count, eligible_for_seniority, custom_overtime_rate, hourly_rate, remaining_leave_hours 
			 FROM employee_profiles WHERE employee_code = $1;`, empCode).
			Scan(&cType, &isMarried, &childCount, &eligibleSeniority, &overtimeRate, &hourlyRate, &remainingLeave)

		if err != nil {
			systemMessage = "❌ خطا: ابتدا باید مشخصات مالی این نیرو را در بخش مدیریت تنظیم نمایید!"
			http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
			return
		}

		updatedLeave := remainingLeave + MonthlyLeaveAccrual
		var leaveDeficit float64 = 0

		if actualHours < expectedHours {
			leaveDeficit = expectedHours - actualHours
			updatedLeave = updatedLeave - leaveDeficit
		}

		_, _ = database.DB.Exec(ctx, "UPDATE employee_profiles SET remaining_leave_hours = $1 WHERE employee_code = $2;", updatedLeave, empCode)

		var baseSalary, bon, housing, marital, child, seniority, overtimeIncome, gross, insurance, totalDeductions, net int64

		if cType == "REGULAR" {
			daysInMonth := 30
			if month <= 6 {
				daysInMonth = 31
			} else if month == 12 {
				daysInMonth = 29
			}

			baseSalary = DailyBaseWage1405 * int64(daysInMonth)
			bon = MonthlyBon1405
			housing = MonthlyHousing1405
			if isMarried {
				marital = MonthlyMarital1405
			}
			child = MonthlyChildPerOne1405 * int64(childCount)
			if eligibleSeniority {
				seniority = DailySeniority1405 * int64(daysInMonth)
			}
			overtimeIncome = int64(overtimeHours) * overtimeRate
			gross = baseSalary + bon + housing + marital + child + seniority + overtimeIncome

			insuredEarnings := baseSalary + bon + housing + marital + seniority + overtimeIncome
			insurance = int64(float64(insuredEarnings) * 0.07)
			totalDeductions = insurance
			net = gross - totalDeductions
		} else {
			baseSalary = int64(actualHours) * hourlyRate
			gross = baseSalary
			net = gross
		}

		insertQuery := `
			INSERT INTO payroll_slips (
				employee_code, year, month, expected_work_hours, actual_work_hours,
				base_salary, bon_allowance, housing_allowance, marital_allowance, child_allowance,
				seniority_allowance, overtime_income, gross_earnings, insurance_deduction,
				leave_deficit_hours, total_deductions, net_payout
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17);`

		_, err = database.DB.Exec(ctx, insertQuery,
			empCode, year, month, expectedHours, actualHours,
			baseSalary, bon, housing, marital, child,
			seniority, overtimeIncome, gross, insurance,
			leaveDeficit, totalDeductions, net)

		if err != nil {
			systemMessage = "❌ خطا در آرشیو فیش حقوقی: " + err.Error()
		} else {
			systemMessage = fmt.Sprintf("✅ فیش حقوقی با موفقیت صادر شد. خالص پرداختی: %d ریال. میزان %.1f ساعت کسر کارکرد خودکار اعمال شد.", net, leaveDeficit)
		}
	}
	http.Redirect(w, r, "/?tab=management", http.StatusSeeOther)
}

func handleExportExcel(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	username, role := getAuthenticatedUser(r)
	if username == "" {
		http.Error(w, "عدم احراز هویت", http.StatusUnauthorized)
		return
	}

	filterEmp := r.URL.Query().Get("employee_code")
	filterProj := r.URL.Query().Get("project_id")
	filterMonth := r.URL.Query().Get("filter_month")

	targetExcelUser := filterEmp
	if role != "ADMIN" {
		targetExcelUser = username
	}

	logQuery := `SELECT w.employee_code, w.shamsi_date, p.name, w.hours_spent, w.description 
                 FROM work_logs w JOIN projects p ON w.project_id = p.id WHERE 1=1 `
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
	if err != nil {
		http.Error(w, "خطا در واکشی داده‌های اکسل", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	f := excelize.NewFile()
	sheetName := "گزارش ماتریسی کارکرد"
	f.SetSheetName("Sheet1", sheetName)

	f.SetCellValue(sheetName, "A1", "کد کارمند")
	f.SetCellValue(sheetName, "B1", "تاریخ شمسی")
	f.SetCellValue(sheetName, "C1", "نام پروژه")
	f.SetCellValue(sheetName, "D1", "مدت زمان (ساعت)")
	f.SetCellValue(sheetName, "E1", "شرح فعالیت روزانه")

	rowIdx := 2
	for rows.Next() {
		var empCode, sDate, pName, desc string
		var hours float64
		if err := rows.Scan(&empCode, &sDate, &pName, &hours, &desc); err == nil {
			f.SetCellValue(sheetName, fmt.Sprintf("A%d", rowIdx), empCode)
			f.SetCellValue(sheetName, fmt.Sprintf("B%d", rowIdx), sDate)
			f.SetCellValue(sheetName, fmt.Sprintf("C%d", rowIdx), pName)
			f.SetCellValue(sheetName, fmt.Sprintf("D%d", rowIdx), hours)
			f.SetCellValue(sheetName, fmt.Sprintf("E%d", rowIdx), desc)
			rowIdx++
		}
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=shamsi_matrix_report.xlsx")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")

	if _, err := f.WriteTo(w); err != nil {
		log.Printf("خطا در ارسال فایل: %v", err)
	}
}