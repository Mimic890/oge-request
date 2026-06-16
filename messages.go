package main

import "fmt"

const (
	appName     = "ОГЭ Монитор"
	repoURL     = "https://github.com/Mimic890/oge-request"
	adminTG     = "@mimic_8"
	repoMessage = "Ссылка на репозиторий с кодом бота"
)

func msgWelcome() string {
	return "Привет! Я <b>ОГЭ Монитор</b>\n\nЯ слежу за результатами ОГЭ и сообщаю об изменениях.\n\n" +
		"Настройте код участника для начала работы.\n\n" +
		"<a href=\"" + repoURL + "\">" + repoMessage + "</a>"
}

func msgWelcomeRegistered(masked string, interval int) string {
	return fmt.Sprintf(
		"<b>ОГЭ Монитор</b>\n\n"+
			"Код: <code>%s</code>\n"+
			"Проверка каждые %d мин\n\n"+
			"Выберите действие:",
		masked, interval)
}

func msgSetCode() string {
	return "Введите код участника в формате <code>XXXX-XXXX-XXXX</code>\n\n" +
		"Отправьте /start для отмены."
}

func msgCodeSaved() string {
	return "Код сохранён. Проверяю результаты..."
}

func msgInvalidCode() string {
	return "Неверный формат. Введите код как <code>XXXX-XXXX-XXXX</code>:"
}

func msgResultsHeader() string {
	return "<b>Результаты ОГЭ</b>\n\n"
}

func msgUpdateHeader() string {
	return "<b>Обновление результатов!</b>\n\n"
}

func msgNoResults() string {
	return "Результаты не найдены."
}

func msgNoSavedResults() string {
	return "Нет сохранённых результатов.\nНажмите «Проверить результаты»."
}

func msgSiteUnavailable() string {
	return "Сайт временно недоступен. Если проблема сохраняется, обратитесь к администратору " + adminTG
}

func msgCodeDeleted() string {
	return "Код удалён. Бот отключён.\n\nДля повторного использования: /start"
}

func msgDeleteConfirm() string {
	return "Удалить код участника и прекратить проверки?"
}

// Notification settings

func msgNotifSettings(enabled bool, interval int) string {
	status := "Включены"
	if !enabled {
		status = "Выключены"
	}
	return fmt.Sprintf(
		"<b>Настройки уведомлений</b>\n\n"+
			"Уведомления: %s\n"+
			"Интервал проверки: %d мин",
		status, interval)
}

func msgNotifToggled(enabled bool) string {
	if enabled {
		return "Уведомления включены. Вы будете получать сообщения об изменениях."
	}
	return "Уведомления выключены. Вы не будете получать автоматические уведомления."
}

func msgIntervalChanged(interval int) string {
	return fmt.Sprintf("Интервал проверки изменён на %d мин.", interval)
}

// Site availability

func msgSiteDownAdmin(failures int) string {
	return fmt.Sprintf(
		"<b>Сайт недоступен</b>\n\n"+
			"ege-kostroma.ru не отвечает (%d попыток подряд).\n"+
			"Проверки результатов приостановлены.",
		failures)
}

func msgSiteRecoveredAdmin() string {
	return "<b>Сайт восстановлен</b>\n\nПроверки результатов возобновлены."
}

// Inactivity

func msgInactivityWarning(daysInactive int) string {
	return fmt.Sprintf(
		"<b>Уведомление</b>\n\n"+
			"Вы не пользовались ботом %d дней.\n"+
			"Через 7 дней ваши данные будут удалены.\n"+
			"Отправьте /start чтобы сохранить аккаунт.",
		daysInactive)
}

func msgInactivityFarewell() string {
	return "<b>Прощание</b>\n\n" +
		"Ваши данные будут удалены в течение часа.\n" +
		"Спасибо за использование бота!\n\n" +
		"Если захотите вернуться — просто напишите /start"
}

// Admin status

func msgAdminStatus(siteOK bool, siteMs int64, hours, mins, totalUsers, activeUsers int, limitStr string, visitsToday, totalVisits int64, traffic, ramAlloc, ramSys string) string {
	siteIcon := "🟢"
	if !siteOK {
		siteIcon = "🔴"
	}
	return fmt.Sprintf(
		"<b>Статус бота</b>\n\n"+
			"%s Сайт: %s (%dms)\n"+
			"⏱ Аптайм: %dч %dм\n"+
			"👥 Пользователей: %d/%s (активных: %d)\n"+
			"📊 Проверок сегодня: %d\n"+
			"📈 Всего проверок: %d\n"+
			"📡 Трафик: %s\n"+
			"💾 RAM (alloc): %s\n"+
			"💾 RAM (sys): %s",
		siteIcon, func() string {
			if siteOK {
				return "Доступен"
			}
			return "Недоступен"
		}(), siteMs,
		hours, mins,
		totalUsers, limitStr, activeUsers,
		visitsToday, totalVisits,
		traffic, ramAlloc, ramSys)
}

func msgAdminUsersEmpty() string {
	return "Нет зарегистрированных пользователей."
}

func msgAdminUsersHeader() string {
	return "<b>Пользователи:</b>\n\n"
}

func msgAdminRAM(alloc, totalAlloc, sys string, numGC uint32, goroutines int) string {
	return fmt.Sprintf(
		"<b>Память:</b>\n\n"+
			"Alloc: %s\n"+
			"TotalAlloc: %s\n"+
			"Sys: %s\n"+
			"NumGC: %d\n"+
			"Goroutines: %d",
		alloc, totalAlloc, sys, numGC, goroutines)
}
