package ui

import (
	"fmt"

	"buckshot-roulette-bot/game"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// MainMenuKeyboard — главное меню
func MainMenuKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🎮 Создать игру", "create_game"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔗 Присоединиться", "join_game"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📖 Правила", "rules"),
		),
	)
}

// WaitingKeyboard — ожидание игрока
func WaitingKeyboard(gameID string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("🎯 Присоединиться к %s", gameID),
				fmt.Sprintf("join_%s", gameID),
			),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👤 Пригласить по нику", fmt.Sprintf("invite_%s", gameID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Отменить", "cancel_game"),
		),
	)
}

// CancelInputKeyboard — кнопка «Отменить» для состояний ввода (код комнаты,
// @ник). Из ввода кода возвращает в главное меню, из ввода @ника — в комнату
// ожидания (если она ещё открыта) или в меню. Без неё человек «застревает»:
// приходится выходить через меню туда-сюда.
func CancelInputKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Отменить", "cancel_input"),
		),
	)
}

// BackToMenuKeyboard — кнопка «назад в меню» для статичных экранов (правила).
func BackToMenuKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🏠 В меню", "main_menu"),
		),
	)
}

// JoinInviteKeyboard — кнопка «присоединиться» в личном приглашении.
// Приглашённому не нужно вводить код — достаточно нажать.
func JoinInviteKeyboard(gameID string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🎯 Присоединиться", fmt.Sprintf("join_%s", gameID)),
		),
	)
}

// GameActionKeyboard — клавиатура действий в игре
func GameActionKeyboard(g *game.Game) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	// Кнопки выстрела
	shootRow := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔫 В противника", "shoot_opponent"),
		tgbotapi.NewInlineKeyboardButtonData("🔫 В себя", "shoot_self"),
	)
	rows = append(rows, shootRow)

	// Кнопки предметов текущего игрока (с названием — чтобы было понятно новичку)
	currentPlayer := g.CurrentPlayer()
	if len(currentPlayer.Items) > 0 {
		var itemRow []tgbotapi.InlineKeyboardButton
		for i, item := range currentPlayer.Items {
			btn := tgbotapi.NewInlineKeyboardButtonData(
				item.Emoji()+" "+item.Name(),
				fmt.Sprintf("use_item_%d", item.ID),
			)
			itemRow = append(itemRow, btn)

			// По 2 в ряд — названия длинные
			if (i+1)%2 == 0 || i == len(currentPlayer.Items)-1 {
				rows = append(rows, itemRow)
				itemRow = nil
			}
		}
	}

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// AdrenalineKeyboard — выбор предмета для кражи. Отмены НЕТ: раз использовал
// адреналин — обязан что-то украсть (как в оригинале, там нет понятия «отмена»).
// UseItem заранее проверяет, что есть что красть, поэтому здесь всегда будет
// хотя бы одна кнопка.
func AdrenalineKeyboard(g *game.Game) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	opponent := g.OpponentPlayer()

	// Собираем только то, что реально можно украсть (адреналин не крадётся).
	// Фильтруем ДО разбивки на ряды: иначе предмет, после которого идёт
	// пропущенный адреналин, «застрял» бы в ряду и кнопка потерялась бы.
	var stealable []game.Item
	for _, item := range opponent.Items {
		if item.Type != game.ItemAdrenaline {
			stealable = append(stealable, item)
		}
	}

	var itemRow []tgbotapi.InlineKeyboardButton
	for i, item := range stealable {
		btn := tgbotapi.NewInlineKeyboardButtonData(
			item.Emoji()+" "+item.Name(),
			fmt.Sprintf("steal_item_%d", item.ID),
		)
		itemRow = append(itemRow, btn)

		if (i+1)%2 == 0 || i == len(stealable)-1 {
			rows = append(rows, itemRow)
			itemRow = nil
		}
	}

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// GameOverKeyboard — конец игры. У обоих игроков одинаковый выбор:
// «Реванш» (оба жмут — попадают в новую комнату) или «Закончить играть»
// (реванш отменяется, соперник получает уведомление). Кнопка несёт ID
// закончившейся игры.
func GameOverKeyboard(gameID string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 Реванш", fmt.Sprintf("rematch_%s", gameID)),
			tgbotapi.NewInlineKeyboardButtonData("⏹️ Закончить играть", fmt.Sprintf("end_game_%s", gameID)),
		),
	)
}

// RulesText — текст правил
func RulesText() string {
	return `
🎯 *BUCKSHOT ROULETTE — ПРАВИЛА*
═══════════════════════════

🔫 *Основы:*
Два игрока по очереди стреляют из дробовика\.
Обойма заряжена случайными боевыми 🔴 и холостыми 🔵 патронами\.

🎯 *Ход:*
• Выстрелить в противника
• Выстрелить в себя \(если холостой — ходишь ещё раз\!\)
⚡ С 3\-го раунда жизни соперника скрываются — «провода перерезаны»\!

🎒 *Предметы:*
🍺 Пиво — извлечь текущий патрон
🚬 Сигареты — \+1 ❤️
🔍 Лупа — посмотреть текущий патрон
🪚 Ножовка — двойной урон следующего выстрела
⛓️ Наручники — противник пропустит ход
📱 Телефон — узнать случайный патрон в обойме
🔄 Инвертер — боевой↔холостой
💊 Таблетки — 40% \+2❤️ / 60% \-1❤️
💉 Адреналин — украсть предмет противника

💀 Проигрывает тот, у кого кончится ❤️\!
`
}
