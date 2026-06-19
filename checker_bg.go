package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

type checkResult struct {
	uid     int64
	entry   UserEntry
	changed []string
	ordered *OrderedResults
	err     error
}

func RunChecker(store *Storage, bot *Bot, concurrency int) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		checkAll(store, bot, concurrency)
	}
}

func checkAll(store *Storage, bot *Bot, concurrency int) {
	users := store.LoadUsers()
	if len(users) == 0 {
		return
	}

	log.Printf("check cycle: %d users (concurrency=%d)", len(users), concurrency)

	results := make(chan checkResult, concurrency)
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)

	skipped := 0
	for uid, entry := range users {
		if !store.NeedsCheck(uid) {
			skipped++
			continue
		}

		wg.Add(1)
		go func(uid int64, entry UserEntry) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			ordered, err := FetchResultsOnceOrdered(entry.Code)
			if err != nil {
				stats.RecordError()
				store.IncrementErrorCount(uid)
				results <- checkResult{uid: uid, entry: entry, err: err}
				return
			}
			if ordered.Len() == 0 {
				results <- checkResult{uid: uid, entry: entry}
				return
			}

			store.UpdateLastCheck(uid)
			store.ResetErrorCount(uid)
			changed := store.UpdateUserResultsOrdered(uid, ordered)
			results <- checkResult{uid: uid, entry: entry, changed: changed, ordered: ordered}
		}(uid, entry)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	checked := 0
	for r := range results {
		if r.err != nil {
			log.Printf("[bg] uid=%d code=%s error: %v", r.uid, maskCode(r.entry.Code), r.err)
			continue
		}
		if r.ordered == nil {
			continue
		}
		checked++
		if len(r.changed) > 0 {
			log.Printf("[bg] uid=%d: %d subjects changed: %v", r.uid, len(r.changed), r.changed)
			if r.entry.Enabled {
				sendChangeNotification(bot, r.uid, r.ordered, r.changed)
			}
		}
	}

	if checked > 0 {
		log.Printf("[bg] checked %d users, skipped %d", checked, skipped)
	}
}

func sendChangeNotification(bot *Bot, uid int64, ordered *OrderedResults, changed []string) {
	var sb strings.Builder
	sb.WriteString(msgUpdateHeader())
	for _, sr := range ordered.Subjects {
		isChanged := false
		for _, c := range changed {
			if c == sr.Name {
				isChanged = true
				break
			}
		}
		if isChanged {
			sb.WriteString("<b>" + sr.Name + "</b>\n")
			if sr.Date != "" {
				sb.WriteString("  Дата: " + sr.Date + "\n")
			}
			if sr.Score != "" {
				sb.WriteString("  Балл: " + sr.Score + "\n")
			}
			if sr.Grade != "" {
				sb.WriteString("  Оценка: " + sr.Grade + "\n")
			}
			sb.WriteString("\n")
		}
	}
	bot.send(uid, sb.String(), nil)
}

func notifyAdminError(bot *Bot, uid int64, code string, username string, err error) {
	if !bot.store.GetErrorsEnabled(uid) {
		return
	}
	name := fmt.Sprintf("%d", uid)
	if username != "" {
		name = fmt.Sprintf("%d (@%s)", uid, username)
	}
	bot.sendSilent(bot.cfg.AdminID,
		fmt.Sprintf("⚠️ Ошибка проверки\n\n"+
			"Пользователь: %s\n"+
			"Код: %s\n"+
			"%v", name, maskCode(code), err))
}
