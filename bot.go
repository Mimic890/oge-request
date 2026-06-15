package main

import (
	"fmt"
	"log"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

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

	users, _ := b.store.LoadUsers()
	txt := "Привет! Я бот для мониторинга результатов ОГЭ.\n\n"
	if u, ok := users[uid]; ok {
		masked := maskCode(u.Code)
		txt += fmt.Sprintf("Код: %s\nИнтервал: %d мин\n\nВыберите действие:", masked, int(b.cfg.CheckInterval.Seconds())/60)
	} else {
		txt += fmt.Sprintf("Проверяю результаты каждые %d мин.\n\nНастройте код участника для начала.", int(b.cfg.CheckInterval.Seconds())/60)
	}

	if b.isAdmin(uid) {
		kb := adminMenu()
		b.send(chatID, txt, kb)
		return
	}
	b.send(chatID, txt, userMenu())
}

func (b *Bot) onCallback(q *tgbotapi.CallbackQuery) {
	b.api.Request(tgbotapi.NewCallback(q.ID, ""))

	uid := q.From.ID
	chatID := q.Message.Chat.ID
	data := q.Data

	switch data {
	case "set_code":
		users, _ := b.store.LoadUsers()
		if _, already := users[uid]; !already && b.cfg.MaxUsers > 0 && len(users) >= b.cfg.MaxUsers {
			b.edit(chatID, q.Message.MessageID, "Лимит пользователей исчерпан. Попробуйте позже.", userMenu())
			return
		}
		b.mu.Lock()
		b.waiting[uid] = true
		b.mu.Unlock()
		b.edit(chatID, q.Message.MessageID, "Введите код участника в формате XXXX-XXXX-XXXX:\n\nОтправьте /start для отмены.", nil)

	case "check":
		b.handleCheck(chatID, uid)

	case "my_results":
		b.handleMyResults(chatID, uid)

	case "disable":
		kb := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Да, удалить", "disable_confirm"),
				tgbotapi.NewInlineKeyboardButtonData("Нет, назад", "menu"),
			),
		)
		b.edit(chatID, q.Message.MessageID, "Удалить код участника и прекратить проверки?", &kb)

	case "disable_confirm":
		b.store.RemoveUser(uid)
		b.store.RemoveState(uid)
		b.edit(chatID, q.Message.MessageID, "Бот отключён. Код и результаты удалены.\n\nДля повторного использования: /start", nil)

	case "menu":
		b.cmdStart(&tgbotapi.Message{
			From: q.From,
			Chat: &tgbotapi.Chat{ID: chatID},
		})

	case "admin_status":
		if b.isAdmin(uid) {
			b.handleStatus(chatID)
		}

	case "admin_users":
		if b.isAdmin(uid) {
			b.handleUsers(chatID)
		}

	case "admin_ram":
		if b.isAdmin(uid) {
			b.handleRAM(chatID)
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
	b.reply(chatID, "Код сохранён.\nПроверяю результаты...")

	b.handleCheck(chatID, uid)
}

func (b *Bot) handleCheck(chatID int64, uid int64) {
	users, _ := b.store.LoadUsers()
	u, ok := users[uid]
	if !ok {
		b.send(chatID, "Сначала настройте код участника.", userMenu())
		return
	}

	results, err := FetchResults(u.Code)
	if err != nil {
		b.send(chatID, fmt.Sprintf("Ошибка запроса: %v", err), resultMenu())
		return
	}
	if len(results) == 0 {
		b.send(chatID, "Результаты не найдены.", resultMenu())
		return
	}

	state, _ := b.store.LoadState()
	old := state[uid]
	changed := DiffResults(old, results)
	text := FormatResults(results)
	if len(changed) > 0 {
		text = "Обнаружены изменения!\n\n" + text
	}
	b.send(chatID, text, resultMenu())

	state[uid] = results
	b.store.SaveState(state)
}

func (b *Bot) handleMyResults(chatID int64, uid int64) {
	state, _ := b.store.LoadState()
	results, ok := state[uid]
	if !ok || len(results) == 0 {
		b.send(chatID, "Нет сохранённых результатов.\nНажмите «Проверить результаты».", userMenu())
		return
	}
	b.send(chatID, FormatResults(results), resultMenu())
}

func (b *Bot) cmdStatus(msg *tgbotapi.Message) {
	b.handleStatus(msg.Chat.ID)
}

func (b *Bot) handleStatus(chatID int64) {
	users, _ := b.store.LoadUsers()
	totalUsers := len(users)
	state, _ := b.store.LoadState()
	activeUsers := 0
	for uid := range users {
		if _, ok := state[uid]; ok {
			activeUsers++
		}
	}

	uptime := stats.UptimeDuration()
	hours := int(uptime.Hours())
	mins := int(uptime.Minutes()) % 60

	alloc, sys := GetRAM()

	limitStr := "без лимита"
	if b.cfg.MaxUsers > 0 {
		limitStr = fmt.Sprintf("%d", b.cfg.MaxUsers)
	}

	text := fmt.Sprintf(
		"<b>Статус бота</b>\n\n"+
			"<b>Аптайм:</b> %dч %dм\n"+
			"<b>Пользователей:</b> %d/%s (активных: %d)\n"+
			"<b>Проверок сайта сегодня:</b> %d\n"+
			"<b>Всего проверок:</b> %d\n"+
			"<b>Трафик:</b> %s\n"+
			"<b>RAM (alloc):</b> %s\n"+
			"<b>RAM (sys):</b> %s",
		hours, mins,
		totalUsers, limitStr, activeUsers,
		stats.SiteVisitsToday(),
		stats.TotalSiteVisits(),
		FormatBytes(stats.TotalBytes()),
		FormatBytes(int64(alloc)),
		FormatBytes(int64(sys)),
	)

	kb := adminStatusMenu()
	b.send(chatID, text, kb)
}

func (b *Bot) handleUsers(chatID int64) {
	users, _ := b.store.LoadUsers()
	if len(users) == 0 {
		b.send(chatID, "Нет зарегистрированных пользователей.", adminStatusMenu())
		return
	}

	text := "<b>Пользователи:</b>\n\n"
	for uid, entry := range users {
		masked := maskCode(entry.Code)
		text += fmt.Sprintf("  <code>%d</code> — %s\n", uid, masked)
	}

	kb := adminStatusMenu()
	b.send(chatID, text, kb)
}

func (b *Bot) handleRAM(chatID int64) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	text := fmt.Sprintf(
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

	kb := adminStatusMenu()
	b.send(chatID, text, kb)
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

func init() {
	_ = time.Now()
}
