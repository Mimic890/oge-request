package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const resultsURL = "https://ege-kostroma.ru/results-z.php"

var httpClient = &http.Client{Timeout: 15 * time.Second}

var headers = map[string]string{
	"User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0 Safari/537.36",
}

type Result struct {
	Date  string `json:"date"`
	Score string `json:"score"`
	Grade string `json:"grade"`
}

func FetchResults(code string) (map[string]Result, error) {
	body := fmt.Sprintf("code=%s&year=26", code)
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
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	stats.RecordVisit()
	stats.AddBytes(int64(len(data)) + int64(len(body)))

	return parseResults(string(data))
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
		grade := strings.TrimSpace(s.Find("[class^='mark']").Text())
		results[subj] = Result{Date: date, Score: score, Grade: grade}
	})

	return results, nil
}

func FormatResults(results map[string]Result) string {
	var sb strings.Builder
	sb.WriteString("<b>Результаты ОГЭ:</b>\n\n")
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

func DiffResults(old, cur map[string]Result) []string {
	var changed []string
	for subj, r := range cur {
		if oldR, ok := old[subj]; !ok || oldR != r {
			changed = append(changed, subj)
		}
	}
	return changed
}
