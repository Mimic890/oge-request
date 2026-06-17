package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
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
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		Proxy: nil,
	},
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

func FetchResults(code string) (map[string]Result, error) {
	return fetchResults(code, 3)
}

func FetchResultsOnce(code string) (map[string]Result, error) {
	return fetchResults(code, 1)
}

func fetchResults(code string, attempts int) (map[string]Result, error) {
	if siteLimiter != nil {
		siteLimiter.Wait()
	}
	body := fmt.Sprintf("code=%s&year=%s", code, time.Now().Format("06"))
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}
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
			lastErr = err
			continue
		}

		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		stats.RecordVisit()
		stats.AddBytes(int64(len(data)) + int64(len(body)))

		return parseResults(string(data))
	}
	return nil, lastErr
}

func parseResults(html string) (map[string]Result, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	results := make(map[string]Result)

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
		results[subj] = Result{Date: date, Score: score, Grade: grade}
	})

	return results, nil
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
