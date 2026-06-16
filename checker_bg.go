package main

import (
	"fmt"
	"log"
	"strings"
	"time"
)

func RunChecker(store *Storage, bot *Bot) {
	ticker := time.NewTicker(1 * time.Minute)
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

	log.Printf("check cycle: %d users", len(users))

	checked := 0
	for uid, entry := range users {
		if uid == bot.cfg.AdminID {
			continue
		}
		if !store.NeedsCheck(uid) {
			continue
		}

		checked++
		results, err := FetchResults(entry.Code)
		if err != nil {
			log.Printf("check uid=%d error: %v", uid, err)
			continue
		}
		if len(results) == 0 {
			continue
		}

		store.ResetErrors(uid)
		store.UpdateLastCheck(uid)
		changed := store.UpdateUserResults(uid, results)
		if len(changed) > 0 {
			log.Printf("uid=%d: %d subjects changed: %v", uid, len(changed), changed)
			if entry.Enabled {
				var sb strings.Builder
				sb.WriteString(msgUpdateHeader())
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
	}
	if checked > 0 {
		log.Printf("checked %d users", checked)
	}
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
