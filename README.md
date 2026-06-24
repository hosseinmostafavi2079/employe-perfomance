<div dir="rtl">
<div align="center">
<img src="https://img.shields.io/badge/Go-1.21+-00ADD8?style=for-the-badge&logo=go&logoColor=white" />
<img src="https://img.shields.io/badge/SQLite-3-003B57?style=for-the-badge&logo=sqlite&logoColor=white" />
<img src="https://img.shields.io/badge/Platform-Windows-0078D6?style=for-the-badge&logo=windows&logoColor=white" />
<img src="https://img.shields.io/badge/License-MIT-green?style=for-the-badge" />
# 🕐 سیستم مدیریت حضور و غیاب پرسنل
### **Employee Performance & Attendance Management System**
 
> یک اپلیکیشن وب سبک و پرسرعت برای ثبت، مدیریت و گزارش‌گیری حضور و غیاب پرسنل  
> با پشتیبانی کامل از **تقویم شمسی (جلالی)** و رابط کاربری **RTL / تاریک**
 
---
 
**توسعه‌دهنده:** [Hossein Mostafavi](https://github.com/hosseinmostafavi2079)  
**زبان برنامه‌نویسی:** Go (Golang)  
**پایگاه داده:** SQLite  
**معماری:** MVC — بدون فریم‌ورک خارجی
 
</div>
---
 
## 📋 فهرست مطالب
 
- [ویژگی‌ها](#-ویژگیها)
- [پیش‌نیازها](#-پیشنیازها)
- [نصب و راه‌اندازی](#-نصب-و-راهاندازی)
- [ساخت فایل اجرایی (EXE)](#-ساخت-فایل-اجرایی-exe)
- [ساختار پروژه](#-ساختار-پروژه)
- [نحوه استفاده](#-نحوه-استفاده)
- [پیکربندی](#-پیکربندی)
- [عکس‌های محیط نرم‌افزار](#-عکسهای-محیط-نرمافزار)
- [مشارکت در توسعه](#-مشارکت-در-توسعه)
- [مجوز](#-مجوز)
---
 
## ✨ ویژگی‌ها
 
| ویژگی | توضیح |
|---|---|
| 📅 تقویم شمسی | پشتیبانی کامل از تاریخ و زمان جلالی |
| 👥 مدیریت پرسنل | افزودن، ویرایش و حذف کارمندان |
| 🔐 نقش‌های کاربری | پنل مجزا برای مدیر و پرسنل |
| ⏱ ثبت تردد | ثبت ورود/خروج با فرمت `HH:MM` |
| 📊 گزارش‌گیری | گزارش ماهانه و روزانه حضور |
| 🌙 رابط تاریک | طراحی Dark UI با جهت RTL فارسی |
| ⚡ سبک و سریع | بدون نیاز به Node.js، Docker یا PHP |
| 💾 بدون نیاز به سرور | اجرا مستقیم با یک فایل `.exe` |
 
---
 
## 🔧 پیش‌نیازها
 
### برای توسعه (Development)
 
| ابزار | نسخه پیشنهادی | لینک دانلود |
|---|---|---|
| **Go** | 1.21 یا بالاتر | [golang.org/dl](https://golang.org/dl/) |
| **Git** | هر نسخه‌ای | [git-scm.com](https://git-scm.com/) |
| **SQLite** (اختیاری) | 3.x | [sqlite.org](https://www.sqlite.org/download.html) |
 
### برای کاربر نهایی (Production)
 
> هیچ‌چیز نصب نیست! فقط فایل `.exe` را اجرا کنید.
 
---
 
## 🚀 نصب و راه‌اندازی
 
### ۱. دریافت کد منبع
 
```bash
git clone https://github.com/hosseinmostafavi2079/employe-perfomance.git
cd employe-perfomance
```
 
### ۲. نصب وابستگی‌ها (Dependencies)
 
```bash
go mod tidy
```
 
این دستور تمام کتابخانه‌های مورد نیاز را از فایل `go.mod` دانلود و نصب می‌کند.
 
### ۳. اجرای برنامه (حالت توسعه)
 
```bash
go run main.go
```
 
سپس مرورگر را باز کنید و به آدرس زیر بروید:
 
```
http://localhost:8080
```
 
---
 
## 🏗 ساخت فایل اجرایی (EXE)
 
### ساخت برای ویندوز (روی هر سیستم‌عاملی)
 
```bash
GOOS=windows GOARCH=amd64 go build -o attendance.exe main.go
```
 
### ساخت برای ویندوز (داخل ویندوز)
 
```cmd
set GOOS=windows
set GOARCH=amd64
go build -o attendance.exe main.go
```
 
### ساخت با حذف پنجره CMD (بدون کنسول سیاه)
 
```bash
go build -ldflags="-H windowsgui" -o attendance.exe main.go
```
 
### ساخت فشرده‌سازی‌شده (حجم کمتر)
 
```bash
go build -ldflags="-s -w" -o attendance.exe main.go
```
 
> **نکته:** فایل `attendance.exe` کاملاً مستقل است و نیازی به نصب Go یا هیچ کتابخانه‌ای روی سیستم مقصد ندارد.
 
---
 
## 📦 کتابخانه‌های استفاده‌شده
 
| کتابخانه | کاربرد | نصب |
|---|---|---|
| `mattn/go-sqlite3` | اتصال به SQLite | خودکار با `go mod tidy` |
| `jalaali/go-jalaali` | تبدیل تاریخ جلالی | خودکار با `go mod tidy` |
| `net/http` | وب‌سرور داخلی | استاندارد Go |
| `html/template` | رندر صفحات HTML | استاندارد Go |
 
### نصب دستی (در صورت نیاز)
 
```bash
go get github.com/mattn/go-sqlite3
go get github.com/jalaali/go-jalaali
```
 
> **توجه:** کتابخانه `go-sqlite3` از CGO استفاده می‌کند. روی ویندوز باید **GCC** نصب باشد:
> - دانلود [TDM-GCC](https://jmeubank.github.io/tdm-gcc/) یا [MinGW-w64](https://www.mingw-w64.org/)
 
---
 
## 🗄 پایگاه داده
 
این پروژه از **SQLite** استفاده می‌کند. فایل پایگاه داده به‌صورت خودکار در مسیر زیر ساخته می‌شود:
 
```
attendance.db
```
 
### ساختار جداول اصلی
 
```sql
-- جدول پرسنل
CREATE TABLE employees (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    name      TEXT NOT NULL,
    national_id TEXT UNIQUE,
    role      TEXT DEFAULT 'employee',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
 
-- جدول تردد
CREATE TABLE attendance (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    employee_id INTEGER,
    work_date   TEXT,
    start_time  TEXT,   -- فرمت HH:MM
    end_time    TEXT,   -- فرمت HH:MM
    duration    TEXT,   -- مدت کارکرد HH:MM
    FOREIGN KEY (employee_id) REFERENCES employees(id)
);
```
 
> پایگاه داده در اولین اجرا به‌صورت **خودکار** ایجاد می‌شود.
 
---
 
## 🗂 ساختار پروژه
 
```
employe-perfomance/
│
├── main.go              # نقطه ورود برنامه
├── go.mod               # تعریف ماژول و وابستگی‌ها
├── go.sum               # چک‌سام وابستگی‌ها
├── attendance.db        # پایگاه داده SQLite (ساخته‌شده در اجرا)
│
├── handlers/            # هندلرهای HTTP
│   ├── employee.go
│   ├── attendance.go
│   └── report.go
│
├── models/              # ساختارهای داده و توابع DB
│   ├── employee.go
│   └── attendance.go
│
├── templates/           # قالب‌های HTML
│   ├── layout.html
│   ├── index.html
│   ├── employees.html
│   ├── add_attendance.html
│   └── report.html
│
├── static/              # فایل‌های استاتیک
│   ├── css/
│   └── js/
│
└── helpers/             # توابع کمکی (تقویم، فرمت زمان)
    └── jalali.go
```
 
---
 
## 🖥 نحوه استفاده
 
### ۱. پنل مدیر
 
- مشاهده لیست کامل پرسنل
- ثبت و ویرایش ورود/خروج کارمندان
- مشاهده گزارش ماهانه هر کارمند
- افزودن / حذف پرسنل
### ۲. ثبت تردد
 
- تاریخ به‌صورت **شمسی** وارد می‌شود
- ساعت ورود و خروج با فرمت **`HH:MM`** (مثلاً `08:30` و `17:00`)
- مدت کارکرد به‌صورت خودکار محاسبه می‌شود
### ۳. گزارش‌گیری
 
```
http://localhost:8080/report?month=1403/04
```
 
---
 
## ⚙ پیکربندی
 
در فایل `main.go` می‌توانید تنظیمات زیر را تغییر دهید:
 
```go
const (
    PORT    = ":8080"           // پورت سرور
    DB_PATH = "./attendance.db" // مسیر فایل دیتابیس
)
```
 
---
 
## 🖼 عکس‌های محیط نرم‌افزار
 
> *(می‌توانید عکس‌های صفحه برنامه را در پوشه `screenshots/` قرار داده و اینجا درج کنید)*
 
```
screenshots/
├── dashboard.png
├── attendance-form.png
└── report.png
```
 
---
 
## 🔐 امنیت
 
- تمام داده‌ها به‌صورت **محلی** روی سیستم ذخیره می‌شوند
- هیچ داده‌ای به اینترنت ارسال نمی‌شود
- پایگاه داده SQLite فایل‌محور و قابل بکاپ است
---
 
## 🤝 مشارکت در توسعه
 
Pull Request‌ها خوشامد هستند! لطفاً برای تغییرات بزرگ ابتدا یک Issue باز کنید.
 
```bash
# Fork کنید، سپس:
git checkout -b feature/my-new-feature
git commit -m "feat: add my new feature"
git push origin feature/my-new-feature
```
 
---
 
## 📞 ارتباط با توسعه‌دهنده
 
| روش | آدرس |
|---|---|
| GitHub | [@hosseinmostafavi2079](https://github.com/hosseinmostafavi2079) |
| وب‌سایت | [mostech.ir](https://mostech.ir) |
 
---
 
## 📄 مجوز
 
این پروژه تحت مجوز **MIT** منتشر شده است.  
برای اطلاعات بیشتر فایل [LICENSE](./LICENSE) را مطالعه کنید.
 
---
 
<div align="center">
ساخته‌شده با ❤️ توسط **حسین مصطفوی**  
*اگر این پروژه برایتان مفید بود، یک ⭐ بدهید!*
 
</div>
</div>
