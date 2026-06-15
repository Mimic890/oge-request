package main

import (
	"fmt"
	"log"
	"strings"
	"time"
)

func RunChecker(cfg Config, store *Storage, bot *Bot) {
	ticker := time.NewTicker(cfg.CheckInterval)
	defer ticker.Stop()
	for range ticker.C {
		checkAll(store, bot)
	}
}

func checkAll(store *Storage, bot *Bot) {
	users := store.LoadUsers()
	if len(users) == 0 {
		return
	}

	var errs []string
	for uid, entry := range users {
		results, err := FetchResults(entry.Code)
		if err != nil {
			errs = append(errs, fmt.Sprintf("uid=%d: %v", uid, err))
			continue
		}
		if len(results) == 0 {
			continue
		}

		changed := store.UpdateUserResults(uid, results)
		if len(changed) > 0 {
			var sb strings.Builder
			sb.WriteString("<b>Обновление результатов!</b>\n\n")
			for _, subj := range changed {
				r := results[subj]
				sb.WriteString("<b>" + subj + "</b>\n")
				if r.Date != "" {
					sb.WriteString("  Дата: " + r.Date + "\n")
				}
				if r.Score != "" {
					sb.WriteString("  Балл: " + r.Score + "\n")
				}
				if r.Grade != "" {
					sb.WriteString("  Оценка: " + r.Grade + "\n")
				}
				sb.WriteString("\n")
			}
			bot.send(uid, sb.String(), nil)
		}
	}

	if len(errs) > 0 {
		log.Printf("check errors (%d/%d users): %s", len(errs), len(users), strings.Join(errs, "; "))
	}
}
