package game

import "fmt"

// ItemType — тип предмета
type ItemType int

const (
	ItemBeer            ItemType = iota // 🍺 Пиво — извлечь текущий патрон
	ItemCigarettes                      // 🚬 Сигареты — восстановить 1 HP
	ItemMagnifyingGlass                 // 🔍 Лупа — посмотреть текущий патрон
	ItemHandsaw                         // 🪚 Ножовка — удвоить урон следующего выстрела
	ItemHandcuffs                       // ⛓️ Наручники — пропуск хода противника
	ItemBurnerPhone                     // 📱 Телефон — узнать случайный патрон в обойме
	ItemInverter                        // 🔄 Инвертер — меняет тип текущего патрона
	ItemExpiredMedicine                 // 💊 Просроченные таблетки — +2 или -1 HP
	ItemAdrenaline                      // 💉 Адреналин — украсть предмет у противника
)

var itemNames = map[ItemType]string{
	ItemBeer:            "🍺 Пиво",
	ItemCigarettes:      "🚬 Сигареты",
	ItemMagnifyingGlass: "🔍 Лупа",
	ItemHandsaw:         "🪚 Ножовка",
	ItemHandcuffs:       "⛓️ Наручники",
	ItemBurnerPhone:     "📱 Телефон",
	ItemInverter:        "🔄 Инвертер",
	ItemExpiredMedicine: "💊 Таблетки",
	ItemAdrenaline:      "💉 Адреналин",
}

var itemDescriptions = map[ItemType]string{
	ItemBeer:            "Извлечь текущий патрон из ствола",
	ItemCigarettes:      "Восстановить 1 ❤️",
	ItemMagnifyingGlass: "Подсмотреть текущий патрон",
	ItemHandsaw:         "Следующий выстрел наносит двойной урон",
	ItemHandcuffs:       "Противник пропускает следующий ход",
	ItemBurnerPhone:     "Узнать тип случайного патрона в обойме",
	ItemInverter:        "Инвертировать текущий патрон (боевой↔холостой)",
	ItemExpiredMedicine: "40% шанс +2❤️, 60% шанс -1❤️",
	ItemAdrenaline:      "Украсть предмет противника в свой инвентарь",
}

var itemCallbacks = map[ItemType]string{
	ItemBeer:            "item_beer",
	ItemCigarettes:      "item_cigarettes",
	ItemMagnifyingGlass: "item_magnifying",
	ItemHandsaw:         "item_handsaw",
	ItemHandcuffs:       "item_handcuffs",
	ItemBurnerPhone:     "item_phone",
	ItemInverter:        "item_inverter",
	ItemExpiredMedicine: "item_medicine",
	ItemAdrenaline:      "item_adrenaline",
}

// Item — конкретный предмет в инвентаре
type Item struct {
	Type ItemType
	ID   int // уникальный ID для идентификации
}

func (i Item) Name() string {
	return itemNames[i.Type]
}

func (i Item) Description() string {
	return itemDescriptions[i.Type]
}

func (i Item) Emoji() string {
	emojis := map[ItemType]string{
		ItemBeer:            "🍺",
		ItemCigarettes:      "🚬",
		ItemMagnifyingGlass: "🔍",
		ItemHandsaw:         "🪚",
		ItemHandcuffs:       "⛓️",
		ItemBurnerPhone:     "📱",
		ItemInverter:        "🔄",
		ItemExpiredMedicine: "💊",
		ItemAdrenaline:      "💉",
	}
	return emojis[i.Type]
}

func (i Item) CallbackData() string {
	return fmt.Sprintf("%s_%d", itemCallbacks[i.Type], i.ID)
}

// AllItemTypes возвращает все типы предметов
func AllItemTypes() []ItemType {
	return []ItemType{
		ItemBeer,
		ItemCigarettes,
		ItemMagnifyingGlass,
		ItemHandsaw,
		ItemHandcuffs,
		ItemBurnerPhone,
		ItemInverter,
		ItemExpiredMedicine,
		ItemAdrenaline,
	}
}
