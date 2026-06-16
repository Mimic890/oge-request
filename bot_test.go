package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

	s.SaveUser(12345, "3426-5251-3725")
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
	s1.SaveUser(111, "1111-1111-1111")
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
	s.SaveUser(999, "9999-9999-9999")
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

	s.SaveUser(1, "1111-1111-1111")
	s.SaveUser(2, "2222-2222-2222")
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
	s.SaveUser(100, "1234-5678-9012")

	users := s.LoadUsers()
	entry := users[100]
	if !entry.Enabled {
		t.Error("default Enabled should be true")
	}
	if entry.Interval != 15 {
		t.Errorf("default Interval = %d, want 15", entry.Interval)
	}
}

func TestGetInterval(t *testing.T) {
	tests := []struct {
		interval int
		want     int
	}{
		{0, 15},
		{-5, 15},
		{15, 15},
		{60, 60},
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
	s.SaveUser(200, "1234-5678-9012")

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
	s.SaveUser(300, "1234-5678-9012")

	if s.GetCheckInterval(300) != 15 {
		t.Error("default interval should be 15")
	}

	s.SetCheckInterval(300, 30)
	if s.GetCheckInterval(300) != 30 {
		t.Error("interval should be 30 after SetCheckInterval(30)")
	}
}

func TestNeedsCheck(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStorage(dir)
	s.SaveUser(400, "1234-5678-9012")

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
	s.SaveUser(500, "1234-5678-9012")
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
	s.SaveUser(600, "1234-5678-9012")

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
	s.SaveUser(700, "1234-5678-9012")

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

func TestSiteFailureTracker(t *testing.T) {
	tracker := &SiteFailureTracker{}
	if tracker.Failures != 0 {
		t.Error("initial Failures should be 0")
	}
	if tracker.NotifiedAdmin {
		t.Error("initial NotifiedAdmin should be false")
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
		{"msgNoSavedResults", msgNoSavedResults},
		{"msgSiteUnavailable", msgSiteUnavailable},
		{"msgCodeDeleted", msgCodeDeleted},
		{"msgDeleteConfirm", msgDeleteConfirm},
		{"msgResultsHeader", msgResultsHeader},
		{"msgUpdateHeader", msgUpdateHeader},
		{"msgInactivityFarewell", msgInactivityFarewell},
		{"msgSiteRecoveredAdmin", msgSiteRecoveredAdmin},
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

func TestMsgSiteDownAdmin(t *testing.T) {
	got := msgSiteDownAdmin(3)
	if !contains(got, "3") {
		t.Error("missing failure count")
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
