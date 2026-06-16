package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var codePattern = regexp.MustCompile(`^\d{4}-\d{4}-\d{4}$`)

type Bot struct {
	api           *tgbotapi.BotAPI
	cfg           Config
	store         *Storage
	siteFailures  *SiteFailureTracker
	mu            sync.Mutex
	waiting       map[int64]bool
}

func NewBot(cfg Config, store *Storage) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		return nil, fmt.Errorf("telegram: %w", err)
	}

	proxyURL := os.Getenv("HTTPS_PROXY")
	if proxyURL == "" {
		proxyURL = os.Getenv("HTTP_PROXY")
	}
	if proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err == nil {
			api.Client = &http.Client{
				Transport: &http.Transport{
					Proxy: http.ProxyURL(parsed),
				},
			}
			log.Printf("telegram proxy: %s", proxyURL)
		}
	}

	log.Printf("authorized as @%s", api.Self.UserName)
	b := &Bot{
		api:          api,
		cfg:          cfg,
		store:        store,
		siteFailures: &SiteFailureTracker{},
		waiting:      make(map[int64]bool),
	}

	b.setCommands()
	go b.poll()
	return b, nil
}

func (b *Bot) setCommands() {
	cmds := []tgbotapi.BotCommand{
		{Command: "start", Description: "Запуск и главное меню"},
		{Command: "repo", Description: "Ссылка на код бота"},
	}
	if b.cfg.AdminID != 0 {
		cmds = append(cmds, tgbotapi.BotCommand{
			Command: "status", Description: "Статус бота (админ)"},
		)
	}
	cfg := tgbotapi.NewSetMyCommands(cmds...)
	b.api.Request(cfg)
}

func (b *Bot) poll() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	updates := b.api.GetUpdatesChan(u)
	for upd := range updates {
		if upd.CallbackQuery != nil {
			b.onCallback(upd.CallbackQuery)
		} else if upd.Message != nil && upd.Message.IsCommand() {
			b.onCommand(upd.Message)
		} else if upd.Message != nil {
			b.onText(upd.Message)
		}
	}
}

func (b *Bot) isAdmin(uid int64) bool {
	return b.cfg.AdminID != 0 && uid == b.cfg.AdminID
}

func (b *Bot) touchUser(uid int64, username string) {
	b.store.UpdateLastActive(uid)
	b.store.UpdateUsername(uid, username)
}

func userLog(u *tgbotapi.User) string {
	if u.UserName != "" {
		return fmt.Sprintf("@%s (%d)", u.UserName, u.ID)
	}
	return fmt.Sprintf("uid:%d", u.ID)
}

func (b *Bot) onCommand(msg *tgbotapi.Message) {
	uid := msg.From.ID
	b.touchUser(uid, msg.From.UserName)
	log.Printf("[cmd] /%s from %s", msg.Command(), userLog(msg.From))

	switch msg.Command() {
	case "start":
		b.cmdStart(msg)
	case "status":
		if b.isAdmin(uid) {
			b.cmdStatus(msg)
		} else {
			b.reply(msg.Chat.ID, "Нет доступа.")
		}
	case "repo":
		b.reply(msg.Chat.ID,
			"Исходный код бота открыт:\n"+
				"<a href=\""+repoURL+"\">"+repoMessage+"</a>")
	default:
		b.reply(msg.Chat.ID, "Неизвестная команда. Нажмите /start")
	}
}

func (b *Bot) cmdStart(msg *tgbotapi.Message) {
	uid := msg.From.ID
	chatID := msg.Chat.ID

	users := b.store.LoadUsers()
	_, hasCode := users[uid]
	kb := mainKeyboard(b.isAdmin(uid), hasCode)

	if u, ok := users[uid]; ok {
		masked := maskCode(u.Code)
		interval := u.GetInterval()
		txt := msgWelcomeRegistered(b.cfg.SiteDomain, masked, interval)
		b.send(chatID, txt, kb)
	} else {
		b.send(chatID, msgWelcome(b.cfg.SiteDomain), kb)
	}
}

func (b *Bot) cmdStartPlain(chatID int64, uid int64) {
	users := b.store.LoadUsers()
	_, hasCode := users[uid]
	kb := mainKeyboard(b.isAdmin(uid), hasCode)

	if u, ok := users[uid]; ok {
		masked := maskCode(u.Code)
		interval := u.GetInterval()
		txt := msgWelcomeRegistered(b.cfg.SiteDomain, masked, interval)
		b.send(chatID, txt, kb)
	} else {
		b.send(chatID, msgWelcome(b.cfg.SiteDomain), kb)
	}
}

func (b *Bot) onCallback(q *tgbotapi.CallbackQuery) {
	b.api.Request(tgbotapi.NewCallback(q.ID, ""))

	uid := q.From.ID
	chatID := q.Message.Chat.ID
	msgID := q.Message.MessageID
	data := q.Data
	b.touchUser(uid, q.From.UserName)
	log.Printf("[btn] %s from %s", data, userLog(q.From))

	switch data {
	case "set_code":
		users := b.store.LoadUsers()
		if _, already := users[uid]; !already && b.cfg.MaxUsers > 0 && len(users) >= b.cfg.MaxUsers {
			b.edit(chatID, msgID, "Лимит пользователей исчерпан. Попробуйте позже.", mainKeyboard(b.isAdmin(uid), false))
			return
		}
		b.mu.Lock()
		b.waiting[uid] = true
		b.mu.Unlock()
		b.edit(chatID, msgID, msgSetCode(), nil)

	case "check":
		b.editCheck(chatID, msgID, uid)

	case "disable":
		kb := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ Да, удалить", "disable_confirm"),
				tgbotapi.NewInlineKeyboardButtonData("❌ Нет, назад", "menu"),
			),
		)
		b.edit(chatID, msgID, msgDeleteConfirm(), &kb)

	case "disable_confirm":
		b.store.RemoveUser(uid)
		b.store.RemoveState(uid)
		b.edit(chatID, msgID, msgCodeDeleted(), nil)

	case "menu":
		users := b.store.LoadUsers()
		_, hasCode := users[uid]
		kb := mainKeyboard(b.isAdmin(uid), hasCode)
		if u, ok := users[uid]; ok {
			masked := maskCode(u.Code)
			interval := u.GetInterval()
			b.edit(chatID, msgID, msgWelcomeRegistered(b.cfg.SiteDomain, masked, interval), kb)
		} else {
			b.edit(chatID, msgID, msgWelcome(b.cfg.SiteDomain), kb)
		}

	case "notif_settings":
		users := b.store.LoadUsers()
		if _, hasCode := users[uid]; !hasCode {
			b.edit(chatID, msgID, "Сначала настройте код участника.", mainKeyboard(b.isAdmin(uid), false))
			return
		}
		enabled := b.store.GetNotifEnabled(uid)
		interval := b.store.GetCheckInterval(uid)
		kb := notifSettingsKeyboard(enabled, interval)
		b.edit(chatID, msgID, msgNotifSettings(enabled, interval), kb)

	case "notif_toggle":
		enabled := b.store.GetNotifEnabled(uid)
		b.store.SetNotifEnabled(uid, !enabled)
		newEnabled := !enabled
		kb := notifSettingsKeyboard(newEnabled, b.store.GetCheckInterval(uid))
		b.edit(chatID, msgID, msgNotifToggled(newEnabled)+"\n\n"+msgNotifSettings(newEnabled, b.store.GetCheckInterval(uid)), kb)

	case "notif_15":
		b.store.SetCheckInterval(uid, 15)
		enabled := b.store.GetNotifEnabled(uid)
		kb := notifSettingsKeyboard(enabled, 15)
		b.edit(chatID, msgID, msgIntervalChanged(15)+"\n\n"+msgNotifSettings(enabled, 15), kb)

	case "notif_30":
		b.store.SetCheckInterval(uid, 30)
		enabled := b.store.GetNotifEnabled(uid)
		kb := notifSettingsKeyboard(enabled, 30)
		b.edit(chatID, msgID, msgIntervalChanged(30)+"\n\n"+msgNotifSettings(enabled, 30), kb)

	case "notif_60":
		b.store.SetCheckInterval(uid, 60)
		enabled := b.store.GetNotifEnabled(uid)
		kb := notifSettingsKeyboard(enabled, 60)
		b.edit(chatID, msgID, msgIntervalChanged(60)+"\n\n"+msgNotifSettings(enabled, 60), kb)

	case "admin_status":
		if b.isAdmin(uid) {
			b.edit(chatID, msgID, b.buildStatusText(), adminStatusMenu())
		}

	case "admin_users":
		if b.isAdmin(uid) {
			b.edit(chatID, msgID, b.buildUsersText(), adminUsersMenu())
		}
	}
}

func (b *Bot) onText(msg *tgbotapi.Message) {
	uid := msg.From.ID
	chatID := msg.Chat.ID
	b.touchUser(uid, msg.From.UserName)
	log.Printf("[text] %q from %s", msg.Text, userLog(msg.From))

	b.mu.Lock()
	waiting := b.waiting[uid]
	b.waiting[uid] = false
	b.mu.Unlock()

	if waiting {
		code := strings.TrimSpace(msg.Text)
		if !codePattern.MatchString(code) {
			b.reply(chatID, msgInvalidCode())
			b.mu.Lock()
			b.waiting[uid] = true
			b.mu.Unlock()
			return
		}
		log.Printf("[code] user %s set code %s", userLog(msg.From), maskCode(code))
		b.store.SaveUser(uid, code)
		b.reply(chatID, msgCodeSaved())
		b.handleCheck(chatID, uid)
		return
	}

	switch msg.Text {
	case "📊 Результаты":
		b.handleCheck(chatID, uid)
	case "⚙️ Настройки":
		users := b.store.LoadUsers()
		if _, hasCode := users[uid]; !hasCode {
			b.sendRaw(chatID, "Сначала настройте код участника.\nНажмите /start")
			return
		}
		enabled := b.store.GetNotifEnabled(uid)
		interval := b.store.GetCheckInterval(uid)
		kb := notifSettingsKeyboard(enabled, interval)
		b.send(chatID, msgNotifSettings(enabled, interval), kb)
	case "❓ Помощь":
		b.cmdStartPlain(chatID, uid)
	}
}

func (b *Bot) handleCheck(chatID int64, uid int64) {
	users := b.store.LoadUsers()
	u, ok := users[uid]
	if !ok {
		b.send(chatID, "Сначала настройте код участника.", mainKeyboard(false, false))
		return
	}

	results, err := FetchResults(u.Code)
	if err != nil {
		b.send(chatID, msgSiteUnavailable(), mainKeyboard(b.isAdmin(uid), true))
		return
	}
	if len(results) == 0 {
		b.send(chatID, msgNoResults(), resultMenu())
		return
	}

	changed := b.store.UpdateUserResults(uid, results)
	text := FormatResults(results)
	if len(changed) > 0 {
		text = msgUpdateHeader() + text
	}
	b.send(chatID, text, resultMenu())
}

func (b *Bot) editCheck(chatID int64, msgID int, uid int64) {
	users := b.store.LoadUsers()
	u, ok := users[uid]
	if !ok {
		b.edit(chatID, msgID, "Сначала настройте код участника.", mainKeyboard(false, false))
		return
	}

	results, err := FetchResults(u.Code)
	if err != nil {
		b.edit(chatID, msgID, msgSiteUnavailable(), resultMenu())
		return
	}
	if len(results) == 0 {
		b.edit(chatID, msgID, msgNoResults(), resultMenu())
		return
	}

	changed := b.store.UpdateUserResults(uid, results)
	text := FormatResults(results)
	if len(changed) > 0 {
		text = msgUpdateHeader() + text
	}
	b.edit(chatID, msgID, text, resultMenu())
}

func (b *Bot) cmdStatus(msg *tgbotapi.Message) {
	b.send(msg.Chat.ID, b.buildStatusText(), adminStatusMenu())
}

func (b *Bot) send(chatID int64, text string, kb *tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = true
	if kb != nil {
		msg.ReplyMarkup = kb
	} else {
		kbReply := replyKeyboard()
		msg.ReplyMarkup = kbReply
	}
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("send error: %v", err)
	}
}

func (b *Bot) reply(chatID int64, text string) {
	b.send(chatID, text, nil)
}

func (b *Bot) sendRaw(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = true
	msg.ReplyMarkup = replyKeyboard()
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("send error: %v", err)
	}
}

func (b *Bot) edit(chatID int64, messageID int, text string, kb *tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = true
	if kb != nil {
		msg.ReplyMarkup = kb
	}
	if _, err := b.api.Send(msg); err != nil {
		if !strings.Contains(err.Error(), "message is not modified") {
			log.Printf("edit error: %v", err)
		}
	}
}

func maskCode(code string) string {
	if len(code) < 8 {
		return "****"
	}
	return code[:4] + "****" + code[len(code)-4:]
}

func replyKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📊 Результаты"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("⚙️ Настройки"),
			tgbotapi.NewKeyboardButton("❓ Помощь"),
		),
	)
}

func mainKeyboard(isAdmin bool, hasCode bool) *tgbotapi.InlineKeyboardMarkup {
	rows := [][]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📊 Результаты", "check"),
		),
	}
	if hasCode {
		rows = append(rows,
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("⚙️ Настройки уведомлений", "notif_settings"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔑 Изменить код", "set_code"),
				tgbotapi.NewInlineKeyboardButtonData("🗑 Удалить код", "disable"),
			),
		)
	} else {
		rows = append(rows,
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔑 Настроить код", "set_code"),
			),
		)
	}
	if isAdmin {
		rows = append(rows,
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("📈 Статус", "admin_status"),
				tgbotapi.NewInlineKeyboardButtonData("👥 Юзеры", "admin_users"),
			),
		)
	}
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	return &kb
}

func notifSettingsKeyboard(enabled bool, interval int) *tgbotapi.InlineKeyboardMarkup {
	toggleText := "🔕 Выключить уведомления"
	if !enabled {
		toggleText = "🔔 Включить уведомления"
	}

	interval15 := "15 мин"
	interval30 := "30 мин"
	interval60 := "60 мин"
	if interval == 15 {
		interval15 = "15 мин ✓"
	}
	if interval == 30 {
		interval30 = "30 мин ✓"
	}
	if interval == 60 {
		interval60 = "60 мин ✓"
	}

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(toggleText, "notif_toggle"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(interval15, "notif_15"),
			tgbotapi.NewInlineKeyboardButtonData(interval30, "notif_30"),
			tgbotapi.NewInlineKeyboardButtonData(interval60, "notif_60"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("◀️ Назад", "menu"),
		),
	)
	return &kb
}

func adminStatusMenu() *tgbotapi.InlineKeyboardMarkup {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 Обновить", "admin_status"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👥 Юзеры", "admin_users"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🏠 Меню", "menu"),
		),
	)
	return &kb
}

func adminUsersMenu() *tgbotapi.InlineKeyboardMarkup {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 Обновить", "admin_users"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📈 Статус", "admin_status"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🏠 Меню", "menu"),
		),
	)
	return &kb
}

func resultMenu() *tgbotapi.InlineKeyboardMarkup {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 Проверить снова", "check"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🏠 Меню", "menu"),
		),
	)
	return &kb
}

func (b *Bot) buildStatusText() string {
	totalUsers, activeUsers := b.store.ActiveUsers()
	uptime := stats.UptimeDuration()
	hours := int(uptime.Hours())
	mins := int(uptime.Minutes()) % 60
	alloc, sys := GetRAM()

	limitStr := "без лимита"
	if b.cfg.MaxUsers > 0 {
		limitStr = fmt.Sprintf("%d", b.cfg.MaxUsers)
	}

	siteOK, siteMs := CheckSiteAvailable()

	return msgAdminStatus(
		b.cfg.SiteDomain,
		siteOK, siteMs,
		hours, mins,
		totalUsers, activeUsers,
		limitStr,
		stats.SiteVisitsToday(), stats.TotalSiteVisits(),
		FormatBytes(stats.TotalBytes()),
		FormatBytes(int64(alloc)), FormatBytes(int64(sys)),
		runtime.NumGoroutine(),
	)
}

func (b *Bot) buildUsersText() string {
	users := b.store.LoadUsers()
	if len(users) == 0 {
		return msgAdminUsersEmpty()
	}
	var sb strings.Builder
	sb.WriteString(msgAdminUsersHeader())
	for uid, entry := range users {
		masked := maskCode(entry.Code)
		interval := entry.GetInterval()
		notif := "вкл"
		if !entry.Enabled {
			notif = "выкл"
		}
		name := fmt.Sprintf("%d", uid)
		if entry.Username != "" {
			name = fmt.Sprintf("%d - @%s", uid, entry.Username)
		}
		sb.WriteString(fmt.Sprintf("<code>%s</code>\n", name))
		sb.WriteString(fmt.Sprintf("  Код: %s\n", masked))
		sb.WriteString(fmt.Sprintf("  Интервал: %d мин | Уведомления: %s\n\n", interval, notif))
	}
	return sb.String()
}

func FormatBytes(b int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
