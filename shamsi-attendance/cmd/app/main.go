package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"shamsi_attendance/internal/attendance"
	"shamsi_attendance/internal/database"
	"shamsi_attendance/internal/project"

	"github.com/xuri/excelize/v2"
	"golang.org/x/crypto/bcrypt"
)

var cookieSecret = []byte("shamsi_matrix_secure_salt_2026_key")

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

type ProjectView struct {
	ID   int
	Name string
}

type EmployeeView struct {
	ID           int
	EmployeeCode string
	FullName     string
	Role         string
}

type PageData struct {
	IsLoggedIn              bool
	CurrentDate             string
	TotalHours              float64
	Message                 string
	CurrentUser             string
	CurrentRole             string
	SelectedFilter          string
	SelectedProjectFilter   string
	SelectedMonthFilter     string
	CurrentTab              string
	WorkLogs                []WorkLogView
	AttendanceLogs        []AttendanceView
	EditLog                 *WorkLogView
	EditAttendanceLog       *AttendanceView
	TotalAttendanceMonthStr string
	TotalAttendanceDayStr   string
	Projects                []ProjectView
	Employees               []EmployeeView
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
	http.HandleFunc("/edit-attendance", handleEditAttendance)
	http.HandleFunc("/edit-worklog", handleEditWorkLog)
	http.HandleFunc("/delete-worklog", handleDeleteWorkLog)
	http.HandleFunc("/delete-attendance", handleDeleteAttendance)
	
	http.HandleFunc("/admin/add-employee", handleAddEmployee)
	http.HandleFunc("/admin/delete-employee", handleDeleteEmployee)
	http.HandleFunc("/admin/create-project", handleCreateProject)
	http.HandleFunc("/admin/delete-project", handleDeleteProject)
	
	http.HandleFunc("/export", handleExportExcel)

	fmt.Println("🚀 وب‌سرور امن تفکیک‌شده با موفقیت روشن شد!")
	fmt.Println("🌐 آدرس ورود به سامانه: http://localhost:8080")
	fmt.Println("==================================================")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func signCookieValue(username string) string {
	mac := hmac.New(sha256.New, cookieSecret)
	mac.Write([]byte(username))
	return username + ":" + hex.EncodeToString(mac.Sum(nil))
}

func verifyCookieValue(cookieVal string) (string, bool) {
	parts := strings.Split(cookieVal, ":")
	if len(parts) != 2 {
		return "", false
	}
	username, clientSig := parts[0], parts[1]
	mac := hmac.New(sha256.New, cookieSecret)
	mac.Write([]byte(username))
	if hmac.Equal([]byte(clientSig), []byte(hex.EncodeToString(mac.Sum(nil)))) {
		return username, true
	}
	return "", false
}

func getAuthenticatedUser(r *http.Request) (string, string) {
	cookie, err := r.Cookie("session_user")
	if err != nil {
		return "", ""
	}
	username, valid := verifyCookieValue(cookie.Value)
	if !valid {
		return "", ""
	}
	var role string
	err = database.DB.QueryRow(r.Context(), "SELECT role FROM employees WHERE employee_code = $1;", username).Scan(&role)
	if err != nil {
		return "", ""
	}
	return username, role
}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
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

	currentShamsiDate := attendance.GetCurrentShamsiDate()
	dateParts := strings.Split(currentShamsiDate, "/")
	currentMonth := ""
	if len(dateParts) == 3 {
		currentMonth = dateParts[1]
	}

	editIDParam := r.URL.Query().Get("edit_id")
	editAttIDParam := r.URL.Query().Get("edit_attendance_id")
	filterEmployee := r.URL.Query().Get("filter_employee")
	filterProject := r.URL.Query().Get("filter_project")
	
	filterMonth := r.URL.Query().Get("filter_month")
	if _, hasMonth := r.URL.Query()["filter_month"]; !hasMonth {
		filterMonth = currentMonth
	}

	tabParam := r.URL.Query().Get("tab")
	currentTab := "attendance"
	if tabParam == "worklog" {
		currentTab = "worklog"
	} else if tabParam == "management" && role == "ADMIN" {
		currentTab = "management"
	} else if tabParam == "project_report" && role == "ADMIN" {
		currentTab = "project_report"
	}

	var projects []ProjectView
	pRows, _ := database.DB.Query(r.Context(), "SELECT id, name FROM projects ORDER BY id ASC;")
	if pRows != nil {
		for pRows.Next() {
			var p ProjectView
			_ = pRows.Scan(&p.ID, &p.Name)
			projects = append(projects, p)
		}
		pRows.Close()
	}

	var employees []EmployeeView
	eRows, _ := database.DB.Query(r.Context(), "SELECT id, employee_code, full_name, role FROM employees ORDER BY id ASC;")
	if eRows != nil {
		for eRows.Next() {
			var emp EmployeeView
			_ = eRows.Scan(&emp.ID, &emp.EmployeeCode, &emp.FullName, &emp.Role)
			employees = append(employees, emp)
		}
		eRows.Close()
	}

	var editLog *WorkLogView = nil
	if editIDParam != "" {
		eID, _ := strconv.Atoi(editIDParam)
		var ev WorkLogView
		err := database.DB.QueryRow(r.Context(), "SELECT id, employee_code, project_id, hours_spent, description, shamsi_date FROM work_logs WHERE id=$1;", eID).
			Scan(&ev.ID, &ev.EmployeeCode, &ev.ProjectID, &ev.HoursSpent, &ev.Description, &ev.ShamsiDate)
		if err == nil {
			editLog = &ev
			currentTab = "worklog"
		}
	}

	var editAttendanceLog *AttendanceView = nil
	if editAttIDParam != "" {
		aID, _ := strconv.Atoi(editAttIDParam)
		var av AttendanceView
		var tIn, tOut *time.Time
		err := database.DB.QueryRow(r.Context(), "SELECT id, employee_code, check_in, check_out, shamsi_date FROM attendance WHERE id=$1;", aID).
			Scan(&av.ID, &av.EmployeeCode, &tIn, &tOut, &av.ShamsiDate)
		if err == nil {
			if tIn != nil {
				av.CheckIn = tIn.In(time.Local).Format("15:04")
			}
			if tOut != nil {
				av.CheckOut = tOut.In(time.Local).Format("15:04")
			}
			editAttendanceLog = &av
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

	rows, err := database.DB.Query(r.Context(), logQuery, args...)
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
	_ = database.DB.QueryRow(r.Context(), sumQuery, sumArgs...).Scan(&totalHours)

	var attLogs []AttendanceView
	var attQuery string
	var attArgs []interface{}
	attArgIdx := 1

	if role == "ADMIN" {
		attQuery = "SELECT id, employee_code, check_in, check_out, shamsi_date FROM attendance WHERE 1=1 "
		if filterEmployee != "" {
			attQuery += fmt.Sprintf("AND employee_code = $%d ", attArgIdx)
			attArgs = append(attArgs, filterEmployee)
			attArgIdx++
		}
	} else {
		attQuery = fmt.Sprintf("SELECT id, employee_code, check_in, check_out, shamsi_date FROM attendance WHERE employee_code = $%d ", attArgIdx)
		attArgs = append(attArgs, username)
		attArgIdx++
	}

	if filterMonth != "" {
		attQuery += fmt.Sprintf("AND split_part(shamsi_date, '/', 2) = $%d ", attArgIdx)
		attArgs = append(attArgs, filterMonth)
		attArgIdx++
	}
	attQuery += "ORDER BY shamsi_date DESC, id DESC;"

	totalMinutesMonth := 0
	totalMinutesDay := 0

	aRows, err := database.DB.Query(r.Context(), attQuery, attArgs...)
	if err == nil {
		defer aRows.Close()
		for aRows.Next() {
			var av AttendanceView
			var tIn, tOut *time.Time
			if err := aRows.Scan(&av.ID, &av.EmployeeCode, &tIn, &tOut, &av.ShamsiDate); err == nil {
				if tIn != nil {
					av.CheckIn = tIn.In(time.Local).Format("15:04") // تغییر به فرمت ساعت:دقیقه بدون ثانیه
				}
				if tOut != nil {
					av.CheckOut = tOut.In(time.Local).Format("15:04") // تغییر به فرمت ساعت:دقیقه بدون ثانیه
					diff := tOut.Sub(*tIn)
					totalMinutesMonth += int(diff.Minutes())
					if av.ShamsiDate == currentShamsiDate {
						totalMinutesDay += int(diff.Minutes())
					}
					// تغییر فیلد نمایش گرید به فرمت درخواستی شما (HH:MM)
					av.Duration = fmt.Sprintf("%d:%02d", int(diff.Hours()), int(diff.Minutes())%60)
				} else {
					av.Duration = "حضور زنده"
					if tIn != nil {
						diff := time.Now().Sub(*tIn)
						totalMinutesMonth += int(diff.Minutes())
						if av.ShamsiDate == currentShamsiDate {
							totalMinutesDay += int(diff.Minutes())
						}
					}
				}
				attLogs = append(attLogs, av)
			}
		}
	}

	data := PageData{
		IsLoggedIn:              true,
		CurrentDate:             currentShamsiDate,
		TotalHours:              totalHours,
		Message:                 systemMessage,
		CurrentUser:             username,
		CurrentRole:             role,
		SelectedFilter:          filterEmployee,
		SelectedProjectFilter:   filterProject,
		SelectedMonthFilter:     filterMonth,
		CurrentTab:              currentTab,
		WorkLogs:                workLogs,
		AttendanceLogs:          attLogs,
		EditLog:                 editLog,
		EditAttendanceLog:       editAttendanceLog,
		// فرمت خروجی مجموع عملکردها به صورت تجمعی صنعتی (مثلاً 144:50)
		TotalAttendanceMonthStr: fmt.Sprintf("%d:%02d", totalMinutesMonth/60, totalMinutesMonth%60),
		TotalAttendanceDayStr:   fmt.Sprintf("%d:%02d", totalMinutesDay/60, totalMinutesDay%60),
		Projects:                projects,
		Employees:               employees,
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
		var dbUser, hashedPass string
		err := database.DB.QueryRow(r.Context(), "SELECT employee_code, password FROM employees WHERE employee_code = $1;", username).Scan(&dbUser, &hashedPass)
		if err == nil && dbUser != "" && bcrypt.CompareHashAndPassword([]byte(hashedPass), []byte(password)) == nil {
			http.SetCookie(w, &http.Cookie{
				Name:     "session_user",
				Value:    signCookieValue(username),
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

		err := project.LogWorkWithDate(username, pID, hours, desc, sDate)
		if err != nil {
			systemMessage = "⚠️ خطا در ثبت کارکرد روزانه: " + err.Error()
		} else {
			systemMessage = "گزارش کارکرد با موفقیت ثبت شد."
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

func handleEditAttendance(w http.ResponseWriter, r *http.Request) {
	username, role := getAuthenticatedUser(r)
	tab := r.FormValue("tab")
	if tab == "" {
		tab = "attendance"
	}
	if username != "" {
		attID, _ := strconv.Atoi(r.FormValue("attendance_id"))
		sDate := r.FormValue("shamsi_date")
		tIn := r.FormValue("check_in_time")
		tOut := r.FormValue("check_out_time")
		target := r.FormValue("target_employee")

		if role != "ADMIN" || target == "" {
			target = username
		}

		err := attendance.UpdateAttendance(attID, target, sDate, tIn, tOut)
		if err != nil {
			systemMessage = "⚠️ خطا در اعمال ویرایش تردد: " + err.Error()
		} else {
			systemMessage = "رکورد تردد انتخاب‌شده با موفقیت بازنویسی و اصلاح شد."
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

		err := project.UpdateWorkLog(logID, pID, hours, desc, sDate)
		if err != nil {
			systemMessage = "⚠️ خطا در ویرایش رکورد: " + err.Error()
		} else {
			systemMessage = "رکورد کارکرد روزانه شما با موفقیت به روزرسانی شد."
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
		logID, _ := strconv.Atoi(r.URL.Query().Get("id"))
		err := project.DeleteWorkLog(logID)
		if err != nil {
			systemMessage = "⚠️ خطا در حذف کارکرد: " + err.Error()
		} else {
			systemMessage = "رکورد کارکرد حذف شد."
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
		attID, _ := strconv.Atoi(r.URL.Query().Get("id"))
		_, err := database.DB.Exec(r.Context(), "DELETE FROM attendance WHERE id = $1;", attID)
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
	tab := r.FormValue("tab")
	if tab == "" {
		tab = "management"
	}
	if role == "ADMIN" && r.Method == http.MethodPost {
		empCode := r.FormValue("emp_code")
		password := r.FormValue("password")
		fullName := r.FormValue("full_name")
		uRole := r.FormValue("role")

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			systemMessage = "خطا در پردازش رمز عبور"
			http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
			return
		}

		query := "INSERT INTO employees (employee_code, full_name, password, role) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING;"
		_, err = database.DB.Exec(r.Context(), query, empCode, fullName, string(hashedPassword), uRole)
		if err != nil {
			systemMessage = "خطا در ثبت پرسنل جدید: " + err.Error()
		} else {
			systemMessage = fmt.Sprintf("موفقیت: نیرو با نام «%s» استخدام شد.", fullName)
		}
	}
	http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
}

func handleDeleteEmployee(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "management"
	}
	if role == "ADMIN" {
		empID, _ := strconv.Atoi(r.URL.Query().Get("id"))

		var empCode string
		_ = database.DB.QueryRow(r.Context(), "SELECT employee_code FROM employees WHERE id = $1;", empID).Scan(&empCode)
		if empCode == "ADMIN" {
			systemMessage = "⚠️ خطای امنیتی: شما مجاز به حذف حساب کاربری ارشد ارگان (ADMIN) نیستید!"
		} else {
			_, err := database.DB.Exec(r.Context(), "DELETE FROM employees WHERE id = $1;", empID)
			if err != nil {
				systemMessage = "⚠️ خطا در حذف کارمند: " + err.Error()
			} else {
				systemMessage = "نیروی انتخابی با موفقیت از سیستم حذف شد."
			}
		}
	}
	http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
}

func handleCreateProject(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	tab := r.FormValue("tab")
	if tab == "" {
		tab = "management"
	}
	if role == "ADMIN" && r.Method == http.MethodPost {
		name := r.FormValue("project_name")
		if name != "" {
			_, err := project.CreateProject(name)
			if err != nil {
				systemMessage = "⚠️ خطا در افزودن پروژه: " + err.Error()
			} else {
				systemMessage = fmt.Sprintf("پروژه جدید با نام «%s» در پایگاه داده ایجاد شد.", name)
			}
		}
	}
	http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
}

func handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	_, role := getAuthenticatedUser(r)
	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "management"
	}
	if role == "ADMIN" {
		pID, _ := strconv.Atoi(r.URL.Query().Get("id"))
		err := project.DeleteProject(pID)
		if err != nil {
			systemMessage = "⚠️ خطا در حذف پروژه: " + err.Error()
		} else {
			systemMessage = "پروژه و تمامی تاریخچه گزارش کارکردهای متصل به آن کاملاً پاکسازی شدند."
		}
	}
	http.Redirect(w, r, "/?tab="+tab, http.StatusSeeOther)
}

func handleExportExcel(w http.ResponseWriter, r *http.Request) {
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

	f := excelize.NewFile()
	sheetWork := "گزارش کارکرد پروژه‌ها"
	f.SetSheetName("Sheet1", sheetWork)
	f.SetCellValue(sheetWork, "A1", "کد کارمند")
	f.SetCellValue(sheetWork, "B1", "تاریخ شمسی")
	f.SetCellValue(sheetWork, "C1", "نام پروژه")
	f.SetCellValue(sheetWork, "D1", "مدت زمان (ساعت)")
	f.SetCellValue(sheetWork, "E1", "شرح فعالیت روزانه")

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

	rows, err := database.DB.Query(r.Context(), logQuery, args...)
	if err == nil {
		rowIdx := 2
		for rows.Next() {
			var empCode, sDate, pName, desc string
			var hours float64
			if err := rows.Scan(&empCode, &sDate, &pName, &hours, &desc); err == nil {
				f.SetCellValue(sheetWork, fmt.Sprintf("A%d", rowIdx), empCode)
				f.SetCellValue(sheetWork, fmt.Sprintf("B%d", rowIdx), sDate)
				f.SetCellValue(sheetWork, fmt.Sprintf("C%d", rowIdx), pName)
				f.SetCellValue(sheetWork, fmt.Sprintf("D%d", rowIdx), hours)
				f.SetCellValue(sheetWork, fmt.Sprintf("E%d", rowIdx), desc)
				rowIdx++
			}
		}
		rows.Close()
	}

	sheetAtt := "گزارش حضور و غیاب"
	f.NewSheet(sheetAtt)
	f.SetCellValue(sheetAtt, "A1", "کد نیرو")
	f.SetCellValue(sheetAtt, "B1", "تاریخ شمسی")
	f.SetCellValue(sheetAtt, "C1", "ساعت ورود")
	f.SetCellValue(sheetAtt, "D1", "ساعت خروج")
	f.SetCellValue(sheetAtt, "E1", "مدت زمان حضور")

	var attQuery string
	var attArgs []interface{}
	attArgIdx := 1
	if role == "ADMIN" && targetExcelUser == "" {
		attQuery = "SELECT id, employee_code, check_in, check_out, shamsi_date FROM attendance WHERE 1=1 "
	} else {
		attQuery = "SELECT id, employee_code, check_in, check_out, shamsi_date FROM attendance WHERE employee_code = $1 "
		attArgs = append(attArgs, targetExcelUser)
		attArgIdx++
	}
	if filterMonth != "" {
		attQuery += fmt.Sprintf("AND split_part(shamsi_date, '/', 2) = $%d ", attArgIdx)
		attArgs = append(attArgs, filterMonth)
	}
	attQuery += "ORDER BY shamsi_date DESC, id DESC;"

	currentShamsiDate := attendance.GetCurrentShamsiDate()
	totalMinutesMonth := 0
	totalMinutesDay := 0

	aRows, err := database.DB.Query(r.Context(), attQuery, attArgs...)
	if err == nil {
		rowIdx := 2
		for aRows.Next() {
			var id int
			var empCode, sDate string
			var tIn, tOut *time.Time
			if err := aRows.Scan(&id, &empCode, &tIn, &tOut, &sDate); err == nil {
				strIn, strOut, strDuration := "--", "--", "حضور زنده"
				if tIn != nil {
					strIn = tIn.In(time.Local).Format("15:04") // ویرایش خروجی اکسل بدون ثانیه
				}
				if tOut != nil {
					strOut = tOut.In(time.Local).Format("15:04") // ویرایش خروجی اکسل بدون ثانیه
					diff := tOut.Sub(*tIn)
					totalMinutesMonth += int(diff.Minutes())
					if sDate == currentShamsiDate {
						totalMinutesDay += int(diff.Minutes())
					}
					strDuration = fmt.Sprintf("%d:%02d", int(diff.Hours()), int(diff.Minutes())%60)
				} else if tIn != nil {
					diff := time.Now().Sub(*tIn)
					totalMinutesMonth += int(diff.Minutes())
					if sDate == currentShamsiDate {
						totalMinutesDay += int(diff.Minutes())
					}
				}

				f.SetCellValue(sheetAtt, fmt.Sprintf("A%d", rowIdx), empCode)
				f.SetCellValue(sheetAtt, fmt.Sprintf("B%d", rowIdx), sDate)
				f.SetCellValue(sheetAtt, fmt.Sprintf("C%d", rowIdx), strIn)
				f.SetCellValue(sheetAtt, fmt.Sprintf("D%d", rowIdx), strOut)
				f.SetCellValue(sheetAtt, fmt.Sprintf("E%d", rowIdx), strDuration)
				rowIdx++
			}
		}
		aRows.Close()

		// فرمت خروجی نهایی تجمعی در انتهای فایل اکسل (مثلاً 144:50)
		rowIdx += 2
		f.SetCellValue(sheetAtt, fmt.Sprintf("A%d", rowIdx), "جمع کل حضور فیلتر ماه:")
		f.SetCellValue(sheetAtt, fmt.Sprintf("B%d", rowIdx), fmt.Sprintf("%d:%02d", totalMinutesMonth/60, totalMinutesMonth%60))
		rowIdx++
		f.SetCellValue(sheetAtt, fmt.Sprintf("A%d", rowIdx), "جمع کل حضور امروز:")
		f.SetCellValue(sheetAtt, fmt.Sprintf("B%d", rowIdx), fmt.Sprintf("%d:%02d", totalMinutesDay/60, totalMinutesDay%60))
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=shamsi_matrix_report.xlsx")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	if _, err := f.WriteTo(w); err != nil {
		log.Printf("خطا در ارسال فایل: %v", err)
	}
}