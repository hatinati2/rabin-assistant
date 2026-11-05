package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	jalaali "rabin-assistant/internal/jalaali"
)

const (
	statusScheduled     = "scheduled"
	statusReady         = "ready"
	statusCancelled     = "cancelled"
	statusManualDone        = "manual_done"
	statusSubmitting        = "submitting"
	statusSubmitted         = "submitted"
	statusSubmissionFailed  = "submission_failed"
	statusSubmissionUnknown = "submission_unknown"
	executionModeManual     = "manual"
	executionModeAPI        = "api"
)

type Order struct {
	ID             string    `json:"id"`
	Symbol         string    `json:"symbol"`
	ISIN           string    `json:"isin,omitempty"`
	Side           string    `json:"side"`
	Price          int64     `json:"price"`
	Quantity       int64     `json:"quantity"`
	ExecuteAt      time.Time `json:"execute_at"`
	ExecutionMode  string    `json:"execution_mode,omitempty"`
	Status         string    `json:"status"`
	BrokerOrderID  string    `json:"broker_order_id,omitempty"`
	BrokerMessage  string    `json:"broker_message,omitempty"`
	AttemptedAt    time.Time `json:"attempted_at,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Store struct {
	mu     sync.Mutex
	path   string
	orders []Order
}

func newStore(path string) (*Store, error) {
	s := &Store{path: path, orders: []Order{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &s.orders); err != nil {
			return nil, fmt.Errorf("خواندن داده سفارش‌ها: %w", err)
		}
	}
	return s, nil
}

func (s *Store) snapshot() []Order {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]Order(nil), s.orders...)
	sort.Slice(out, func(i, j int) bool { return out[i].ExecuteAt.Before(out[j].ExecuteAt) })
	return out
}

func (s *Store) list(history bool) []Order {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Order, 0, len(s.orders))
	for _, order := range s.orders {
		if (order.Status != statusScheduled) == history {
			out = append(out, order)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if history {
			return out[i].ExecuteAt.After(out[j].ExecuteAt)
		}
		return out[i].ExecuteAt.Before(out[j].ExecuteAt)
	})
	return out
}

func (s *Store) activeSnapshot() []Order  { return s.list(false) }
func (s *Store) historySnapshot() []Order { return s.list(true) }

func (s *Store) add(order Order) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orders = append(s.orders, order)
	if err := s.saveLocked(); err != nil {
		s.orders = s.orders[:len(s.orders)-1]
		return err
	}
	return nil
}

func (s *Store) confirmManual(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.orders {
		if s.orders[i].ID != id {
			continue
		}
		mode := s.orders[i].ExecutionMode
		if mode == "" {
			mode = executionModeManual
		}
		if mode != executionModeManual || s.orders[i].Status != statusReady {
			return errors.New("فقط سفارش آماده در مسیر بدون API قابل تأیید دستی است")
		}
		old := s.orders[i]
		s.orders[i].Status = statusManualDone
		s.orders[i].UpdatedAt = time.Now().In(tehran())
		if err := s.saveLocked(); err != nil {
			s.orders[i] = old
			return err
		}
		return nil
	}
	return errors.New("سفارش پیدا نشد")
}

func (s *Store) cancel(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.orders {
		if s.orders[i].ID != id {
			continue
		}
		orderMode := s.orders[i].ExecutionMode
		if orderMode == "" {
			orderMode = executionModeManual
		}
		readyAPI := s.orders[i].Status == statusReady && orderMode == executionModeAPI
		if s.orders[i].Status != statusScheduled && !readyAPI {
			return errors.New("فقط سفارش زمان‌بندی‌شده یا سفارش API آماده ارسال قابل لغو است")
		}
		old := s.orders[i]
		s.orders[i].Status = statusCancelled
		s.orders[i].UpdatedAt = time.Now().In(tehran())
		if err := s.saveLocked(); err != nil {
			s.orders[i] = old
			return err
		}
		return nil
	}
	return errors.New("سفارش پیدا نشد")
}

func (s *Store) markDue(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for i := range s.orders {
		if s.orders[i].Status == statusScheduled && !s.orders[i].ExecuteAt.After(now) {
			s.orders[i].Status = statusReady
			s.orders[i].UpdatedAt = now
			changed = true
		}
	}
	if changed {
		return s.saveLocked()
	}
	return nil
}

func (s *Store) saveLocked() error {
	data, err := json.MarshalIndent(s.orders, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

var tehranLocation = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Tehran")
	if err != nil {
		return time.FixedZone("IRST", 3*3600+30*60)
	}
	return loc
}()

func tehran() *time.Location { return tehranLocation }

var persianDigitReplacer = strings.NewReplacer(
	"۰", "0", "۱", "1", "۲", "2", "۳", "3", "۴", "4",
	"۵", "5", "۶", "6", "۷", "7", "۸", "8", "۹", "9",
	"٠", "0", "١", "1", "٢", "2", "٣", "3", "٤", "4",
	"٥", "5", "٦", "6", "٧", "7", "٨", "8", "٩", "9",
)

func parseJalaliDateTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(persianDigitReplacer.Replace(raw))
	fields := strings.Fields(raw)
	if len(fields) != 2 {
		return time.Time{}, errors.New("فرمت تاریخ و ساعت باید مانند 1405/08/14 08:44:50 باشد")
	}

	dateParts := strings.FieldsFunc(fields[0], func(r rune) bool { return r == '/' || r == '-' || r == '.' })
	if len(dateParts) != 3 {
		return time.Time{}, errors.New("فرمت تاریخ شمسی نامعتبر است")
	}
	year, errY := strconv.Atoi(dateParts[0])
	month, errM := strconv.Atoi(dateParts[1])
	day, errD := strconv.Atoi(dateParts[2])
	if errY != nil || errM != nil || errD != nil || !jalaali.IsValidDate(year, month, day) {
		return time.Time{}, errors.New("تاریخ شمسی معتبر نیست")
	}

	timeParts := strings.Split(fields[1], ":")
	if len(timeParts) != 3 {
		return time.Time{}, errors.New("ساعت باید به شکل ساعت:دقیقه:ثانیه و میلی‌ثانیه اختیاری باشد")
	}
	secondParts := strings.Split(timeParts[2], ".")
	if len(secondParts) > 2 {
		return time.Time{}, errors.New("فرمت میلی‌ثانیه نامعتبر است")
	}
	hour, errH := strconv.Atoi(timeParts[0])
	minute, errMin := strconv.Atoi(timeParts[1])
	second, errSec := strconv.Atoi(secondParts[0])
	millisecond := 0
	if len(secondParts) == 2 {
		fraction := secondParts[1]
		if len(fraction) < 1 || len(fraction) > 3 {
			return time.Time{}, errors.New("میلی‌ثانیه باید یک تا سه رقم باشد")
		}
		for len(fraction) < 3 {
			fraction += "0"
		}
		millisecond, errSec = strconv.Atoi(fraction)
	}
	if errH != nil || errMin != nil || errSec != nil ||
		hour < 0 || hour > 23 || minute < 0 || minute > 59 || second < 0 || second > 59 ||
		millisecond < 0 || millisecond > 999 {
		return time.Time{}, errors.New("ساعت واردشده معتبر نیست")
	}

	gy, gm, gd, err := jalaali.ToGregorian(year, jalaali.Month(month), day)
	if err != nil {
		return time.Time{}, fmt.Errorf("تبدیل تاریخ شمسی: %w", err)
	}
	return time.Date(gy, gm, gd, hour, minute, second, millisecond*int(time.Millisecond), tehran()), nil
}

func formatJalaliDateTime(t time.Time) string {
	local := t.In(tehran())
	formatted, err := jalaali.From(local).JFormat("2006/01/02 15:04:05")
	if err != nil {
		formatted = local.Format("2006/01/02 15:04:05")
	}
	millis := fmt.Sprintf("%03d", local.Nanosecond()/int(time.Millisecond))
	persianMillis := strings.NewReplacer("0", "۰", "1", "۱", "2", "۲", "3", "۳", "4", "۴", "5", "۵", "6", "۶", "7", "۷", "8", "۸", "9", "۹").Replace(millis)
	return formatted + "٫" + persianMillis
}

func executionModeLabel(mode string) string {
	if mode == executionModeAPI {
		return "با API"
	}
	return "بدون API"
}

func isManualExecution(mode string) bool {
	return mode == "" || mode == executionModeManual
}

var symbolPattern = regexp.MustCompile(`^[\p{L}\p{N}._-]{1,24}$`)

func validateOrder(r *http.Request, now time.Time) (Order, error) {
	if err := r.ParseForm(); err != nil {
		return Order{}, errors.New("فرم معتبر نیست")
	}
	symbol := strings.TrimSpace(r.FormValue("symbol"))
	side := r.FormValue("side")
	executionMode := strings.TrimSpace(r.FormValue("execution_mode"))
	if executionMode == "" {
		executionMode = executionModeManual
	}
	if executionMode == executionModeAPI && !rabinOrderAPICapable() {
		return Order{}, errors.New("ارسال واقعی سفارش هنوز در تنظیمات سرور فعال و پیکربندی نشده است")
	}
	if executionMode != executionModeManual && executionMode != executionModeAPI {
		return Order{}, errors.New("مسیر اجرای سفارش معتبر نیست")
	}
	price, priceErr := strconv.ParseInt(r.FormValue("price"), 10, 64)
	quantity, quantityErr := strconv.ParseInt(r.FormValue("quantity"), 10, 64)
	isin := strings.ToUpper(strings.TrimSpace(r.FormValue("isin")))
	if executionMode == executionModeAPI && !regexp.MustCompile("^[A-Z]{2}[A-Z0-9]{9}[0-9]$").MatchString(isin) {
		return Order{}, errors.New("برای سفارش با API، شناسه ۱۲ کاراکتری ISIN معتبر وارد کنید")
	}
	if !symbolPattern.MatchString(symbol) {
		return Order{}, errors.New("نماد نامعتبر است")
	}
	if side != "buy" && side != "sell" {
		return Order{}, errors.New("نوع سفارش نامعتبر است")
	}
	if priceErr != nil || price <= 0 {
		return Order{}, errors.New("قیمت باید عدد صحیح مثبت و به ریال باشد")
	}
	if quantityErr != nil || quantity <= 0 {
		return Order{}, errors.New("تعداد باید عدد صحیح مثبت باشد")
	}
	rawExecuteAt := strings.TrimSpace(r.FormValue("execute_at"))
	if rawExecuteAt == "" {
		rawDate := strings.TrimSpace(r.FormValue("execute_at_date"))
		rawTime := strings.TrimSpace(r.FormValue("execute_time"))
		if rawDate != "" && rawTime != "" {
			rawExecuteAt = rawDate + " " + rawTime
		}
	}
	executeAt, err := parseJalaliDateTime(rawExecuteAt)
	if err != nil || !executeAt.After(now) {
		return Order{}, errors.New("تاریخ شمسی یا ساعت معتبر نیست، یا زمان باید در آینده باشد؛ نمونه: ۱۴۰۵/۰۸/۱۴ ۰۸:۴۴:۵۰")
	}
	return Order{
		ID:            fmt.Sprintf("%d", now.UnixNano()),
		Symbol:        symbol,
		ISIN:          isin,
		Side:          side,
		Price:         price,
		Quantity:      quantity,
		ExecuteAt:     executeAt,
		ExecutionMode: executionMode,
		Status:        statusScheduled,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

type PageData struct {
	Orders        []Order
	Error, Notice string
	Now           string
	JYear         int
	JMonth        int
	JDay          int
	History       bool
	APIAvailable  bool
}

func currentJalaliParts(t time.Time) (int, int, int) {
	y, m, d, err := jalaali.ToJalaali(t.In(tehran()).Year(), t.In(tehran()).Month(), t.In(tehran()).Day())
	if err != nil {
		return 1405, 1, 1
	}
	return y, int(m), d
}

type CalendarResponse struct {
	Year         int    `json:"year"`
	Month        int    `json:"month"`
	MonthName    string `json:"month_name"`
	Days         int    `json:"days"`
	FirstWeekday int    `json:"first_weekday"`
}

func buildCalendar(year, month int) (CalendarResponse, error) {
	if month < 1 || month > 12 || year < 1 || year > 3177 || !jalaali.IsValidDate(year, month, 1) {
		return CalendarResponse{}, errors.New("ماه یا سال شمسی نامعتبر است")
	}
	days, err := jalaali.MonthLength(year, month)
	if err != nil {
		return CalendarResponse{}, err
	}
	gy, gm, gd, err := jalaali.ToGregorian(year, jalaali.Month(month), 1)
	if err != nil {
		return CalendarResponse{}, err
	}
	weekday := int(jalaali.From(time.Date(gy, gm, gd, 0, 0, 0, 0, tehran())).Weekday())
	return CalendarResponse{
		Year: year, Month: month,
		MonthName: jalaali.Month(month).String(),
		Days: days, FirstWeekday: weekday,
	}, nil
}

var page = template.Must(template.New("page").Funcs(template.FuncMap{
	"jalali":        formatJalaliDateTime,
	"executionMode": executionModeLabel,
	"manualMode":    isManualExecution,
}).Parse(`<!doctype html>
<html lang="fa" dir="rtl"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>دستیار رابین</title>
<style>
*{box-sizing:border-box}body{margin:0;background:#f4f6fa;color:#182230;font:14px Tahoma,Arial}
.wrap{max-width:1100px;margin:30px auto;padding:0 16px}.head{display:flex;justify-content:space-between;align-items:center;gap:14px;margin-bottom:18px}
h1{font-size:22px;margin:0}.muted,.foot{color:#667085}.tag{background:#eef4ff;color:#1849a9;border-radius:18px;padding:7px 10px;font-size:12px}
.nav{display:flex;gap:7px;flex-wrap:wrap}.nav a{display:inline-block;padding:9px 12px;border:1px solid #d0d5dd;border-radius:9px;color:#344054;text-decoration:none;background:#fff}.nav a.active{background:#175cd3;color:#fff;border-color:#175cd3}
.note,.msg{padding:12px;border-radius:9px;margin:12px 0;line-height:1.8}.note{background:#fffaeb;border:1px solid #fedf89;color:#93370d}
.msg{background:#ecfdf3;color:#05603a}.err{background:#fef3f2;color:#b42318}
.grid{display:grid;grid-template-columns:320px 1fr;gap:16px}.grid.history{grid-template-columns:1fr}.card{background:white;border:1px solid #e4e7ec;border-radius:13px;padding:17px;min-width:0}
h2{font-size:16px;margin:0 0 14px}label{display:block;margin:12px 0 6px;font-weight:bold}
input,select{width:100%;padding:10px;border:1px solid #d0d5dd;border-radius:8px;font:inherit}
button{border:0;border-radius:8px;background:#175cd3;color:white;padding:10px 13px;font:inherit;cursor:pointer}
.submit{width:100%;margin-top:15px}.cancel{background:white;color:#b42318;border:1px solid #fecdca;padding:6px 9px}
table{width:100%;border-collapse:collapse;white-space:nowrap}th,td{text-align:right;padding:10px 7px;border-bottom:1px solid #eaecf0}
th{font-size:12px;color:#667085}.tablewrap{overflow:auto}.status{display:inline-block;border-radius:16px;padding:5px 8px;font-size:11px}
.scheduled{background:#eff8ff;color:#175cd3}.ready{background:#fffaeb;color:#b54708}.cancelled{background:#f2f4f7;color:#475467}
.buy{color:#067647;font-weight:bold}.sell{color:#b42318;font-weight:bold}.empty{text-align:center;color:#667085;padding:28px}
.foot{font-size:12px;line-height:1.8;margin-top:12px}
.datebox{position:relative}.datebar{display:flex;gap:8px}.datebar input{flex:1}.datebar button{white-space:nowrap}.calendar{margin-top:8px;padding:12px;border:1px solid #d0d5dd;border-radius:10px;background:#fff}.cal-head{display:flex;justify-content:space-between;align-items:center;gap:8px;margin-bottom:12px}.cal-nav{background:#f2f4f7;color:#344054;padding:7px 10px}.cal-grid{display:grid;grid-template-columns:repeat(7,minmax(0,1fr));gap:5px;text-align:center}.cal-grid span{padding:7px 0;color:#667085;font-size:12px}.cal-day{background:#f8fafc;color:#182230;padding:9px 2px;border-radius:7px}.cal-day:hover,.cal-day.selected{background:#175cd3;color:#fff}.cal-day.today{outline:2px solid #84adff}.cal-empty{min-height:34px}
@media(max-width:800px){.grid{grid-template-columns:1fr}.head{align-items:flex-start;gap:10px;flex-direction:column}}
</style></head><body data-jyear="{{.JYear}}" data-jmonth="{{.JMonth}}" data-jday="{{.JDay}}"><main class="wrap">
<div class="head"><div><h1>دستیار رابین</h1><div class="muted" style="margin-top:7px">مدیریت سفارش‌ها · ساعت تهران</div></div><div class="nav"><a class="{{if not .History}}active{{end}}" href="/">سفارش‌های فعال</a><a class="{{if .History}}active{{end}}" href="/history">تاریخچه</a><a href="/settings">اتصال به رابین</a></div><span class="tag">{{if .APIAvailable}}ارسال واقعی API فعال است{{else}}ارسال واقعی API غیرفعال است{{end}}</span></div>
<div class="note"><b>دو مسیر اجرا:</b> مسیر «بدون API» سفارش را برای ثبت دستی آماده می‌کند. مسیر «با API» فقط در صورت فعال‌سازی صریح در تنظیمات سرور، وجود نشست معتبر و ثبت ISIN، در زمان مقرر یک درخواست سفارش عادی به رابین ارسال می‌کند. در صورت نامشخص بودن نتیجه ارسال، سیستم سفارش را خودکار تکرار نمی‌کند.</div>
{{if .Error}}<div class="msg err">{{.Error}}</div>{{end}}{{if .Notice}}<div class="msg">{{.Notice}}</div>{{end}}
<div class="grid{{if .History}} history{{end}}">{{if not .History}}<section class="card"><h2>تعریف سفارش</h2>
<form id="order-form" method="post" action="/orders"><label>مسیر ثبت سفارش</label><select id="execution-mode" name="execution_mode"><option value="manual" selected>بدون API — ثبت دستی در رابین</option><option value="api" {{if not .APIAvailable}}disabled{{end}}>با API — ارسال عادی به رابین</option></select>
<label>نماد نمایشی</label><input name="symbol" maxlength="24" placeholder="مثلاً خودرو" required>
<label>شناسه ISIN (فقط برای مسیر API)</label><input id="order-isin" name="isin" maxlength="12" minlength="12" pattern="[A-Za-z]{2}[A-Za-z0-9]{9}[0-9]" placeholder="مثلاً IR..." autocomplete="off">
<label>نوع سفارش</label><select name="side"><option value="buy">خرید</option><option value="sell">فروش</option></select>
<label>قیمت (ریال)</label><input name="price" type="number" min="1" step="1" required>
<label>تعداد</label><input name="quantity" type="number" min="1" step="1" required>
<label>تاریخ شمسی</label>
<div class="datebox">
  <div class="datebar"><input id="jalali-date-display" name="execute_at_date" type="text" value="" inputmode="numeric" dir="ltr" placeholder="مثلاً ۱۴۰۵/۰۸/۱۴" pattern="[0-9۰-۹٠-٩]{4}[/.-][0-9۰-۹٠-٩]{1,2}[/.-][0-9۰-۹٠-٩]{1,2}" autocomplete="off" required><button type="button" id="calendar-toggle">تقویم</button></div>
  <div id="jalali-calendar" class="calendar" hidden>
    <div class="cal-head"><button type="button" id="cal-prev" class="cal-nav">ماه قبل</button><strong id="cal-title">تقویم شمسی</strong><button type="button" id="cal-next" class="cal-nav">ماه بعد</button></div>
    <div class="cal-grid" id="cal-weekdays"><span>ش</span><span>ی</span><span>د</span><span>س</span><span>چ</span><span>پ</span><span>ج</span></div>
    <div class="cal-grid" id="cal-days"></div>
  </div>
</div>
<label>ساعت اجرا (تهران، با دقت میلی‌ثانیه)</label><input id="execute-time" name="execute_time" type="time" step="0.001" value="08:44:50.000" required>
<button class="submit" type="submit">ذخیره سفارش</button></form>
<div class="foot">قیمت به ریال و تعداد به عدد سهم وارد شود. در مسیر API، ISIN دقیق نماد الزامی است.</div></section>{{end}}
<section class="card"><h2>{{if .History}}تاریخچه سفارش‌ها{{else}}سفارش‌های فعال{{end}} ({{len .Orders}})</h2><div class="tablewrap"><table>
<thead><tr><th>نماد</th><th>نوع</th><th>قیمت</th><th>تعداد</th><th>زمان تهران</th><th>مسیر</th><th>وضعیت</th><th>عملیات</th></tr></thead><tbody>
{{range .Orders}}<tr><td><b>{{.Symbol}}</b>{{if .ISIN}}<div class="foot">{{.ISIN}}</div>{{end}}</td><td>{{if eq .Side "buy"}}<span class="buy">خرید</span>{{else}}<span class="sell">فروش</span>{{end}}</td>
<td>{{.Price}}</td><td>{{.Quantity}}</td><td>{{jalali .ExecuteAt}}</td><td>{{executionMode .ExecutionMode}}</td>
<td>{{if eq .Status "scheduled"}}<span class="status scheduled">زمان‌بندی‌شده</span>{{else if eq .Status "ready"}}<span class="status ready">{{if eq .ExecutionMode "api"}}آماده ارسال{{else}}آماده بررسی{{end}}</span>{{else if eq .Status "manual_done"}}<span class="status ready">ثبت دستی تأیید شد</span>{{else if eq .Status "submitting"}}<span class="status ready">در حال ارسال</span>{{else if eq .Status "submitted"}}<span class="status ready">ثبت در رابین</span>{{else if eq .Status "submission_failed"}}<span class="status cancelled">ارسال ناموفق</span>{{else if eq .Status "submission_unknown"}}<span class="status cancelled">نتیجه نامشخص — بررسی لازم</span>{{else}}<span class="status cancelled">لغوشده</span>{{end}}{{if .BrokerOrderID}}<div class="foot">شناسه رابین: {{.BrokerOrderID}}</div>{{end}}{{if .BrokerMessage}}<div class="foot">{{.BrokerMessage}}</div>{{end}}</td>
<td>{{if or (eq .Status "scheduled") (and (eq .Status "ready") (eq .ExecutionMode "api"))}}<form method="post" action="/orders/cancel"><input type="hidden" name="id" value="{{.ID}}"><button class="cancel">لغو</button></form>{{else if and $.History (eq .Status "ready") (manualMode .ExecutionMode)}}<form method="post" action="/orders/manual-confirm" onsubmit="return confirm('آیا سفارش را شخصاً در سامانه رابین ثبت کرده‌ای؟')"><input type="hidden" name="id" value="{{.ID}}"><button class="submit">ثبت دستی انجام شد</button></form>{{else}}—{{end}}</td></tr>
{{else}}<tr><td colspan="8" class="empty">{{if .History}}هنوز موردی در تاریخچه ثبت نشده است.{{else}}سفارش فعالی ثبت نشده است.{{end}}</td></tr>{{end}}
</tbody></table></div><div class="foot">{{if .History}}این صفحه سفارش‌های سررسیدشده و لغوشده را نگه می‌دارد؛ هیچ سفارشی از داده‌ها حذف نشده است.{{else}}ساعت سرور: {{.Now}} · سفارش‌های سررسیدشده در صفحه تاریخچه نمایش داده می‌شوند.{{end}} وضعیت «آماده بررسی» به معنی ارسال به کارگزاری نیست.</div>
</section></div></main>
<script>
(() => {
  const $ = (id) => document.getElementById(id);
  const state = {
    year: Number(document.body.dataset.jyear),
    month: Number(document.body.dataset.jmonth),
    day: Number(document.body.dataset.jday)
  };
  const fa = (value) => String(value).replace(/[0-9]/g, d => "۰۱۲۳۴۵۶۷۸۹"[Number(d)]);
  const latin = (value) => String(value).replace(/[۰-۹٠-٩]/g, d => {
    const faDigits = "۰۱۲۳۴۵۶۷۸۹";
    const arDigits = "٠١٢٣٤٥٦٧٨٩";
    const index = faDigits.indexOf(d);
    return String(index >= 0 ? index : arDigits.indexOf(d));
  });
  const pad = (value) => String(value).padStart(2, "0");
  const display = $("jalali-date-display");
  const calendar = $("jalali-calendar");
  const executionMode = $("execution-mode");
  const isinInput = $("order-isin");
  function syncExecutionMode() {
    const apiMode = executionMode && executionMode.value === "api";
    if (isinInput) {
      isinInput.required = !!apiMode;
      isinInput.disabled = !apiMode;
    }
  }
  if (executionMode) {
    executionMode.addEventListener("change", syncExecutionMode);
    syncExecutionMode();
  }
  const orderForm = $("order-form");
  if (orderForm) {
    orderForm.addEventListener("submit", (event) => {
      if (executionMode && executionMode.value === "api") {
        const accepted = window.confirm(
          "این گزینه یک سفارش واقعی عادی را به کارگزاری رابین می‌فرستد. اگر کارگزاری آن را در تعطیلی بپذیرد، ممکن است در اولین زمان مجاز بازار اجرا شود. آیا ثبت این سفارش زمان‌بندی‌شده را تأیید می‌کنید؟"
        );
        if (!accepted) event.preventDefault();
      }
    });
  }

  function parseTypedDate() {
    const raw = latin(display.value.trim());
    const match = raw.match(/^(\d{4})[\/.-](\d{1,2})[\/.-](\d{1,2})$/);
    if (!match) return null;
    const year = Number(match[1]), month = Number(match[2]), day = Number(match[3]);
    if (month < 1 || month > 12 || day < 1 || day > 31) return null;
    return {year, month, day};
  }

  function showDate(year, month, day) {
    display.value = fa(year + "/" + pad(month) + "/" + pad(day));
  }

  function syncCalendarToTypedDate() {
    const parsed = parseTypedDate();
    if (parsed) Object.assign(state, parsed);
  }

  async function renderCalendar() {
    const response = await fetch("/api/calendar?year=" + state.year + "&month=" + state.month, {
      headers: {"Accept": "application/json"}
    });
    if (!response.ok) {
      $("cal-title").textContent = "خطا در بارگذاری تقویم";
      return;
    }
    const data = await response.json();
    $("cal-title").textContent = data.month_name + " " + fa(data.year);
    const days = $("cal-days");
    days.replaceChildren();

    for (let i = 0; i < data.first_weekday; i++) {
      const blank = document.createElement("span");
      blank.className = "cal-empty";
      days.appendChild(blank);
    }

    for (let day = 1; day <= data.days; day++) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "cal-day" +
        (state.year === data.year && state.month === data.month && state.day === day ? " selected" : "");
      button.textContent = fa(day);
      button.addEventListener("click", () => {
        state.year = data.year;
        state.month = data.month;
        state.day = day;
        showDate(state.year, state.month, state.day);
        calendar.hidden = true;
      });
      days.appendChild(button);
    }
  }

  $("calendar-toggle").addEventListener("click", () => {
    syncCalendarToTypedDate();
    calendar.hidden = !calendar.hidden;
    if (!calendar.hidden) renderCalendar().catch(() => {
      $("cal-title").textContent = "خطا در بارگذاری تقویم";
    });
  });

  $("cal-prev").addEventListener("click", () => {
    state.month--;
    if (state.month < 1) { state.month = 12; state.year--; }
    state.day = 0;
    renderCalendar().catch(() => { $("cal-title").textContent = "خطا در بارگذاری تقویم"; });
  });

  $("cal-next").addEventListener("click", () => {
    state.month++;
    if (state.month > 12) { state.month = 1; state.year++; }
    state.day = 0;
    renderCalendar().catch(() => { $("cal-title").textContent = "خطا در بارگذاری تقویم"; });
  });

  display.addEventListener("input", syncCalendarToTypedDate);
})();
</script></body></html>`))

func main() {
	user, pass := os.Getenv("RABIN_ADMIN_USER"), os.Getenv("RABIN_ADMIN_PASS")
	if user == "" || pass == "" {
		log.Fatal("RABIN_ADMIN_USER و RABIN_ADMIN_PASS باید تنظیم شوند")
	}
	addr := env("RABIN_ADDR", "0.0.0.0:8098")
	if strings.TrimSpace(addr) == "" {
		log.Fatal("RABIN_ADDR خالی است")
	}
	dataDir := env("RABIN_DATA_DIR", "./data")
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		log.Fatal(err)
	}
	store, err := newStore(filepath.Join(dataDir, "orders.json"))
	if err != nil {
		log.Fatal(err)
	}

	credentials := NewRabinCredentialStore(dataDir, env("RABIN_CREDENTIALS_KEY_FILE", "/etc/rabin-assistant.key"))
	if err := store.recoverInterruptedAPIOrders(time.Now().In(tehran())); err != nil {
		log.Printf("recover interrupted API orders: %v", err)
	}
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for now := range ticker.C {
			now = now.In(tehran())
			if err := store.markDue(now); err != nil {
				log.Printf("scheduler: %v", err)
			}
			if rabinOrderAPICapable() {
				dispatchReadyRabinOrders(store, credentials, now)
			}
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/calendar", func(w http.ResponseWriter, r *http.Request) {
		year, errY := strconv.Atoi(r.URL.Query().Get("year"))
		month, errM := strconv.Atoi(r.URL.Query().Get("month"))
		if errY != nil || errM != nil {
			http.Error(w, "سال و ماه شمسی نامعتبر است", http.StatusBadRequest)
			return
		}
		data, err := buildCalendar(year, month)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(data)
	})
	renderOrdersPage := func(w http.ResponseWriter, r *http.Request, history bool) {
		now := time.Now().In(tehran())
		if err := store.markDue(now); err != nil {
			log.Printf("mark due before page render: %v", err)
		}
		year, month, day := currentJalaliParts(now)
		orders := store.activeSnapshot()
		if history {
			orders = store.historySnapshot()
		}
		data := PageData{Orders: orders, Now: formatJalaliDateTime(now), JYear: year, JMonth: month, JDay: day, History: history, APIAvailable: rabinOrderAPICapable()}
		switch r.URL.Query().Get("notice") {
		case "saved":
			data.Notice = "سفارش ذخیره شد."
		case "cancelled":
			data.Notice = "سفارش لغو شد."
		case "manual_confirmed":
			data.Notice = "تأیید دستی ثبت شد. این تأیید به معنی راستی‌آزمایی خودکار کارگزاری نیست."
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := page.Execute(w, data); err != nil {
			log.Printf("template: %v", err)
		}
	}
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		renderOrdersPage(w, r, false)
	})

	mux.HandleFunc("GET /settings", func(w http.ResponseWriter, r *http.Request) {
		notice := ""
		if r.URL.Query().Get("notice") == "rabin_saved" {
			notice = "اطلاعات ورود رابین به‌صورت رمزنگاری‌شده ذخیره شد."
		} else if r.URL.Query().Get("notice") == "rabin_deleted" {
			notice = "اطلاعات ورود رابین حذف شد."
		} else if r.URL.Query().Get("notice") == "rabin_login_ok" {
			notice = "ورود عادی به رابین موفق بود و نشست رمزنگاری‌شده ذخیره شد."
		}
		credentials.Render(w, "", notice)
	})
	mux.HandleFunc("POST /settings/rabin-credentials", func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			http.Error(w, "درخواست نامعتبر", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
		if err := r.ParseForm(); err != nil {
			credentials.Render(w, "فرم معتبر نیست یا حجم آن بیش از حد مجاز است.", "")
			return
		}
		if err := credentials.Save(r.FormValue("username"), r.FormValue("password")); err != nil {
			credentials.Render(w, err.Error(), "")
			return
		}
		http.Redirect(w, r, "/settings?notice=rabin_saved", http.StatusSeeOther)
	})
	mux.HandleFunc("POST /settings/rabin-credentials/delete", func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			http.Error(w, "درخواست نامعتبر", http.StatusForbidden)
			return
		}
		if err := credentials.Delete(); err != nil {
			credentials.Render(w, "حذف اطلاعات ورود انجام نشد.", "")
			return
		}
		http.Redirect(w, r, "/settings?notice=rabin_deleted", http.StatusSeeOther)
	})

	mux.HandleFunc("POST /settings/rabin-login", func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			http.Error(w, "درخواست نامعتبر", http.StatusForbidden)
			return
		}
		username, password, err := credentials.Load()
		if err != nil {
			credentials.Render(w, "خواندن اطلاعات ورود ذخیره‌شده ممکن نشد.", "")
			return
		}
		if username == "" || password == "" {
			credentials.Render(w, "ابتدا نام کاربری و رمز عبور رابین را ذخیره کنید.", "")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 13*time.Second)
		defer cancel()
		token, err := (RabinLoginClient{}).Login(ctx, username, password)
		if err != nil {
			credentials.Render(w, err.Error(), "")
			return
		}
		if err := credentials.SaveAccessToken(token); err != nil {
			credentials.Render(w, "ورود موفق بود، اما ذخیره امن نشست انجام نشد.", "")
			return
		}
		http.Redirect(w, r, "/settings?notice=rabin_login_ok", http.StatusSeeOther)
	})
	mux.HandleFunc("GET /history", func(w http.ResponseWriter, r *http.Request) {
		renderOrdersPage(w, r, true)
	})
	mux.HandleFunc("POST /orders", func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			log.Printf("POST /orders rejected: host=%q origin=%q referer=%q fetch-site=%q", r.Host, r.Header.Get("Origin"), r.Referer(), r.Header.Get("Sec-Fetch-Site"))
			http.Error(w, "درخواست نامعتبر", http.StatusForbidden)
			return
		}
		now := time.Now().In(tehran())
		order, err := validateOrder(r, now)
		if err == nil {
			err = store.add(order)
		}
		if err != nil {
			renderError(w, store, err.Error())
			return
		}
		http.Redirect(w, r, "/?notice=saved", http.StatusSeeOther)
	})
	mux.HandleFunc("POST /orders/manual-confirm", func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			http.Error(w, "درخواست نامعتبر", http.StatusForbidden)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "فرم نامعتبر است", http.StatusBadRequest)
			return
		}
		if err := store.confirmManual(r.FormValue("id")); err != nil {
			renderError(w, store, err.Error())
			return
		}
		http.Redirect(w, r, "/history?notice=manual_confirmed", http.StatusSeeOther)
	})
	mux.HandleFunc("POST /orders/cancel", func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			http.Error(w, "درخواست نامعتبر", http.StatusForbidden)
			return
		}
		if err := r.ParseForm(); err != nil {
			renderError(w, store, "فرم نامعتبر است")
			return
		}
		if err := store.cancel(r.FormValue("id")); err != nil {
			renderError(w, store, err.Error())
			return
		}
		http.Redirect(w, r, "/?notice=cancelled", http.StatusSeeOther)
	})

	handler := securityHeaders(basicAuth(user, pass, mux))
	server := &http.Server{
		Addr: addr, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second,
	}
	log.Printf("Rabin Assistant listening on %s", addr)
	log.Fatal(server.ListenAndServe())
}

func renderError(w http.ResponseWriter, store *Store, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = page.Execute(w, PageData{
		Orders: store.snapshot(), Error: message,
		Now: formatJalaliDateTime(time.Now().In(tehran())),
		JYear: func() int { y, _, _ := currentJalaliParts(time.Now().In(tehran())); return y }(),
		JMonth: func() int { _, m, _ := currentJalaliParts(time.Now().In(tehran())); return m }(),
		JDay: func() int { _, _, d := currentJalaliParts(time.Now().In(tehran())); return d }(),
		APIAvailable: rabinOrderAPICapable(),
	})
}

func sameOrigin(r *http.Request) bool {
	// Prefer the Origin header for POST requests. Some browsers or proxies omit
	// it for ordinary form submissions, so fall back to Referer and then
	// Fetch Metadata when available.
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" && !strings.EqualFold(origin, "null") {
		return sameHostURL(origin, r.Host)
	}
	if referer := strings.TrimSpace(r.Referer()); referer != "" {
		return sameHostURL(referer, r.Host)
	}
	if fetchSite := strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")); fetchSite != "" {
		return strings.EqualFold(fetchSite, "same-origin")
	}
	// Some browsers and reverse proxies omit all three headers on normal form POSTs.
	// Basic authentication still protects the panel; do not reject these requests.
	return true
}

func sameHostURL(rawURL, requestHost string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" || requestHost == "" {
		return false
	}
	return strings.EqualFold(u.Host, requestHost)
}

func basicAuth(user, pass string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, ok := r.BasicAuth()
		userOK := subtle.ConstantTimeCompare([]byte(gotUser), []byte(user)) == 1
		passOK := subtle.ConstantTimeCompare([]byte(gotPass), []byte(pass)) == 1
		if !ok || !userOK || !passOK {
			w.Header().Set("WWW-Authenticate", `Basic realm="Rabin Assistant", charset="UTF-8"`)
			http.Error(w, "احراز هویت لازم است", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		next.ServeHTTP(w, r)
	})
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
