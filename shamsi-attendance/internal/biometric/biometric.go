package biometric

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"shamsi_attendance/internal/database"

	"github.com/go-webauthn/webauthn/webauthn"
)

var (
	WebAuthnInstance *webauthn.WebAuthn
	sessionStore     = make(map[string]webauthn.SessionData)
	sessionMu        sync.Mutex
)

// InitWebAuthn هسته امنیتی را منحصراً برای دامنه پروداکشن شما قفل و تنظیم می‌کند
func InitWebAuthn() error {
	w, err := webauthn.New(&webauthn.Config{
		RPDisplayName: "سامانه کارکرد شمسی",
		RPID:          "p.mediasanat.ir",                  // 👈 قفل شدن سنسور روی دامنه شما
		RPOrigins:     []string{"https://p.mediasanat.ir"}, // 👈 الزام به استفاده از HTTPS
	})
	if err != nil {
		return err
	}
	WebAuthnInstance = w
	return nil
}

// WebUser ساختار رابط استاندارد برای کاربر
type WebUser struct {
	EmployeeCode string
	FullName     string
	Credentials  []webauthn.Credential
}

func (u *WebUser) WebAuthnID() []byte { return []byte(u.EmployeeCode) }
func (u *WebUser) WebAuthnName() string { return u.EmployeeCode }
func (u *WebUser) WebAuthnDisplayName() string { return u.FullName }
func (u *WebUser) WebAuthnIcon() string { return "" }
func (u *WebUser) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }

func getWebUser(employeeCode string) (*WebUser, error) {
	ctx := context.Background()
	var fullName string
	err := database.DB.QueryRow(ctx, "SELECT full_name FROM employees WHERE employee_code=$1", employeeCode).Scan(&fullName)
	if err != nil {
		return nil, fmt.Errorf("کاربر یافت نشد")
	}

	user := &WebUser{EmployeeCode: employeeCode, FullName: fullName, Credentials: []webauthn.Credential{}}
	rows, _ := database.DB.Query(ctx, "SELECT credential_id, public_key, attestation_type, sign_count, aaguid FROM user_biometrics WHERE employee_code=$1", employeeCode)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var c webauthn.Credential
			rows.Scan(&c.ID, &c.PublicKey, &c.AttestationType, &c.Authenticator.SignCount, &c.Authenticator.AAGUID)
			user.Credentials = append(user.Credentials, c)
		}
	}
	return user, nil
}

// HandleRegisterBegin درخواست ثبت اثر انگشت جدید
func HandleRegisterBegin(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_user")
	if err != nil {
		http.Error(w, "عدم دسترسی", http.StatusUnauthorized)
		return
	}
	empCode, _ := url.QueryUnescape(cookie.Value)

	user, err := getWebUser(empCode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	options, sessionData, err := WebAuthnInstance.BeginRegistration(user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sessionMu.Lock()
	sessionStore[empCode+"_reg"] = *sessionData
	sessionMu.Unlock()

	json.NewEncoder(w).Encode(options)
}

// HandleRegisterFinish ذخیره اثر انگشت در دیتابیس
func HandleRegisterFinish(w http.ResponseWriter, r *http.Request) {
	cookie, _ := r.Cookie("session_user")
	empCode, _ := url.QueryUnescape(cookie.Value)
	user, _ := getWebUser(empCode)

	sessionMu.Lock()
	sessionData, ok := sessionStore[empCode+"_reg"]
	delete(sessionStore, empCode+"_reg")
	sessionMu.Unlock()

	if !ok {
		http.Error(w, "منقضی شدن نشست", http.StatusBadRequest)
		return
	}

	credential, err := WebAuthnInstance.FinishRegistration(user, sessionData, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	database.DB.Exec(context.Background(), `
		INSERT INTO user_biometrics (employee_code, credential_id, public_key, attestation_type, sign_count, aaguid)
		VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (credential_id) DO UPDATE SET sign_count = EXCLUDED.sign_count
	`, empCode, credential.ID, credential.PublicKey, credential.AttestationType, credential.Authenticator.SignCount, credential.Authenticator.AAGUID)

	w.WriteHeader(http.StatusOK)
}

// HandleLoginBegin درخواست چالش برای ورود
func HandleLoginBegin(w http.ResponseWriter, r *http.Request) {
	empCode := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("username")))
	user, err := getWebUser(empCode)
	if err != nil || len(user.Credentials) == 0 {
		http.Error(w, "سنسوری ثبت نشده است", http.StatusNotFound)
		return
	}

	options, sessionData, err := WebAuthnInstance.BeginLogin(user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sessionMu.Lock()
	sessionStore[empCode+"_login"] = *sessionData
	sessionMu.Unlock()

	json.NewEncoder(w).Encode(options)
}

// HandleLoginFinish تایید اثر انگشت و صدور کوکی ورود
func HandleLoginFinish(w http.ResponseWriter, r *http.Request) {
	empCode := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("username")))
	user, _ := getWebUser(empCode)

	sessionMu.Lock()
	sessionData, ok := sessionStore[empCode+"_login"]
	delete(sessionStore, empCode+"_login")
	sessionMu.Unlock()

	if !ok {
		http.Error(w, "منقضی شدن نشست", http.StatusBadRequest)
		return
	}

	credential, err := WebAuthnInstance.FinishLogin(user, sessionData, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	database.DB.Exec(context.Background(), "UPDATE user_biometrics SET sign_count=$1 WHERE credential_id=$2", credential.Authenticator.SignCount, credential.ID)

	http.SetCookie(w, &http.Cookie{
		Name:     "session_user",
		Value:    url.QueryEscape(empCode),
		Expires:  time.Now().Add(24 * time.Hour),
		HttpOnly: true,
		Path:     "/",
	})
	
	w.WriteHeader(http.StatusOK)
}