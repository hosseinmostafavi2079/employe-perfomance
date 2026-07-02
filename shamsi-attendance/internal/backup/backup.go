package backup

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const BackupDir = "backups"

func InitBackupDir() error {
	if _, err := os.Stat(BackupDir); os.IsNotExist(err) {
		return os.MkdirAll(BackupDir, 0755)
	}
	return nil
}

func RunManualBackup() (string, error) {
	if err := InitBackupDir(); err != nil {
		return "", fmt.Errorf("خطا در ایجاد پوشه بکاپ: %v", err)
	}

	fileName := fmt.Sprintf("shamsi_db_backup_%s.sql", time.Now().Format("20060102_150405"))
	filePath := filepath.Join(BackupDir, fileName)

	// مدیریت فایل خروجی به صورت کاملاً بومی توسط Go
	outfile, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("خطا در ایجاد ساختار فایل: %v", err)
	}

	// آرگومان‌های خالص استخراج دیتابیس
	dockerArgs := []string{
		"exec",
		"-e", "PGPASSWORD=shamsi_secure_pass_2026",
		"shamsi-attendance-db",
		"pg_dump",
		"-U", "attendance_admin",
		"-d", "shamsi_attendance_platform",
		"-F", "p",
		"--clean",
		"--if-exists",
		"--inserts",
	}

	var attempts []struct {
		exe  string
		args []string
	}

	if runtime.GOOS == "windows" {
		attempts = append(attempts, struct{ exe string; args []string }{"docker", dockerArgs})
		
		// 🔑 راهکار قطعی: افزودن "-u root" برای دور زدن خطای permission denied سوکت داکر در WSL
		wslRootArgs := append([]string{"-u", "root", "docker"}, dockerArgs...)
		attempts = append(attempts, struct{ exe string; args []string }{"wsl", wslRootArgs})

		sys32Wsl := `C:\Windows\System32\wsl.exe`
		if _, err := os.Stat(sys32Wsl); err == nil {
			attempts = append(attempts, struct{ exe string; args []string }{sys32Wsl, wslRootArgs})
		}

		dockerPath := `C:\Program Files\Docker\Docker\resources\bin\docker.exe`
		if _, err := os.Stat(dockerPath); err == nil {
			attempts = append(attempts, struct{ exe string; args []string }{dockerPath, dockerArgs})
		}
	} else {
		attempts = append(attempts, struct{ exe string; args []string }{"docker", dockerArgs})
	}

	var lastErr error
	var lastStderr string

	for _, attempt := range attempts {
		exePath := attempt.exe
		if !strings.Contains(exePath, `\`) && !strings.Contains(exePath, `/`) {
			if p, err := exec.LookPath(attempt.exe); err == nil {
				exePath = p
			} else {
				continue 
			}
		}

		cmd := exec.Command(exePath, attempt.args...)
		
		// هدایت داده‌ها از کرنل مستقیماً به فایل هدف
		cmd.Stdout = outfile
		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		err := cmd.Run()

		if err == nil {
			outfile.Close()
			if info, statErr := os.Stat(filePath); statErr == nil && info.Size() > 100 {
				return fileName, nil // ✅ عملیات استخراج بدون هیچ واسطه‌ای موفق بود
			}
		}

		lastErr = err
		lastStderr = strings.TrimSpace(stderr.String())
		outfile.Seek(0, 0)
		outfile.Truncate(0)
	}

	outfile.Close()
	os.Remove(filePath)

	if lastErr == nil {
		return "", fmt.Errorf("هیچ ابزار داکری در سرور شما یافت نشد")
	}

	return "", fmt.Errorf("وضعیت خطا: %v | پاسخ داکر: %s", lastErr, lastStderr)
}

func StartScheduledBackups() {
	go func() {
		for {
			now := time.Now()
			next := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
			if now.After(next) {
				next = next.Add(24 * time.Hour)
			}
			timeToWait := next.Sub(now)
			log.Printf("⏳ ماژول بکاپ خودکار فعال است. بکاپ بعدی در زمان: %s انجام می‌شود.", next.Format("2006-01-02 15:04:05"))
			time.Sleep(timeToWait)

			log.Println("🔄 در حال آغاز عملیات بکاپ‌گیری زمان‌بندی شده...")
			fileName, err := RunManualBackup()
			if err != nil {
				log.Printf("❌ خطا در فرآیند بکاپ‌گیری اتوماتیک: %v\n", err)
			} else {
				log.Printf("✅ سیستم: بکاپ اتوماتیک بامداد با موفقیت در پوشه ذخیره شد: %s\n", fileName)
			}
		}
	}()
}