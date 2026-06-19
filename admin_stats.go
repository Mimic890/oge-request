package main

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

type Stats struct {
	mu          sync.Mutex
	SiteVisits  int64
	BytesTotal  int64
	Uptime      time.Time
	visitDate   string
	visitsToday int64
	errorDate   string
	errorsToday int64
}

var stats = &Stats{Uptime: time.Now()}

func (s *Stats) RecordVisit() {
	today := time.Now().Format("2006-01-02")
	s.mu.Lock()
	if s.visitDate != today {
		s.visitDate = today
		s.visitsToday = 0
	}
	s.visitsToday++
	s.mu.Unlock()
	atomic.AddInt64(&s.SiteVisits, 1)
}

func (s *Stats) RecordError() {
	today := time.Now().Format("2006-01-02")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.errorDate != today {
		s.errorDate = today
		s.errorsToday = 0
	}
	s.errorsToday++
}

func (s *Stats) ErrorsToday() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	today := time.Now().Format("2006-01-02")
	if s.errorDate != today {
		return 0
	}
	return s.errorsToday
}

func (s *Stats) AddBytes(n int64) {
	atomic.AddInt64(&s.BytesTotal, n)
}

func (s *Stats) SiteVisitsToday() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	today := time.Now().Format("2006-01-02")
	if s.visitDate != today {
		return 0
	}
	return s.visitsToday
}

func (s *Stats) TotalSiteVisits() int64 {
	return atomic.LoadInt64(&s.SiteVisits)
}

func (s *Stats) TotalBytes() int64 {
	return atomic.LoadInt64(&s.BytesTotal)
}

func (s *Stats) UptimeDuration() time.Duration {
	return time.Since(s.Uptime)
}

func GetRAM() (alloc, sys uint64) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc, m.Sys
}
