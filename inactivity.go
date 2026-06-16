package main

import (
	"log"
	"time"
)

const (
	inactivityWarningDays = 54
	inactivityDeleteDays  = 61
)

func RunInactivityManager(store *Storage, bot *Bot) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		cleanupInactive(store, bot)
	}
}

func cleanupInactive(store *Storage, bot *Bot) {
	users := store.LoadUsers()
	now := time.Now()

	for uid, entry := range users {
		if uid == bot.cfg.AdminID {
			continue
		}
		if entry.LastActive.IsZero() {
			continue
		}
		daysInactive := int(now.Sub(entry.LastActive).Hours() / 24)

		switch {
		case daysInactive >= inactivityDeleteDays:
			log.Printf("deleting inactive user %d (%d days)", uid, daysInactive)
			store.RemoveUser(uid)
			store.RemoveState(uid)
		case daysInactive >= inactivityWarningDays && !entry.WarningSent:
			store.SetWarningSent(uid)
			bot.send(uid, msgInactivityWarning(daysInactive), nil)
		}
	}
}
