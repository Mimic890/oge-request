package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var resultsURL string
var siteDomain string

func InitSiteURL(domain string) {
	siteDomain = domain
	resultsURL = "https://" + domain + "/results-z.php"
}

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		Proxy:                 nil,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   5,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:  5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
	},
}

type siteCacheEntry struct {
	results *OrderedResults
	expires time.Time
}

type siteCache struct {
	mu      sync.Mutex
	entries map[string]*siteCacheEntry
	ttl     time.Duration
}

var siteCacheStore = &siteCache{
	entries: make(map[string]*siteCacheEntry),
	ttl:     2 * time.Minute,
}

var fetchMu sync.Mutex
var fetchCodeMu = make(map[string]*sync.Mutex)

func fetchLock(code string) *sync.Mutex {
	fetchMu.Lock()
	defer fetchMu.Unlock()
	m, ok := fetchCodeMu[code]
	if !ok {
		m = &sync.Mutex{}
		fetchCodeMu[code] = m
	}
	return m
}

func (c *siteCache) get(code string) *OrderedResults {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[code]
	if !ok {
		return nil
	}
	if time.Now().After(e.expires) {
		delete(c.entries, code)
		return nil
	}
	return e.results
}

func (c *siteCache) set(code string, results *OrderedResults) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[code] = &siteCacheEntry{
		results: results,
		expires: time.Now().Add(c.ttl),
	}
}

var siteLimiter *RateLimiter

func InitRateLimiter(rps int) {
	siteLimiter = NewRateLimiter(rps)
}

var headers = map[string]string{
	"User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0 Safari/537.36",
}

type Result struct {
	Date  string `json:"date"`
	Score string `json:"score"`
	Grade string `json:"grade"`
}

func fetchOnceOrdered(code string) (*OrderedResults, error) {
	if siteLimiter != nil {
		siteLimiter.Wait()
	}
	body := fmt.Sprintf("code=%s&year=%s", code, time.Now().Format("06"))
	req, err := http.NewRequest("POST", resultsURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}

	stats.RecordVisit()
	stats.AddBytes(int64(len(data)) + int64(len(body)))

	return ParseOrderedResults(string(data))
}

func FetchResultsFresh(code string) (*OrderedResults, error) {
	m := fetchLock(code)
	m.Lock()
	defer m.Unlock()

	if cached := siteCacheStore.get(code); cached != nil {
		return cached, nil
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}
		ordered, err := fetchOnceOrdered(code)
		if err != nil {
			lastErr = err
			continue
		}
		siteCacheStore.set(code, ordered)
		return ordered, nil
	}
	return nil, lastErr
}

func FetchResultsOnceOrdered(code string) (*OrderedResults, error) {
	if cached := siteCacheStore.get(code); cached != nil {
		return cached, nil
	}

	m := fetchLock(code)
	m.Lock()
	defer m.Unlock()

	if cached := siteCacheStore.get(code); cached != nil {
		return cached, nil
	}

	ordered, err := fetchOnceOrdered(code)
	if err != nil {
		return nil, err
	}
	siteCacheStore.set(code, ordered)
	return ordered, nil
}

func FetchResultsOrdered(code string) (*OrderedResults, error) {
	return FetchResultsFresh(code)
}

type SubjectResult struct {
	Name string
	Date string
	Score string
	Grade string
}

type OrderedResults struct {
	Subjects []SubjectResult
	byName   map[string]SubjectResult
}

func NewOrderedResults() *OrderedResults {
	return &OrderedResults{
		byName: make(map[string]SubjectResult),
	}
}

func (o *OrderedResults) Add(name, date, score, grade string) {
	sr := SubjectResult{Name: name, Date: date, Score: score, Grade: grade}
	o.byName[name] = sr
	o.Subjects = append(o.Subjects, sr)
}

func (o *OrderedResults) Get(name string) (SubjectResult, bool) {
	sr, ok := o.byName[name]
	return sr, ok
}

func (o *OrderedResults) Len() int {
	return len(o.Subjects)
}

func (o *OrderedResults) ToMap() map[string]Result {
	out := make(map[string]Result, len(o.Subjects))
	for _, sr := range o.Subjects {
		out[sr.Name] = Result{Date: sr.Date, Score: sr.Score, Grade: sr.Grade}
	}
	return out
}

func (o *OrderedResults) SubjectsSlice() []string {
	out := make([]string, len(o.Subjects))
	for i, sr := range o.Subjects {
		out[i] = sr.Name
	}
	return out
}

func ParseOrderedResults(html string) (*OrderedResults, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	results := NewOrderedResults()

	doc.Find(".panel.panel-default").Each(func(_ int, s *goquery.Selection) {
		nameEl := s.Find(".subject-name a")
		if nameEl.Length() == 0 {
			return
		}
		subj := strings.TrimSpace(nameEl.Text())
		date := strings.TrimSpace(s.Find(".subject-date").Text())
		score := strings.TrimSpace(s.Find(".primary").Text())
		gradeRaw := strings.TrimSpace(s.Find("[class^='mark']").Text())
		gradeRaw = strings.TrimPrefix(gradeRaw, "Оценка:")
		gradeRaw = strings.TrimSpace(gradeRaw)
		grade := fixGradeSpacing(gradeRaw)
		results.Add(subj, date, score, grade)
	})

	return results, nil
}

func parseResults(html string) (map[string]Result, error) {
	ordered, err := ParseOrderedResults(html)
	if err != nil {
		return nil, err
	}
	return ordered.ToMap(), nil
}

func FormatResults(results map[string]Result) string {
	var sb strings.Builder
	sb.WriteString(msgResultsHeader())
	for subj, r := range results {
		sb.WriteString(fmt.Sprintf("<b>%s</b>\n", subj))
		if r.Date != "" {
			sb.WriteString(fmt.Sprintf("  Дата: %s\n", r.Date))
		}
		if r.Score != "" {
			sb.WriteString(fmt.Sprintf("  Балл: %s\n", r.Score))
		}
		if r.Grade != "" {
			sb.WriteString(fmt.Sprintf("  Оценка: %s\n", r.Grade))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func FormatOrderedResults(subjects []SubjectResult, lastUpdated time.Time) string {
	var sb strings.Builder
	sb.WriteString(msgResultsHeader())
	if !lastUpdated.IsZero() {
		months := [...]string{"января", "февраля", "марта", "апреля", "мая", "июня",
			"июля", "августа", "сентября", "октября", "ноября", "декабря"}
		m := lastUpdated.Month() - 1
		sb.WriteString(fmt.Sprintf("🔄 Обновлено: %d %s %s\n\n",
			lastUpdated.Day(), months[m], lastUpdated.Format("15:04:05")))
	}
	for _, sr := range subjects {
		sb.WriteString(fmt.Sprintf("<b>%s</b>\n", sr.Name))
		if sr.Date != "" {
			sb.WriteString(fmt.Sprintf("  Дата: %s\n", sr.Date))
		}
		if sr.Score != "" {
			sb.WriteString(fmt.Sprintf("  Балл: %s\n", sr.Score))
		}
		if sr.Grade != "" {
			sb.WriteString(fmt.Sprintf("  Оценка: %s\n", sr.Grade))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func fixGradeSpacing(grade string) string {
	if len(grade) == 0 {
		return grade
	}
	runes := []rune(grade)
	for i, ch := range runes {
		if isLetter(ch) {
			if i > 0 && runes[i-1] != ' ' {
				result := make([]rune, 0, len(runes)+1)
				result = append(result, runes[:i]...)
				result = append(result, ' ')
				result = append(result, runes[i:]...)
				return string(result)
			}
			break
		}
	}
	return grade
}

func isLetter(ch rune) bool {
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') ||
		(ch >= 'А' && ch <= 'Я') || (ch >= 'а' && ch <= 'я') || ch == 'ё' || ch == 'Ё'
}

func DiffResults(old, cur map[string]Result) []string {
	var changed []string
	for subj, r := range cur {
		if oldR, ok := old[subj]; !ok || oldR != r {
			changed = append(changed, subj)
		}
	}
	return changed
}
