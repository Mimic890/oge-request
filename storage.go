package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Storage struct {
	dir    string
	mu     sync.Mutex
	users  map[int64]UserEntry
	state  map[int64]map[string]Result
	order  map[int64][]string
	timing map[int64]time.Time
}

type UserEntry struct {
	Code          string        `json:"code"`
	Username      string        `json:"username"`
	Enabled       bool          `json:"enabled"`
	ErrorsEnabled bool          `json:"errors_enabled"`
	Interval      int           `json:"interval"`
	ErrorCount    int64         `json:"error_count"`
	LastActive    time.Time     `json:"last_active"`
	LastCheck     time.Time     `json:"last_check"`
	WarningSent   bool          `json:"warning_sent"`
	FarewellSent  bool          `json:"farewell_sent"`
	CreatedAt     time.Time     `json:"created_at"`
}

func (e UserEntry) GetInterval() int {
	if e.Interval <= 0 {
		return 5
	}
	return e.Interval
}

func NewStorage(dir string) (*Storage, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	s := &Storage{dir: dir}
	s.users = s.readUsersFile()
	s.state = s.readStateFile()
	s.order = s.readOrderFile()
	s.timing = s.readTimingFile()
	migrated := false
	for uid := range s.users {
		entry := s.users[uid]
		if entry.Interval <= 0 {
			entry.Interval = 5
			s.users[uid] = entry
			migrated = true
		}
		if entry.CreatedAt.IsZero() {
			entry.CreatedAt = entry.LastActive
			if entry.CreatedAt.IsZero() {
				entry.CreatedAt = time.Now()
			}
			s.users[uid] = entry
			migrated = true
		}
	}
	if migrated || len(s.users) > 0 {
		s.writeUsersFile()
	}
	return s, nil
}

func (s *Storage) SaveUser(uid int64, code string, username string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.users[uid]
	if !exists {
		entry = UserEntry{Enabled: true, Interval: 5, CreatedAt: time.Now()}
	}
	entry.Code = code
	entry.ErrorCount = 0
	if username != "" {
		entry.Username = username
	}
	entry.LastActive = time.Now()
	entry.WarningSent = false
	entry.FarewellSent = false
	s.users[uid] = entry
	s.writeUsersFile()
}

func (s *Storage) RemoveUser(uid int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.users, uid)
	delete(s.state, uid)
	delete(s.order, uid)
	delete(s.timing, uid)
	s.writeUsersFile()
	s.writeStateFile()
	s.writeOrderFile()
	s.writeTimingFile()
}

func (s *Storage) UpdateLastActive(uid int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.users[uid]; ok {
		entry.LastActive = time.Now()
		entry.WarningSent = false
		entry.FarewellSent = false
		s.users[uid] = entry
		s.writeUsersFile()
	}
}

func (s *Storage) UpdateUsername(uid int64, username string) {
	if username == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.users[uid]; ok {
		if entry.Username != username {
			entry.Username = username
			s.users[uid] = entry
			s.writeUsersFile()
		}
	}
}

func (s *Storage) UpdateLastCheck(uid int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.users[uid]; ok {
		entry.LastCheck = time.Now()
		s.users[uid] = entry
		s.writeUsersFile()
	}
}

func (s *Storage) SetWarningSent(uid int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.users[uid]; ok {
		entry.WarningSent = true
		s.users[uid] = entry
		s.writeUsersFile()
	}
}

func (s *Storage) SetFarewellSent(uid int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.users[uid]; ok {
		entry.FarewellSent = true
		s.users[uid] = entry
		s.writeUsersFile()
	}
}

func (s *Storage) IncrementErrorCount(uid int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.users[uid]; ok {
		entry.ErrorCount++
		s.users[uid] = entry
		s.writeUsersFile()
	}
}

func (s *Storage) ResetErrorCount(uid int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.users[uid]; ok {
		if entry.ErrorCount > 0 {
			entry.ErrorCount = 0
			s.users[uid] = entry
			s.writeUsersFile()
		}
	}
}

func (s *Storage) SetNotifEnabled(uid int64, enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.users[uid]; ok {
		entry.Enabled = enabled
		s.users[uid] = entry
		s.writeUsersFile()
	}
}

func (s *Storage) SetCheckInterval(uid int64, interval int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.users[uid]; ok {
		entry.Interval = interval
		s.users[uid] = entry
		s.writeUsersFile()
	}
}

func (s *Storage) GetNotifEnabled(uid int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.users[uid]; ok {
		return entry.Enabled
	}
	return true
}

func (s *Storage) GetCheckInterval(uid int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.users[uid]; ok {
		return entry.GetInterval()
	}
	return 5
}

func (s *Storage) GetErrorsEnabled(uid int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.users[uid]; ok {
		return entry.ErrorsEnabled
	}
	return true
}

func (s *Storage) SetErrorsEnabled(uid int64, enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.users[uid]; ok {
		entry.ErrorsEnabled = enabled
		s.users[uid] = entry
		s.writeUsersFile()
	}
}

func (s *Storage) NeedsCheck(uid int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.users[uid]
	if !ok {
		return false
	}
	if entry.LastCheck.IsZero() {
		return true
	}
	return time.Since(entry.LastCheck) >= time.Duration(entry.GetInterval())*time.Minute
}

func (s *Storage) LoadUsers() map[int64]UserEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[int64]UserEntry, len(s.users))
	for k, v := range s.users {
		out[k] = v
	}
	return out
}

func (s *Storage) LoadUsersSortedWithIDs() []struct {
	ID    int64
	Entry UserEntry
} {
	s.mu.Lock()
	defer s.mu.Unlock()
	type pair struct {
		ID    int64
		Entry UserEntry
	}
	out := make([]pair, 0, len(s.users))
	for k, v := range s.users {
		out = append(out, pair{ID: k, Entry: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Entry.CreatedAt.Equal(out[j].Entry.CreatedAt) {
			return out[i].Entry.Code < out[j].Entry.Code
		}
		return out[i].Entry.CreatedAt.Before(out[j].Entry.CreatedAt)
	})
	result := make([]struct {
		ID    int64
		Entry UserEntry
	}, len(out))
	for i, p := range out {
		result[i] = struct {
			ID    int64
			Entry UserEntry
		}{ID: p.ID, Entry: p.Entry}
	}
	return result
}

func (s *Storage) GetUserResults(uid int64) map[string]Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.state[uid]
	out := make(map[string]Result, len(r))
	for k, v := range r {
		out[k] = v
	}
	return out
}

func (s *Storage) UpdateUserResults(uid int64, results map[string]Result) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.state[uid]
	changed := DiffResults(old, results)
	s.state[uid] = results
	s.writeStateFile()
	return changed
}

func (s *Storage) UpdateUserResultsOrdered(uid int64, ordered *OrderedResults) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	results := ordered.ToMap()
	old := s.state[uid]
	changed := DiffResults(old, results)
	s.state[uid] = results
	s.order[uid] = ordered.SubjectsSlice()
	s.timing[uid] = time.Now()
	s.writeStateFile()
	s.writeOrderFile()
	s.writeTimingFile()
	return changed
}

func (s *Storage) GetUserOrder(uid int64) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.order[uid]
}

func (s *Storage) GetLastUpdated(uid int64) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.timing[uid]
}

func (s *Storage) SetLastUpdated(uid int64, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timing[uid] = t
	s.writeTimingFile()
}

func (s *Storage) GetUserResultsOrdered(uid int64) []SubjectResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	results := s.state[uid]
	order := s.order[uid]
	if len(order) == 0 {
		out := make([]SubjectResult, 0, len(results))
		for name, r := range results {
			out = append(out, SubjectResult{Name: name, Date: r.Date, Score: r.Score, Grade: r.Grade})
		}
		return out
	}
	out := make([]SubjectResult, 0, len(order))
	for _, name := range order {
		if r, ok := results[name]; ok {
			out = append(out, SubjectResult{Name: name, Date: r.Date, Score: r.Score, Grade: r.Grade})
		}
	}
	for name, r := range results {
		found := false
		for _, n := range order {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			out = append(out, SubjectResult{Name: name, Date: r.Date, Score: r.Score, Grade: r.Grade})
		}
	}
	return out
}

func (s *Storage) TotalUsers() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.users)
}

func (s *Storage) ActiveUsers() (total, active int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	total = len(s.users)
	for uid := range s.users {
		if _, ok := s.state[uid]; ok {
			active++
		}
	}
	return
}

func (s *Storage) readUsersFile() map[int64]UserEntry {
	path := filepath.Join(s.dir, "users.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return make(map[int64]UserEntry)
	}
	var users map[int64]UserEntry
	if err := json.Unmarshal(data, &users); err != nil {
		log.Printf("users.json parse error: %v", err)
		return make(map[int64]UserEntry)
	}
	return users
}

func (s *Storage) writeUsersFile() {
	path := filepath.Join(s.dir, "users.json")
	data, err := json.MarshalIndent(s.users, "", "  ")
	if err != nil {
		log.Printf("users.json marshal error: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		log.Printf("users.json write error: %v", err)
	}
}

func (s *Storage) readStateFile() map[int64]map[string]Result {
	path := filepath.Join(s.dir, "state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return make(map[int64]map[string]Result)
	}
	var state map[int64]map[string]Result
	if err := json.Unmarshal(data, &state); err != nil {
		log.Printf("state.json parse error: %v", err)
		return make(map[int64]map[string]Result)
	}
	return state
}

func (s *Storage) writeStateFile() {
	path := filepath.Join(s.dir, "state.json")
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		log.Printf("state.json marshal error: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		log.Printf("state.json write error: %v", err)
	}
}

func (s *Storage) readOrderFile() map[int64][]string {
	path := filepath.Join(s.dir, "order.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return make(map[int64][]string)
	}
	var order map[int64][]string
	if err := json.Unmarshal(data, &order); err != nil {
		log.Printf("order.json parse error: %v", err)
		return make(map[int64][]string)
	}
	return order
}

func (s *Storage) writeOrderFile() {
	path := filepath.Join(s.dir, "order.json")
	data, err := json.MarshalIndent(s.order, "", "  ")
	if err != nil {
		log.Printf("order.json marshal error: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		log.Printf("order.json write error: %v", err)
	}
}

func (s *Storage) readTimingFile() map[int64]time.Time {
	path := filepath.Join(s.dir, "timing.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return make(map[int64]time.Time)
	}
	var timing map[int64]time.Time
	if err := json.Unmarshal(data, &timing); err != nil {
		log.Printf("timing.json parse error: %v", err)
		return make(map[int64]time.Time)
	}
	return timing
}

func (s *Storage) writeTimingFile() {
	path := filepath.Join(s.dir, "timing.json")
	data, err := json.MarshalIndent(s.timing, "", "  ")
	if err != nil {
		log.Printf("timing.json marshal error: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		log.Printf("timing.json write error: %v", err)
	}
}
