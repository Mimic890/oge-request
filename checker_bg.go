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
		ordered, err := FetchResultsOnceOrdered(entry.Code)
		if err != nil {
			log.Printf("check uid=%d error: %v", uid, err)
			continue
		}
		if ordered.Len() == 0 {
			continue
		}

		store.ResetErrors(uid)
		store.UpdateLastCheck(uid)
		changed := store.UpdateUserResultsOrdered(uid, ordered)
		if len(changed) > 0 {
			log.Printf("uid=%d: %d subjects changed: %v", uid, len(changed), changed)
			if entry.Enabled {
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
