package main

import (
	"os"
	"path/filepath"
	"testing"
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
