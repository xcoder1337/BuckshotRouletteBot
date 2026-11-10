package ui

import (
	"strings"
	"testing"

	"buckshot-roulette-bot/game"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// gameForKeyboard — детерминированная игра для проверки клавиатур.
// Ходит A (TelegramID 1). У A: 🍺 (ID 1), 🪚 (ID 2). У B: 🚬 (ID 3), 💉 (ID 4).
func gameForKeyboard() *game.Game {
	g := game.NewGame("KB", 100, &game.Player{TelegramID: 1, DisplayName: "A"})
	g.Join(&game.Player{TelegramID: 2, DisplayName: "B"})
	g.CurrentTurn = game.Player1
	g.Players[0].Items = []game.Item{{Type: game.ItemBeer, ID: 1}, {Type: game.ItemHandsaw, ID: 2}}
	g.Players[1].Items = []game.Item{{Type: game.ItemCigarettes, ID: 3}, {Type: game.ItemAdrenaline, ID: 4}}
	return g
}

func collectCallbacks(kb tgbotapi.InlineKeyboardMarkup) []string {
	var out []string
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData != nil {
				out = append(out, *btn.CallbackData)
			}
		}
	}
	return out
}

func hasCallback(cbs []string, target string) bool {
	for _, c := range cbs {
		if c == target {
			return true
		}
	}
	return false
}

// collectLabels — тексты кнопок (для проверки подписей).
func collectLabels(kb tgbotapi.InlineKeyboardMarkup) []string {
	var out []string
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			out = append(out, btn.Text)
		}
	}
	return out
}

func hasLabel(labels []string, target string) bool {
	for _, l := range labels {
		if l == target {
			return true
		}
	}
	return false
}

// Подписи кнопок предметов: «🍺 Пиво», а не «🍺 🍺 Пиво». item.Name() уже
// содержит эмодзи, и склейка Emoji()+" "+Name() давала удвоенное эмодзи —
// реальный баг, который тут и ловится.
func TestItemButtonLabels(t *testing.T) {
	g := gameForKeyboard()
	own := collectLabels(GameActionKeyboard(g))
	steal := collectLabels(AdrenalineKeyboard(g))

	if !hasLabel(own, "🍺 Пиво") || !hasLabel(own, "🪚 Ножовка") {
		t.Fatalf("подписи своих предметов неверны: %v", own)
	}
	if !hasLabel(steal, "🚬 Сигареты") {
		t.Fatalf("подписи меню кражи неверны: %v", steal)
	}

	for _, labels := range [][]string{own, steal} {
		for _, l := range labels {
			for _, it := range game.AllItemTypes() {
				if strings.Contains(l, it.Emoji()+" "+it.Emoji()) {
					t.Fatalf("эмодзи продублирован в подписи кнопки: %q", l)
				}
			}
		}
	}
}

// Клавиатура действий показывает ТОЛЬКО свои предметы + выстрелы
func TestGameActionKeyboardShowsOwnItems(t *testing.T) {
	cbs := collectCallbacks(GameActionKeyboard(gameForKeyboard()))

	if !hasCallback(cbs, "use_item_1") || !hasCallback(cbs, "use_item_2") {
		t.Fatalf("own item buttons missing: %v", cbs)
	}
	// Предметы соперника НЕ должны светиться в обычном меню
	if hasCallback(cbs, "use_item_3") || hasCallback(cbs, "steal_item_3") {
		t.Fatalf("opponent items leaked into action keyboard: %v", cbs)
	}
	if !hasCallback(cbs, "shoot_opponent") || !hasCallback(cbs, "shoot_self") {
		t.Fatalf("shoot buttons wrong: %v", cbs)
	}
}

// Меню кражи показывает ТОЛЬКО предметы соперника — это баг-регресс: раньше
// сюда попадали предметы самого игрока, и красть можно было «своё».
func TestAdrenalineKeyboardShowsOpponentItemsOnly(t *testing.T) {
	cbs := collectCallbacks(AdrenalineKeyboard(gameForKeyboard()))

	// Соперник: 🚬 (ID 3) — можно украсть
	if !hasCallback(cbs, "steal_item_3") {
		t.Fatalf("opponent item missing from steal menu: %v", cbs)
	}
	// Свои предметы (ID 1, 2) НЕ должны попасть в меню кражи
	if hasCallback(cbs, "steal_item_1") || hasCallback(cbs, "steal_item_2") ||
		hasCallback(cbs, "use_item_1") || hasCallback(cbs, "use_item_2") {
		t.Fatalf("OWN items leaked into steal menu — exactly the reported bug: %v", cbs)
	}
	// Чужой адреналин (ID 4) красть нельзя
	if hasCallback(cbs, "steal_item_4") {
		t.Fatalf("adrenaline should not be stealable: %v", cbs)
	}
	// Отмены НЕТ (как в оригинале): раз использовал адреналин — обязан красть.
	// UseItem гарантирует наличие хотя бы одной кражи, так что застревания нет.
	if hasCallback(cbs, "cancel_adrenaline") {
		t.Fatalf("cancel button must NOT exist in steal menu: %v", cbs)
	}
}
