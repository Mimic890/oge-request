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
	api     *tgbotapi.BotAPI
	cfg     Config
	store   *Storage
	mu      sync.Mutex
	waiting map[int64]bool
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
		api:     api,
		cfg:     cfg,
		store:   store,
		waiting: make(map[int64]bool),
	}
	go b.poll()
	return b, nil
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

func (b *Bot) onCommand(msg *tgbotapi.Message) {
	switch msg.Command() {
	case "start":
		b.cmdStart(msg)
	case "status":
		if b.isAdmin(msg.From.ID) {
			b.cmdStatus(msg)
		} else {
			b.reply(msg.Chat.ID, "Нет доступа.")
		}
	default:
		b.reply(msg.Chat.ID, "Неизвестная команда. Нажмите /start")
	}
}

func (b *Bot) cmdStart(msg *tgbotapi.Message) {
	uid := msg.From.ID
	chatID := msg.Chat.ID

	users := b.store.LoadUsers()
	txt := "Привет! Я бот для мониторинга результатов ОГЭ.\n\n"
	if u, ok := users[uid]; ok {
		masked := maskCode(u.Code)
		txt += fmt.Sprintf("Код: %s\nИнтервал: %d мин\n\nВыберите действие:", masked, int(b.cfg.CheckInterval.Seconds())/60)
	} else {
		txt += fmt.Sprintf("Проверяю результаты каждые %d мин.\n\nНастройте код участника для начала.", int(b.cfg.CheckInterval.Seconds())/60)
	}

	if b.isAdmin(uid) {
		b.send(chatID, txt, adminMenu())
		return
	}
	b.send(chatID, txt, userMenu())
}

func (b *Bot) onCallback(q *tgbotapi.CallbackQuery) {
	b.api.Request(tgbotapi.NewCallback(q.ID, ""))

	uid := q.From.ID
	chatID := q.Message.Chat.ID
	msgID := q.Message.MessageID
	data := q.Data

	switch data {
	case "set_code":
		users := b.store.LoadUsers()
		if _, already := users[uid]; !already && b.cfg.MaxUsers > 0 && len(users) >= b.cfg.MaxUsers {
			b.edit(chatID, msgID, "Лимит пользователей исчерпан. Попробуйте позже.", userMenu())
			return
		}
		b.mu.Lock()
		b.waiting[uid] = true
		b.mu.Unlock()
		b.edit(chatID, msgID, "Введите код участника в формате XXXX-XXXX-XXXX:\n\nОтправьте /start для отмены.", nil)

	case "check":
		b.editCheck(chatID, msgID, uid)

	case "my_results":
		b.editMyResults(chatID, msgID, uid)

	case "disable":
		kb := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Да, удалить", "disable_confirm"),
				tgbotapi.NewInlineKeyboardButtonData("Нет, назад", "menu"),
			),
		)
		b.edit(chatID, msgID, "Удалить код участника и прекратить проверки?", &kb)

	case "disable_confirm":
		b.store.RemoveUser(uid)
		b.store.RemoveState(uid)
		b.edit(chatID, msgID, "Бот отключён. Код и результаты удалены.\n\nДля повторного использования: /start", nil)

	case "menu":
		users := b.store.LoadUsers()
		txt := "Привет! Я бот для мониторинга результатов ОГЭ.\n\n"
		if u, ok := users[uid]; ok {
			masked := maskCode(u.Code)
			txt += fmt.Sprintf("Код: %s\nИнтервал: %d мин\n\nВыберите действие:", masked, int(b.cfg.CheckInterval.Seconds())/60)
		} else {
			txt += fmt.Sprintf("Проверяю результаты каждые %d мин.\n\nНастройте код участника для начала.", int(b.cfg.CheckInterval.Seconds())/60)
		}
		if b.isAdmin(uid) {
			b.edit(chatID, msgID, txt, adminMenu())
		} else {
			b.edit(chatID, msgID, txt, userMenu())
		}

	case "admin_status":
		if b.isAdmin(uid) {
			b.edit(chatID, msgID, b.buildStatusText(), adminStatusMenu())
		}

	case "admin_users":
		if b.isAdmin(uid) {
			b.edit(chatID, msgID, b.buildUsersText(), adminStatusMenu())
		}

	case "admin_ram":
		if b.isAdmin(uid) {
			b.edit(chatID, msgID, b.buildRAMText(), adminStatusMenu())
		}
	}
}

func (b *Bot) onText(msg *tgbotapi.Message) {
	uid := msg.From.ID
	chatID := msg.Chat.ID

	b.mu.Lock()
	waiting := b.waiting[uid]
	b.waiting[uid] = false
	b.mu.Unlock()

	if !waiting {
		return
	}

	code := strings.TrimSpace(msg.Text)
	if !codePattern.MatchString(code) {
		b.reply(chatID, "Неверный формат. Введите код как XXXX-XXXX-XXXX:")
		b.mu.Lock()
		b.waiting[uid] = true
		b.mu.Unlock()
		return
	}

	b.store.SaveUser(uid, code)
	b.reply(chatID, "Код сохранён. Проверяю результаты...")
	b.handleCheck(chatID, uid)
}

func (b *Bot) handleCheck(chatID int64, uid int64) {
	users := b.store.LoadUsers()
	u, ok := users[uid]
	if !ok {
		b.send(chatID, "Сначала настройте код участника.", userMenu())
		return
	}

	results, err := FetchResults(u.Code)
	if err != nil {
		b.send(chatID, friendlyError(err), resultMenu())
		return
	}
	if len(results) == 0 {
		b.send(chatID, "Результаты не найдены.", resultMenu())
		return
	}

	changed := b.store.UpdateUserResults(uid, results)
	text := FormatResults(results)
	if len(changed) > 0 {
		text = "Обнаружены изменения!\n\n" + text
	}
	b.send(chatID, text, resultMenu())
}

func (b *Bot) editCheck(chatID int64, msgID int, uid int64) {
	users := b.store.LoadUsers()
	u, ok := users[uid]
	if !ok {
		b.edit(chatID, msgID, "Сначала настройте код участника.", userMenu())
		return
	}

	results, err := FetchResults(u.Code)
	if err != nil {
		b.edit(chatID, msgID, friendlyError(err), resultMenu())
		return
	}
	if len(results) == 0 {
		b.edit(chatID, msgID, "Результаты не найдены.", resultMenu())
		return
	}

	changed := b.store.UpdateUserResults(uid, results)
	text := FormatResults(results)
	if len(changed) > 0 {
		text = "Обнаружены изменения!\n\n" + text
	}
	b.edit(chatID, msgID, text, resultMenu())
}

func (b *Bot) editMyResults(chatID int64, msgID int, uid int64) {
	results := b.store.GetUserResults(uid)
	if len(results) == 0 {
		b.edit(chatID, msgID, "Нет сохранённых результатов.\nНажмите «Проверить результаты».", userMenu())
		return
	}
	b.edit(chatID, msgID, FormatResults(results), resultMenu())
}

func (b *Bot) cmdStatus(msg *tgbotapi.Message) {
	b.send(msg.Chat.ID, b.buildStatusText(), adminStatusMenu())
}

func (b *Bot) send(chatID int64, text string, kb *tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	if kb != nil {
		msg.ReplyMarkup = kb
	}
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("send error: %v", err)
	}
}

func (b *Bot) reply(chatID int64, text string) {
	b.send(chatID, text, nil)
}

func (b *Bot) edit(chatID int64, messageID int, text string, kb *tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	msg.ParseMode = "HTML"
	if kb != nil {
		msg.ReplyMarkup = kb
	}
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("edit error: %v", err)
	}
}

func maskCode(code string) string {
	if len(code) < 8 {
		return "****"
	}
	return code[:4] + "****" + code[len(code)-4:]
}

func userMenu() *tgbotapi.InlineKeyboardMarkup {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Настроить код", "set_code"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Проверить результаты", "check"),
			tgbotapi.NewInlineKeyboardButtonData("Мои результаты", "my_results"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Отключить бота", "disable"),
		),
	)
	return &kb
}

func adminMenu() *tgbotapi.InlineKeyboardMarkup {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Настроить код", "set_code"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Проверить результаты", "check"),
			tgbotapi.NewInlineKeyboardButtonData("Мои результаты", "my_results"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Отключить бота", "disable"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Статус бота", "admin_status"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Пользователи", "admin_users"),
			tgbotapi.NewInlineKeyboardButtonData("Память", "admin_ram"),
		),
	)
	return &kb
}

func adminStatusMenu() *tgbotapi.InlineKeyboardMarkup {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Обновить", "admin_status"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Пользователи", "admin_users"),
			tgbotapi.NewInlineKeyboardButtonData("Память", "admin_ram"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Меню", "menu"),
		),
	)
	return &kb
}

func resultMenu() *tgbotapi.InlineKeyboardMarkup {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Проверить снова", "check"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Меню", "menu"),
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
	siteStatus := "Доступен"
	if !siteOK {
		siteStatus = "Недоступен"
	}

	return fmt.Sprintf(
		"<b>Статус бота</b>\n\n"+
			"Сайт ege-kostroma.ru: %s (%dms)\n"+
			"Аптайм: %dч %dм\n"+
			"Пользователей: %d/%s (активных: %d)\n"+
			"Проверок сайта сегодня: %d\n"+
			"Всего проверок: %d\n"+
			"Трафик: %s\n"+
			"RAM (alloc): %s\n"+
			"RAM (sys): %s",
		siteStatus, siteMs,
		hours, mins,
		totalUsers, limitStr, activeUsers,
		stats.SiteVisitsToday(),
		stats.TotalSiteVisits(),
		FormatBytes(stats.TotalBytes()),
		FormatBytes(int64(alloc)),
		FormatBytes(int64(sys)),
	)
}

func (b *Bot) buildUsersText() string {
	users := b.store.LoadUsers()
	if len(users) == 0 {
		return "Нет зарегистрированных пользователей."
	}
	text := "<b>Пользователи:</b>\n\n"
	for uid, entry := range users {
		masked := maskCode(entry.Code)
		text += fmt.Sprintf("  <code>%d</code> — %s\n", uid, masked)
	}
	return text
}

func (b *Bot) buildRAMText() string {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return fmt.Sprintf(
		"<b>Память:</b>\n\n"+
			"Alloc: %s\n"+
			"TotalAlloc: %s\n"+
			"Sys: %s\n"+
			"NumGC: %d\n"+
			"Goroutines: %d",
		FormatBytes(int64(m.Alloc)),
		FormatBytes(int64(m.TotalAlloc)),
		FormatBytes(int64(m.Sys)),
		m.NumGC,
		runtime.NumGoroutine(),
	)
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
