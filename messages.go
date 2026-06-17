package main

import "fmt"

const (
	appName     = "ОГЭ Монитор"
	repoURL     = "https://github.com/Mimic890/oge-request"
	repoMessage = "Ссылка на репозиторий с кодом бота"
)

func msgWelcome(siteDomain string) string {
	return "Привет! Я <b>📊 ОГЭ Монитор</b>\n\n" +
		"Мониторю результаты на <code>" + siteDomain + "</code>\n\n" +
		"Настройте код участника для начала работы.\n\n" +
		"<a href=\"" + repoURL + "\">" + repoMessage + "</a>"
}

func msgHelp(siteDomain, adminContact string) string {
	return fmt.Sprintf(
		"<b>📊 ОГЭ Монитор</b>\n"+
			"<code>%s</code>\n\n"+
			"Бот проверяет результаты ОГЭ и сообщает об изменениях.\n\n"+
			"<b>Как пользоваться:</b>\n"+
			"1. Нажмите «Настроить код»\n"+
			"2. Введите код участника XXXX-XXXX-XXXX\n"+
			"3. Бот начнёт проверять результаты\n\n"+
			"<b>Настройки:</b>\n"+
			"• Интервал проверки: 15, 30 или 60 мин\n"+
			"• Уведомления можно включить/выключить\n\n"+
			"<b>Администратор:</b> %s\n\n"+
			"<a href=\"%s\">%s</a>",
		siteDomain, adminContact, repoURL, repoMessage)
}

func msgWelcomeRegistered(siteDomain, masked string, interval int) string {
	return fmt.Sprintf(
		"<b>📊 ОГЭ Монитор</b>\n"+
			"<code>%s</code>\n\n"+
			"🔑 Код: <code>%s</code>\n"+
			"⏱ Проверка каждые %d мин\n\n"+
			"Выберите действие:",
		siteDomain, masked, interval)
}

func msgSetCode() string {
	return "🔑 Введите код участника в формате <code>XXXX-XXXX-XXXX</code>"
}

func msgCodeSaved() string {
	return "✅ Код сохранён. Проверяю результаты..."
}

func msgInvalidCode() string {
	return "❌ Неверный формат. Введите код как <code>XXXX-XXXX-XXXX</code>:"
}

func msgResultsHeader() string {
	return "<b>📊 Результаты ОГЭ</b>\n\n"
}

func msgUpdateHeader() string {
	return "<b>🔄 Обновление результатов!</b>\n\n"
}

func msgNoResults() string {
	return "📭 Результаты не найдены."
}

func msgSiteUnavailable(adminContact string) string {
	return "⚠️ Сайт временно недоступен. Если проблема сохраняется, обратитесь к администратору " + adminContact
}

func msgCodeDeleted() string {
	return "✅ Код удалён. Бот отключён.\n\nДля повторного использования: /start"
}

func msgDeleteConfirm() string {
	return "⚠️ Удалить код участника и прекратить проверки?"
}

// Notification settings

func msgNotifSettings(enabled bool, interval int, errorsEnabled bool) string {
	status := "🔔 Включены"
	if !enabled {
		status = "🔕 Выключены"
	}
	errStatus := "🔔 Вкл"
	if !errorsEnabled {
		errStatus = "🔕 Выкл"
	}
	return fmt.Sprintf(
		"<b>⚙️ Настройки уведомлений</b>\n\n"+
			"Уведомления: %s\n"+
			"Интервал проверки: %d мин\n"+
			"Ошибки админу: %s",
		status, interval, errStatus)
}

func msgNotifToggled(enabled bool) string {
	if enabled {
		return "🔔 Уведомления включены. Вы будете получать сообщения об изменениях."
	}
	return "🔕 Уведомления выключены. Вы не будете получать автоматические уведомления."
}

func msgIntervalChanged(interval int) string {
	return fmt.Sprintf("✅ Интервал проверки изменён на %d мин.", interval)
}

func msgErrorsToggled(enabled bool) string {
	if enabled {
		return "🔔 Уведомления об ошибках включены. Администратор будет получать сообщения при ошибках проверки."
	}
	return "🔕 Уведомления об ошибках выключены."
}

// Inactivity

func msgInactivityWarning(daysInactive int) string {
	return fmt.Sprintf(
		"<b>⚠️ Уведомление</b>\n\n"+
			"Вы не пользовались ботом %d дней.\n"+
			"Через 7 дней ваши данные будут удалены.\n"+
			"Отправьте /start чтобы сохранить аккаунт.",
		daysInactive)
}

func msgInactivityFarewell() string {
	return "<b>👋 Прощание</b>\n\n" +
		"Ваши данные будут удалены в течение часа.\n" +
		"Спасибо за использование бота!\n\n" +
		"Если захотите вернуться — просто напишите /start"
}

// Admin status

func msgAdminStatus(siteDomain string, hours, mins, totalUsers, activeUsers int, limitStr string, visitsToday, totalVisits int64, traffic, ramAlloc, ramSys string, goroutines int) string {
	return fmt.Sprintf(
		"<b>Bot Status</b>\n"+
			"<code>%s</code>\n\n"+
			"⏱ Uptime: %dh %dm\n"+
			"👥 Users: %d/%s (active: %d)\n"+
			"📊 Checks today: %d\n"+
			"📈 Total checks: %d\n"+
			"📡 Traffic: %s\n"+
			"💾 RAM: %s / %s\n"+
			"⚙️ Goroutines: %d",
		siteDomain,
		hours, mins,
		totalUsers, limitStr, activeUsers,
		visitsToday, totalVisits,
		traffic, ramAlloc, ramSys, goroutines)
}

func msgAdminUsersEmpty() string {
	return "📭 Нет зарегистрированных пользователей."
}

func msgAdminUsersHeader() string {
	return "<b>👥 Пользователи:</b>\n\n"
}
