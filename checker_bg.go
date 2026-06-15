package main

import (
	"log"
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
	users, _ := store.LoadUsers()
	if len(users) == 0 {
		return
	}
	state, _ := store.LoadState()

	for uid, entry := range users {
		results, err := FetchResults(entry.Code)
		if err != nil {
			log.Printf("fetch error uid=%d: %v", uid, err)
			continue
		}
		if len(results) == 0 {
			continue
		}

		old := state[uid]
		changed := DiffResults(old, results)
		if len(changed) > 0 {
			text := "<b>Обновление результатов!</b>\n\n"
			for _, subj := range changed {
				r := results[subj]
				text += "<b>" + subj + "</b>\n"
				if r.Date != "" {
					text += "  Дата: " + r.Date + "\n"
				}
				if r.Score != "" {
					text += "  Балл: " + r.Score + "\n"
				}
				if r.Grade != "" {
					text += "  Оценка: " + r.Grade + "\n"
				}
				text += "\n"
			}
			bot.send(uid, text, nil)
		}

		state[uid] = results
	}
	store.SaveState(state)
}
