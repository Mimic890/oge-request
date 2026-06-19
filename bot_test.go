package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestFixGradeSpacing(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"5ЗАЧЕТ", "5 ЗАЧЕТ"},
		{"4ХОРОШО", "4 ХОРОШО"},
		{"3УДОВЛЕТВОРИТЕЛЬНО", "3 УДОВЛЕТВОРИТЕЛЬНО"},
		{"5 ЗАЧЕТ", "5 ЗАЧЕТ"},
		{"", ""},
		{"ЗАЧЕТ", "ЗАЧЕТ"},
		{"5б", "5 б"},
		{"5ё", "5 ё"},
	}
	for _, tt := range tests {
		got := fixGradeSpacing(tt.input)
		if got != tt.want {
			t.Errorf("fixGradeSpacing(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsLetter(t *testing.T) {
	letters := []rune{'A', 'Z', 'a', 'z', 'А', 'Я', 'а', 'я', 'ё', 'Ё', 'Б', 'м'}
	for _, r := range letters {
		if !isLetter(r) {
			t.Errorf("isLetter(%c) = false, want true", r)
		}
	}
	nonLetters := []rune{'0', '9', ' ', '-', '.', '5'}
	for _, r := range nonLetters {
		if isLetter(r) {
			t.Errorf("isLetter(%c) = true, want false", r)
		}
	}
}

func TestFormatResults(t *testing.T) {
	results := map[string]Result{
		"9 - Математика": {
			Date:  "02.06.2026",
			Score: "Первичный балл: 23 (74%)",
			Grade: "5 ЗАЧЕТ",
		},
	}
	got := FormatResults(results)
	if got == "" {
		t.Error("FormatResults returned empty string")
	}
	if !contains(got, "9 - Математика") {
		t.Error("FormatResults missing subject name")
	}
	if !contains(got, "5 ЗАЧЕТ") {
		t.Error("FormatResults missing grade")
	}
	if !contains(got, "02.06.2026") {
		t.Error("FormatResults missing date")
	}
}

func TestDiffResults(t *testing.T) {
	old := map[string]Result{
		"Мат": {Date: "01.06", Score: "20", Grade: "4 ХОРОШО"},
	}
	cur := map[string]Result{
		"Мат": {Date: "01.06", Score: "23", Grade: "5 ЗАЧЕТ"},
		"Рус": {Date: "02.06", Score: "18", Grade: "4 ХОРОШО"},
	}
	changed := DiffResults(old, cur)
	if len(changed) != 2 {
		t.Errorf("DiffResults returned %d changed, want 2: %v", len(changed), changed)
	}

	changed = DiffResults(cur, cur)
	if len(changed) != 0 {
		t.Errorf("DiffResults on identical maps returned %d, want 0", len(changed))
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tt := range tests {
		got := FormatBytes(tt.input)
		if got != tt.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMaskCode(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"3426-5251-3725", "3426****3725"},
		{"12", "****"},
		{"1234-5678-9012", "1234****9012"},
	}
	for _, tt := range tests {
		got := maskCode(tt.input)
		if got != tt.want {
			t.Errorf("maskCode(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestStorageSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStorage(dir)
	if err != nil {
		t.Fatal(err)
	}

	s.SaveUser(12345, "3426-5251-3725", "")
	users := s.LoadUsers()
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if users[12345].Code != "3426-5251-3725" {
		t.Errorf("code = %q, want %q", users[12345].Code, "3426-5251-3725")
	}

	changed := s.UpdateUserResults(12345, map[string]Result{
		"Мат": {Date: "01.06", Score: "23", Grade: "5 ЗАЧЕТ"},
	})
	if len(changed) != 1 {
		t.Errorf("expected 1 changed, got %d", len(changed))
	}

	results := s.GetUserResults(12345)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results["Мат"].Grade != "5 ЗАЧЕТ" {
		t.Errorf("grade = %q, want %q", results["Мат"].Grade, "5 ЗАЧЕТ")
	}

	s.RemoveUser(12345)
	users = s.LoadUsers()
	if len(users) != 0 {
		t.Errorf("expected 0 users after remove, got %d", len(users))
	}
}

func TestStoragePersistence(t *testing.T) {
	dir := t.TempDir()

	s1, _ := NewStorage(dir)
	s1.SaveUser(111, "1111-1111-1111", "")
	s1.UpdateUserResults(111, map[string]Result{
		"Рус": {Date: "01.06", Score: "20", Grade: "4 ХОРОШО"},
	})

	s2, _ := NewStorage(dir)
	users := s2.LoadUsers()
	if len(users) != 1 {
		t.Fatalf("expected 1 user after reload, got %d", len(users))
	}
	results := s2.GetUserResults(111)
	if results["Рус"].Score != "20" {
		t.Errorf("state not persisted: score = %q", results["Рус"].Score)
	}
}

func TestStorageJSONFilesExist(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStorage(dir)
	s.SaveUser(999, "9999-9999-9999", "")
	s.UpdateUserResults(999, map[string]Result{"Тест": {Grade: "5"}})

	if _, err := os.Stat(filepath.Join(dir, "users.json")); os.IsNotExist(err) {
		t.Error("users.json not created")
	}
	if _, err := os.Stat(filepath.Join(dir, "state.json")); os.IsNotExist(err) {
		t.Error("state.json not created")
	}
}

func TestActiveUsers(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStorage(dir)

	s.SaveUser(1, "1111-1111-1111", "")
	s.SaveUser(2, "2222-2222-2222", "")
	s.UpdateUserResults(1, map[string]Result{"Мат": {Grade: "5"}})

	total, active := s.ActiveUsers()
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if active != 1 {
		t.Errorf("active = %d, want 1", active)
	}
}

func TestParseResultsEmpty(t *testing.T) {
	results, err := parseResults("<html><body>Ничего нет</body></html>")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results from empty page, got %d", len(results))
	}
}

func TestUserEntryDefaults(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStorage(dir)
	s.SaveUser(100, "1234-5678-9012", "")

	users := s.LoadUsers()
	entry := users[100]
	if !entry.Enabled {
		t.Error("default Enabled should be true")
	}
	if entry.Interval != 5 {
		t.Errorf("default Interval = %d, want 5", entry.Interval)
	}
}

func TestGetInterval(t *testing.T) {
	tests := []struct {
		interval int
		want     int
	}{
		{0, 5},
		{-5, 5},
		{5, 5},
		{15, 15},
	}
	for _, tt := range tests {
		e := UserEntry{Interval: tt.interval}
		got := e.GetInterval()
		if got != tt.want {
			t.Errorf("GetInterval(%d) = %d, want %d", tt.interval, got, tt.want)
		}
	}
}

func TestNotifEnabled(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStorage(dir)
	s.SaveUser(200, "1234-5678-9012", "")

	if !s.GetNotifEnabled(200) {
		t.Error("default notif should be enabled")
	}

	s.SetNotifEnabled(200, false)
	if s.GetNotifEnabled(200) {
		t.Error("notif should be disabled after SetNotifEnabled(false)")
	}

	s.SetNotifEnabled(200, true)
	if !s.GetNotifEnabled(200) {
		t.Error("notif should be enabled after SetNotifEnabled(true)")
	}
}

func TestCheckInterval(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStorage(dir)
	s.SaveUser(300, "1234-5678-9012", "")

	if s.GetCheckInterval(300) != 5 {
		t.Error("default interval should be 5")
	}

	s.SetCheckInterval(300, 10)
	if s.GetCheckInterval(300) != 10 {
		t.Error("interval should be 10 after SetCheckInterval(10)")
	}
}

func TestNeedsCheck(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStorage(dir)
	s.SaveUser(400, "1234-5678-9012", "")

	if !s.NeedsCheck(400) {
		t.Error("new user should need check")
	}

	s.UpdateLastCheck(400)
	if s.NeedsCheck(400) {
		t.Error("just checked user should not need check")
	}
}

func TestNeedsCheckExpired(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStorage(dir)
	s.SaveUser(500, "1234-5678-9012", "")
	s.SetCheckInterval(500, 1)

	s.mu.Lock()
	entry := s.users[500]
	entry.LastCheck = time.Now().Add(-2 * time.Minute)
	s.users[500] = entry
	s.mu.Unlock()

	if !s.NeedsCheck(500) {
		t.Error("user with expired interval should need check")
	}
}

func TestUpdateLastActive(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStorage(dir)
	s.SaveUser(600, "1234-5678-9012", "")

	users := s.LoadUsers()
	if users[600].LastActive.IsZero() {
		t.Error("LastActive should be set after SaveUser")
	}

	s.UpdateLastActive(600)
	users = s.LoadUsers()
	if users[600].LastActive.IsZero() {
		t.Error("LastActive should be updated")
	}
}

func TestWarningSent(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStorage(dir)
	s.SaveUser(700, "1234-5678-9012", "")

	users := s.LoadUsers()
	if users[700].WarningSent {
		t.Error("WarningSent should default to false")
	}

	s.SetWarningSent(700)
	users = s.LoadUsers()
	if !users[700].WarningSent {
		t.Error("WarningSent should be true after SetWarningSent")
	}
}

func TestMsgFunctions(t *testing.T) {
	tests := []struct {
		name string
		fn   func() string
	}{
		{"msgWelcome", func() string { return msgWelcome("ege-kostroma.ru") }},
		{"msgSetCode", msgSetCode},
		{"msgCodeSaved", msgCodeSaved},
		{"msgInvalidCode", msgInvalidCode},
		{"msgNoResults", msgNoResults},
		{"msgSiteUnavailable", func() string { return msgSiteUnavailable("@admin") }},
		{"msgCodeDeleted", msgCodeDeleted},
		{"msgDeleteConfirm", msgDeleteConfirm},
		{"msgResultsHeader", msgResultsHeader},
		{"msgUpdateHeader", msgUpdateHeader},
		{"msgInactivityFarewell", msgInactivityFarewell},
		{"msgNotifToggled(true)", func() string { return msgNotifToggled(true) }},
		{"msgNotifToggled(false)", func() string { return msgNotifToggled(false) }},
		{"msgIntervalChanged(15)", func() string { return msgIntervalChanged(15) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn()
			if got == "" {
				t.Errorf("%s() returned empty string", tt.name)
			}
		})
	}
}

func TestMsgWelcomeRegistered(t *testing.T) {
	got := msgWelcomeRegistered("ege-kostroma.ru", "3426****3725", 30)
	if got == "" {
		t.Error("msgWelcomeRegistered returned empty string")
	}
	if !contains(got, "ege-kostroma.ru") {
		t.Error("missing site domain")
	}
	if !contains(got, "3426****3725") {
		t.Error("missing masked code")
	}
	if !contains(got, "30") {
		t.Error("missing interval")
	}
}

func TestMsgNotifSettings(t *testing.T) {
	got := msgNotifSettings(true, 15)
	if !contains(got, "15") {
		t.Error("missing interval")
	}
	if !contains(got, "Включены") {
		t.Error("missing enabled status")
	}

	got = msgNotifSettings(false, 60)
	if !contains(got, "60") {
		t.Error("missing interval")
	}
	if !contains(got, "Выключены") {
		t.Error("missing disabled status")
	}
}

func TestMsgInactivityWarning(t *testing.T) {
	got := msgInactivityWarning(54)
	if !contains(got, "54") {
		t.Error("missing days count")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestRemoveUserCleansUpAll(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStorage(dir)
	s.SaveUser(100, "1234-5678-9012", "testuser")
	ordered := NewOrderedResults()
	ordered.Add("Мат", "01.06", "23", "5 ЗАЧЕТ")
	s.UpdateUserResultsOrdered(100, ordered)

	s.RemoveUser(100)

	if len(s.LoadUsers()) != 0 {
		t.Error("user not removed from users")
	}
	if len(s.GetUserResults(100)) != 0 {
		t.Error("state not removed")
	}
	if len(s.GetUserOrder(100)) != 0 {
		t.Error("order not removed")
	}
	if !s.GetLastUpdated(100).IsZero() {
		t.Error("timing not removed")
	}
}

func TestNotifSettingsKeyboardIntervals(t *testing.T) {
	kb := notifSettingsKeyboard(true, 5)
	found5 := false
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.Text == "5 мин ✓" && btn.CallbackData != nil && *btn.CallbackData == "notif_5" {
				found5 = true
			}
		}
	}
	if !found5 {
		t.Error("missing 5 мин ✓ button with notif_5 callback")
	}

	kb = notifSettingsKeyboard(true, 10)
	found10 := false
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.Text == "10 мин ✓" && btn.CallbackData != nil && *btn.CallbackData == "notif_10" {
				found10 = true
			}
		}
	}
	if !found10 {
		t.Error("missing 10 мин ✓ button with notif_10 callback")
	}

	kb = notifSettingsKeyboard(false, 15)
	found15 := false
	foundEnable := false
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.Text == "15 мин ✓" && btn.CallbackData != nil && *btn.CallbackData == "notif_15" {
				found15 = true
			}
			if btn.CallbackData != nil && *btn.CallbackData == "notif_toggle" {
				if contains(btn.Text, "Включить") {
					foundEnable = true
				}
			}
		}
	}
	if !found15 {
		t.Error("missing 15 мин ✓ button with notif_15 callback")
	}
	if !foundEnable {
		t.Error("should show Включить when disabled")
	}

	kb = notifSettingsKeyboard(true, 5)
	foundDisable := false
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData != nil && *btn.CallbackData == "notif_toggle" {
				if contains(btn.Text, "Выключить") {
					foundDisable = true
				}
			}
		}
	}
	if !foundDisable {
		t.Error("should show Выключить when enabled")
	}
}

func TestMainKeyboardLayout(t *testing.T) {
	hasCallback := func(kb *tgbotapi.InlineKeyboardMarkup, data string) bool {
		for _, row := range kb.InlineKeyboard {
			for _, btn := range row {
				if btn.CallbackData != nil && *btn.CallbackData == data {
					return true
				}
			}
		}
		return false
	}

	kb := mainKeyboard(false, false)
	if !hasCallback(kb, "check") {
		t.Error("should see check button")
	}
	if !hasCallback(kb, "set_code") {
		t.Error("should see set_code button")
	}
	if hasCallback(kb, "notif_settings") {
		t.Error("non-registered user should not see notif_settings")
	}

	kb = mainKeyboard(false, true)
	if !hasCallback(kb, "notif_settings") {
		t.Error("registered user should see notif_settings")
	}
	if !hasCallback(kb, "disable") {
		t.Error("registered user should see disable button")
	}

	kb = mainKeyboard(true, true)
	if !hasCallback(kb, "admin_status") {
		t.Error("admin should see admin_status")
	}
	if !hasCallback(kb, "admin_users") {
		t.Error("admin should see admin_users")
	}
	if !hasCallback(kb, "admin_broadcast") {
		t.Error("admin should see admin_broadcast")
	}
}

func TestUpdateUserResultsOrdered(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStorage(dir)
	s.SaveUser(800, "1234-5678-9012", "")

	ordered := NewOrderedResults()
	ordered.Add("Рус", "01.06", "20", "4 ХОРОШО")
	ordered.Add("Мат", "02.06", "23", "5 ЗАЧЕТ")

	changed := s.UpdateUserResultsOrdered(800, ordered)
	if len(changed) != 2 {
		t.Errorf("first update: expected 2 changed, got %d", len(changed))
	}

	order := s.GetUserOrder(800)
	if len(order) != 2 || order[0] != "Рус" || order[1] != "Мат" {
		t.Errorf("order wrong: %v", order)
	}

	lastUpdated := s.GetLastUpdated(800)
	if lastUpdated.IsZero() {
		t.Error("lastUpdated should be set")
	}

	ordered2 := NewOrderedResults()
	ordered2.Add("Рус", "01.06", "20", "4 ХОРОШО")
	ordered2.Add("Мат", "02.06", "25", "5 ЗАЧЕТ")

	changed = s.UpdateUserResultsOrdered(800, ordered2)
	if len(changed) != 1 || changed[0] != "Мат" {
		t.Errorf("second update: expected [Мат] changed, got %v", changed)
	}
}

func TestLoadUsersSortedWithIDs(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStorage(dir)

	s.SaveUser(1, "1111-1111-1111", "alice")
	time.Sleep(10 * time.Millisecond)
	s.SaveUser(2, "2222-2222-2222", "bob")

	users := s.LoadUsersSortedWithIDs()
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	if users[0].Entry.Username != "alice" {
		t.Errorf("first user should be alice, got %s", users[0].Entry.Username)
	}
	if users[1].Entry.Username != "bob" {
		t.Errorf("second user should be bob, got %s", users[1].Entry.Username)
	}
	if users[0].ID != 1 {
		t.Errorf("first user ID should be 1, got %d", users[0].ID)
	}
}

func TestErrorsEnabled(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStorage(dir)
	s.SaveUser(900, "1234-5678-9012", "")

	if s.GetErrorsEnabled(900) {
		t.Error("default errors should be disabled")
	}

	s.SetErrorsEnabled(900, true)
	if !s.GetErrorsEnabled(900) {
		t.Error("errors should be enabled after SetErrorsEnabled(true)")
	}

	s.SetErrorsEnabled(900, false)
	if s.GetErrorsEnabled(900) {
		t.Error("errors should be disabled after SetErrorsEnabled(false)")
	}
}

func TestGetIntervalAllOptions(t *testing.T) {
	tests := []struct {
		interval int
		want     int
	}{
		{0, 5},
		{-1, 5},
		{5, 5},
		{10, 10},
		{15, 15},
	}
	for _, tt := range tests {
		e := UserEntry{Interval: tt.interval}
		got := e.GetInterval()
		if got != tt.want {
			t.Errorf("GetInterval(%d) = %d, want %d", tt.interval, got, tt.want)
		}
	}
}

func TestFormatOrderedResultsWithTimestamp(t *testing.T) {
	subjects := []SubjectResult{
		{Name: "Математика", Date: "02.06.2026", Score: "23", Grade: "5 ЗАЧЕТ"},
	}
	ts := time.Date(2026, 6, 19, 14, 30, 0, 0, time.Local)
	got := FormatOrderedResults(subjects, ts)
	if !contains(got, "Обновлено") {
		t.Error("missing Обновлено header")
	}
	if !contains(got, "Математика") {
		t.Error("missing subject name")
	}
	if !contains(got, "14:30:00") {
		t.Error("missing timestamp")
	}
}

func TestFormatOrderedResultsZeroTime(t *testing.T) {
	subjects := []SubjectResult{
		{Name: "Русский", Date: "01.06.2026", Score: "18", Grade: "4 ХОРОШО"},
	}
	got := FormatOrderedResults(subjects, time.Time{})
	if contains(got, "Обновлено") {
		t.Error("should not show Обновлено for zero time")
	}
	if !contains(got, "Русский") {
		t.Error("missing subject name")
	}
}

func TestParseOrderedResults(t *testing.T) {
	html := `<html><body>
		<div class="panel panel-default">
			<div class="subject-name"><a>Русский язык</a></div>
			<div class="subject-date">01.06.2026</div>
			<div class="primary">Первичный балл: 20 (67%)</div>
			<div class="mark4">Оценка:4ХОРОШО</div>
		</div>
		<div class="panel panel-default">
			<div class="subject-name"><a>Математика</a></div>
			<div class="subject-date">02.06.2026</div>
			<div class="primary">Первичный балл: 23 (74%)</div>
			<div class="mark5">Оценка:5ЗАЧЕТ</div>
		</div>
	</body></html>`

	results, err := ParseOrderedResults(html)
	if err != nil {
		t.Fatal(err)
	}
	if results.Len() != 2 {
		t.Fatalf("expected 2 results, got %d", results.Len())
	}

	subjects := results.Subjects
	if subjects[0].Name != "Русский язык" {
		t.Errorf("first subject = %q, want Русский язык", subjects[0].Name)
	}
	if subjects[1].Name != "Математика" {
		t.Errorf("second subject = %q, want Математика", subjects[1].Name)
	}
	if subjects[0].Grade != "4 ХОРОШО" {
		t.Errorf("first grade = %q, want 4 ХОРОШО", subjects[0].Grade)
	}
	if subjects[1].Grade != "5 ЗАЧЕТ" {
		t.Errorf("second grade = %q, want 5 ЗАЧЕТ", subjects[1].Grade)
	}

	r, ok := results.Get("Математика")
	if !ok {
		t.Error("Get Математика not found")
	}
	if r.Score != "Первичный балл: 23 (74%)" {
		t.Errorf("score = %q", r.Score)
	}
}

func TestParseOrderedResultsEmpty(t *testing.T) {
	results, err := ParseOrderedResults("<html><body></body></html>")
	if err != nil {
		t.Fatal(err)
	}
	if results.Len() != 0 {
		t.Errorf("expected 0 results, got %d", results.Len())
	}
}

func TestOrderedResultsToMap(t *testing.T) {
	ordered := NewOrderedResults()
	ordered.Add("Мат", "01.06", "23", "5")
	ordered.Add("Рус", "02.06", "18", "4")

	m := ordered.ToMap()
	if len(m) != 2 {
		t.Fatalf("ToMap: expected 2 entries, got %d", len(m))
	}
	if m["Мат"].Score != "23" {
		t.Errorf("Мат score = %q", m["Мат"].Score)
	}
}

func TestOrderedResultsSubjectsSlice(t *testing.T) {
	ordered := NewOrderedResults()
	ordered.Add("А", "01", "1", "1")
	ordered.Add("Б", "02", "2", "2")
	ordered.Add("В", "03", "3", "3")

	slice := ordered.SubjectsSlice()
	if len(slice) != 3 || slice[0] != "А" || slice[1] != "Б" || slice[2] != "В" {
		t.Errorf("SubjectsSlice = %v", slice)
	}
}

func TestSiteCacheSetGet(t *testing.T) {
	cache := &siteCache{
		entries: make(map[string]*siteCacheEntry),
		ttl:     2 * time.Minute,
	}

	if got := cache.get("test"); got != nil {
		t.Error("expected nil for missing cache entry")
	}

	ordered := NewOrderedResults()
	ordered.Add("Мат", "01.06", "23", "5 ЗАЧЕТ")
	cache.set("test", ordered)

	got := cache.get("test")
	if got == nil {
		t.Fatal("expected cached result, got nil")
	}
	if got.Len() != 1 {
		t.Errorf("expected 1 result, got %d", got.Len())
	}
}

func TestSiteCacheExpiry(t *testing.T) {
	cache := &siteCache{
		entries: make(map[string]*siteCacheEntry),
		ttl:     1 * time.Millisecond,
	}

	ordered := NewOrderedResults()
	ordered.Add("Мат", "01.06", "23", "5")
	cache.set("test", ordered)

	time.Sleep(5 * time.Millisecond)

	if got := cache.get("test"); got != nil {
		t.Error("expected nil for expired cache entry")
	}
}

func TestSiteCacheOverwrite(t *testing.T) {
	cache := &siteCache{
		entries: make(map[string]*siteCacheEntry),
		ttl:     2 * time.Minute,
	}

	ordered1 := NewOrderedResults()
	ordered1.Add("Мат", "01.06", "20", "4")
	cache.set("test", ordered1)

	ordered2 := NewOrderedResults()
	ordered2.Add("Мат", "01.06", "23", "5")
	ordered2.Add("Рус", "02.06", "18", "4")
	cache.set("test", ordered2)

	got := cache.get("test")
	if got == nil {
		t.Fatal("expected cached result")
	}
	if got.Len() != 2 {
		t.Errorf("expected 2 results after overwrite, got %d", got.Len())
	}
}

func TestStatsErrorTracking(t *testing.T) {
	s := &Stats{Uptime: time.Now()}

	if s.ErrorsToday() != 0 {
		t.Error("expected 0 errors initially")
	}

	s.RecordError()
	s.RecordError()
	if s.ErrorsToday() != 2 {
		t.Errorf("expected 2 errors, got %d", s.ErrorsToday())
	}
}

func TestStatsErrorReset(t *testing.T) {
	s := &Stats{Uptime: time.Now()}
	s.errorDate = "2020-01-01"
	s.errorsToday = 999

	if s.ErrorsToday() != 0 {
		t.Error("expected 0 errors after date reset")
	}
}
