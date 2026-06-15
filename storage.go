package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Storage struct {
	dir string
	mu  sync.RWMutex
}

type UserEntry struct {
	Code string `json:"code"`
}

func NewStorage(dir string) (*Storage, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &Storage{dir: dir}, nil
}

func (s *Storage) SaveUser(uid int64, code string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	users := s.readUsersFile()
	users[uid] = UserEntry{Code: code}
	s.writeUsersFile(users)
}

func (s *Storage) RemoveUser(uid int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	users := s.readUsersFile()
	delete(users, uid)
	s.writeUsersFile(users)
}

func (s *Storage) LoadUsers() (map[int64]UserEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.readUsersFile(), nil
}

func (s *Storage) LoadState() (map[int64]map[string]Result, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.readStateFile(), nil
}

func (s *Storage) SaveState(state map[int64]map[string]Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writeStateFile(state)
}

func (s *Storage) RemoveState(uid int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.readStateFile()
	delete(state, uid)
	s.writeStateFile(state)
}

func (s *Storage) readUsersFile() map[int64]UserEntry {
	path := filepath.Join(s.dir, "users.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return make(map[int64]UserEntry)
	}
	var users map[int64]UserEntry
	if err := json.Unmarshal(data, &users); err != nil {
		return make(map[int64]UserEntry)
	}
	return users
}

func (s *Storage) writeUsersFile(users map[int64]UserEntry) {
	path := filepath.Join(s.dir, "users.json")
	data, _ := json.MarshalIndent(users, "", "  ")
	os.WriteFile(path, data, 0600)
}

func (s *Storage) readStateFile() map[int64]map[string]Result {
	path := filepath.Join(s.dir, "state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return make(map[int64]map[string]Result)
	}
	var state map[int64]map[string]Result
	if err := json.Unmarshal(data, &state); err != nil {
		return make(map[int64]map[string]Result)
	}
	return state
}

func (s *Storage) writeStateFile(state map[int64]map[string]Result) {
	path := filepath.Join(s.dir, "state.json")
	data, _ := json.MarshalIndent(state, "", "  ")
	os.WriteFile(path, data, 0600)
}
