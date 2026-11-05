package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const credentialAAD = "rabin-assistant/rabin-credentials/v1"

type encryptedRabinCredentials struct {
	Version    int       `json:"version"`
	Nonce      string    `json:"nonce"`
	Ciphertext string    `json:"ciphertext"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type RabinCredentialStore struct {
	mu          sync.Mutex
	path        string
	sessionPath string
	keyPath     string
}

type RabinSettingsPageData struct {
	Username   string
	Saved      bool
	HasSession bool
	LoginAt    string
	Error      string
	Notice     string
}

var rabinSettingsPage = template.Must(template.New("rabin-settings").Parse(`<!doctype html>
<html lang="fa" dir="rtl">
<head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>اتصال به رابین · دستیار رابین</title>
<style>
*{box-sizing:border-box}body{margin:0;background:#f4f6fa;color:#182230;font:14px Tahoma,Arial}
.wrap{max-width:760px;margin:30px auto;padding:0 16px}.head{display:flex;justify-content:space-between;align-items:center;gap:12px;flex-wrap:wrap;margin-bottom:18px}
h1{font-size:22px;margin:0}h2{font-size:16px;margin:0 0 14px}.muted,.foot{color:#667085}
.nav{display:flex;gap:7px;flex-wrap:wrap}.nav a{display:inline-block;padding:9px 12px;border:1px solid #d0d5dd;border-radius:9px;color:#344054;text-decoration:none;background:#fff}
.nav a.active{background:#175cd3;color:#fff;border-color:#175cd3}
.card{background:#fff;border:1px solid #e4e7ec;border-radius:13px;padding:18px}
label{display:block;margin:14px 0 6px;font-weight:bold}input{width:100%;padding:11px;border:1px solid #d0d5dd;border-radius:8px;font:inherit}
button{border:0;border-radius:8px;background:#175cd3;color:#fff;padding:10px 14px;font:inherit;cursor:pointer}
.row{display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin-top:16px}.danger{background:#fff;color:#b42318;border:1px solid #fecdca}
.msg{padding:12px;border-radius:9px;margin:12px 0;line-height:1.8;background:#ecfdf3;color:#05603a}
.err{background:#fef3f2;color:#b42318}.note{padding:12px;border-radius:9px;margin:12px 0;line-height:1.9;background:#fffaeb;border:1px solid #fedf89;color:#93370d}
.tag{display:inline-block;padding:6px 10px;border-radius:16px;background:#ecfdf3;color:#067647;font-size:12px}
.foot{font-size:12px;line-height:1.9;margin-top:14px}
</style></head>
<body><main class="wrap">
<div class="head"><div><h1>اتصال به سامانه رابین</h1><div class="muted" style="margin-top:7px">مدیریت اطلاعات ورود برای آزمایش اتصال</div></div>
<nav class="nav"><a href="/">سفارش‌های فعال</a><a href="/history">تاریخچه</a><a class="active" href="/settings">اتصال به رابین</a></nav></div>
{{if .Error}}<div class="msg err">{{.Error}}</div>{{end}}{{if .Notice}}<div class="msg">{{.Notice}}</div>{{end}}
<section class="card">
<h2>اطلاعات حساب رابین</h2>
{{if .Saved}}<p><span class="tag">اطلاعات ورود ذخیره شده</span></p>{{else}}<p class="muted">هنوز اطلاعات ورود رابین ثبت نشده است.</p>{{end}}
<form method="post" action="/settings/rabin-credentials" autocomplete="off">
<label for="rabin-username">نام کاربری</label>
<input id="rabin-username" name="username" type="text" maxlength="200" value="{{.Username}}" autocomplete="username" required>
<label for="rabin-password">رمز عبور</label>
<input id="rabin-password" name="password" type="password" maxlength="1024" autocomplete="new-password" placeholder="{{if .Saved}}برای حفظ رمز قبلی خالی بگذارید؛ برای تغییر، رمز جدید را وارد کنید{{else}}رمز عبور حساب رابین{{end}}" {{if not .Saved}}required{{end}}>
<div class="row"><button type="submit">ذخیره اطلاعات ورود</button></div>
</form>
{{if .Saved}}
<form method="post" action="/settings/rabin-credentials/delete" onsubmit="return confirm('اطلاعات ورود رابین حذف شود؟')">
<div class="row"><button class="danger" type="submit">حذف اطلاعات ذخیره‌شده</button></div>
</form>
{{end}}
{{if .HasSession}}<div class="msg">آخرین ورود عادی رابین با موفقیت انجام شد: {{.LoginAt}}. توکن نشست نیز به‌صورت رمزنگاری‌شده روی سرور ذخیره شده است.</div>{{else}}<div class="note">اطلاعات ورود در سرور به‌صورت رمزنگاری‌شده نگهداری می‌شوند. هنوز ورود موفقی برای این حساب ثبت نشده است.</div>{{end}}
<form method="post" action="/settings/rabin-login">
<div class="row"><button type="submit" {{if not .Saved}}disabled title="ابتدا اطلاعات ورود را ذخیره کنید"{{end}}>تست ورود با نام کاربری و رمز ذخیره‌شده</button></div>
</form>
<div class="foot">این دکمه فقط ورود عادی و دریافت نشست را آزمایش می‌کند؛ هیچ سفارش خرید یا فروشی ارسال نمی‌شود. در این نسخه فیلد یا مرحله‌ای برای رمز دومرحله‌ای وجود ندارد.</div>
<div class="foot">رمز عبور در صفحه دوباره نمایش داده نمی‌شود. پس از ثبت، برای حفظ رمز قبلی کادر رمز را خالی بگذارید. برای جایگزینی رمز، رمز جدید را وارد کنید.</div>
</section></main></body></html>`))

func NewRabinCredentialStore(dataDir, keyPath string) *RabinCredentialStore {
	return &RabinCredentialStore{
		path:        filepath.Join(dataDir, "rabin-credentials.enc.json"),
		sessionPath: filepath.Join(dataDir, "rabin-session.enc.json"),
		keyPath:     keyPath,
	}
}

func (s *RabinCredentialStore) readKey() ([]byte, error) {
	info, err := os.Stat(s.keyPath)
	if err != nil {
		return nil, fmt.Errorf("کلید رمزنگاری پیدا نشد: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 {
		return nil, errors.New("مجوز فایل کلید رمزنگاری ناامن است")
	}
	key, err := os.ReadFile(s.keyPath)
	if err != nil {
		return nil, fmt.Errorf("خواندن کلید رمزنگاری: %w", err)
	}
	if len(key) != 32 {
		return nil, errors.New("طول کلید رمزنگاری باید دقیقاً ۳۲ بایت باشد")
	}
	return key, nil
}

func (s *RabinCredentialStore) loadLocked() (string, string, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("خواندن اطلاعات ورود: %w", err)
	}
	var record encryptedRabinCredentials
	if err := json.Unmarshal(data, &record); err != nil {
		return "", "", errors.New("ساختار فایل اطلاعات ورود معتبر نیست")
	}
	if record.Version != 1 {
		return "", "", errors.New("نسخه فایل اطلاعات ورود پشتیبانی نمی‌شود")
	}
	nonce, err := base64.StdEncoding.DecodeString(record.Nonce)
	if err != nil {
		return "", "", errors.New("مقدار nonce در فایل اطلاعات ورود نامعتبر است")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(record.Ciphertext)
	if err != nil {
		return "", "", errors.New("داده رمزنگاری‌شده اطلاعات ورود نامعتبر است")
	}
	key, err := s.readKey()
	if err != nil {
		return "", "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}
	if len(nonce) != gcm.NonceSize() {
		return "", "", errors.New("اندازه nonce اطلاعات ورود نامعتبر است")
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, []byte(credentialAAD))
	if err != nil {
		return "", "", errors.New("رمزگشایی اطلاعات ورود ناموفق بود؛ کلید یا فایل ممکن است تغییر کرده باشد")
	}
	parts := strings.SplitN(string(plain), "\x00", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("محتوای اطلاعات ورود ناقص است")
	}
	return parts[0], parts[1], nil
}

func (s *RabinCredentialStore) Save(username, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	username = strings.TrimSpace(username)
	if username == "" || len(username) > 200 || strings.ContainsRune(username, '\x00') {
		return errors.New("نام کاربری رابین معتبر نیست")
	}
	if password == "" {
		previousUser, previousPassword, err := s.loadLocked()
		if err != nil {
			return err
		}
		if previousPassword == "" {
			return errors.New("برای ذخیره اولیه، رمز عبور رابین را وارد کنید")
		}
		if username == "" {
			username = previousUser
		}
		password = previousPassword
	}
	if len(password) > 1024 || strings.ContainsRune(password, '\x00') {
		return errors.New("رمز عبور واردشده معتبر نیست")
	}

	key, err := s.readKey()
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("تولید nonce رمزنگاری: %w", err)
	}
	plain := []byte(username + "\x00" + password)
	ciphertext := gcm.Seal(nil, nonce, plain, []byte(credentialAAD))
	record := encryptedRabinCredentials{
		Version: 1,
		Nonce: base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
		UpdatedAt: time.Now().UTC(),
	}
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(encoded)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(tmp)
		return writeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if dir, err := os.Open(filepath.Dir(s.path)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func (s *RabinCredentialStore) Status() (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	username, password, err := s.loadLocked()
	if err != nil {
		return "", false, err
	}
	return username, password != "", nil
}

func (s *RabinCredentialStore) Load() (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *RabinCredentialStore) SaveAccessToken(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if token == "" || len(token) > 64<<10 || strings.ContainsRune(token, '\x00') {
		return errors.New("توکن دریافتی رابین معتبر نیست")
	}
	key, err := s.readKey()
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("تولید nonce نشست: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(token), []byte("rabin-assistant/rabin-session/v1"))
	record := encryptedRabinCredentials{
		Version: 1,
		Nonce: base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
		UpdatedAt: time.Now().UTC(),
	}
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.sessionPath), 0700); err != nil {
		return err
	}
	tmp := s.sessionPath + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(encoded)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(tmp)
		return writeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Rename(tmp, s.sessionPath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if dir, err := os.Open(filepath.Dir(s.sessionPath)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func (s *RabinCredentialStore) LoadAccessToken() (string, time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.sessionPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", time.Time{}, nil
	}
	if err != nil {
		return "", time.Time{}, fmt.Errorf("خواندن نشست رابین: %w", err)
	}
	var record encryptedRabinCredentials
	if err := json.Unmarshal(data, &record); err != nil {
		return "", time.Time{}, errors.New("ساختار فایل نشست رابین معتبر نیست")
	}
	if record.Version != 1 {
		return "", time.Time{}, errors.New("نسخه فایل نشست رابین پشتیبانی نمی‌شود")
	}
	nonce, err := base64.StdEncoding.DecodeString(record.Nonce)
	if err != nil {
		return "", time.Time{}, errors.New("nonce نشست رابین نامعتبر است")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(record.Ciphertext)
	if err != nil {
		return "", time.Time{}, errors.New("داده نشست رابین نامعتبر است")
	}
	key, err := s.readKey()
	if err != nil {
		return "", time.Time{}, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", time.Time{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", time.Time{}, err
	}
	if len(nonce) != gcm.NonceSize() {
		return "", time.Time{}, errors.New("اندازه nonce نشست رابین نامعتبر است")
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, []byte("rabin-assistant/rabin-session/v1"))
	if err != nil {
		return "", time.Time{}, errors.New("رمزگشایی نشست رابین ناموفق بود")
	}
	if len(plain) == 0 {
		return "", time.Time{}, errors.New("توکن نشست رابین خالی است")
	}
	return string(plain), record.UpdatedAt, nil
}

func (s *RabinCredentialStore) SessionStatus() (time.Time, bool, error) {
	token, updatedAt, err := s.LoadAccessToken()
	return updatedAt, token != "", err
}

func (s *RabinCredentialStore) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var firstErr error
	for _, path := range []string{s.path, s.sessionPath} {
		err := os.Remove(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *RabinCredentialStore) Render(w http.ResponseWriter, errorMessage, notice string) {
	username, saved, err := s.Status()
	if err != nil && errorMessage == "" {
		errorMessage = "خواندن اطلاعات ذخیره‌شده ممکن نشد: " + err.Error()
	}
	loginAt, hasSession, sessionErr := s.SessionStatus()
	if sessionErr != nil && errorMessage == "" {
		errorMessage = "خواندن نشست ذخیره‌شده رابین ممکن نشد."
	}
	loginAtText := ""
	if hasSession {
		loginAtText = formatJalaliDateTime(loginAt.In(tehran()))
	}
	data := RabinSettingsPageData{
		Username: username,
		Saved: saved,
		HasSession: hasSession,
		LoginAt: loginAtText,
		Error: errorMessage,
		Notice: notice,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := rabinSettingsPage.Execute(w, data); err != nil {
		// Do not include credential values in logs or error messages.
		fmt.Fprintln(w, "خطا در نمایش تنظیمات رابین")
	}
}
