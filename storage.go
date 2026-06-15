package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
)

type Storage struct {
	dir   string
	mu    sync.Mutex
	users map[int64]UserEntry
	state map[int64]map[string]Result
}

type UserEntry struct {
	Code string `json:"code"`
}

func NewStorage(dir string) (*Storage, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	s := &Storage{dir: dir}
	s.users = s.readUsersFile()
	s.state = s.readStateFile()
	return s, nil
}

func (s *Storage) SaveUser(uid int64, code string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[uid] = UserEntry{Code: code}
	s.writeUsersFile()
}

func (s *Storage) RemoveUser(uid int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.users, uid)
	s.writeUsersFile()
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

func (s *Storage) RemoveState(uid int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.state, uid)
	s.writeStateFile()
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
