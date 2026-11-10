package game

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

var itemEmojis = map[ItemType]string{
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

// Item — конкретный предмет в инвентаре
type Item struct {
	Type ItemType
	ID   int // уникальный ID для идентификации
}

func (t ItemType) Name() string  { return itemNames[t] }
func (t ItemType) Emoji() string { return itemEmojis[t] }

// IsPrivate — предмет, РЕЗУЛЬТАТ которого видит только применивший (лупа,
// телефон). В оригинале факт применения виден всегда: противник видит, что ты
// взял лупу, но не то, что она показала. Поэтому приватность касается только
// этих двух предметов, а не самого действия.
func (t ItemType) IsPrivate() bool {
	return t == ItemMagnifyingGlass || t == ItemBurnerPhone
}

func (i Item) Name() string {
	return i.Type.Name()
}

func (i Item) Description() string {
	return itemDescriptions[i.Type]
}

func (i Item) Emoji() string {
	return i.Type.Emoji()
}

func (i Item) IsPrivate() bool {
	return i.Type.IsPrivate()
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
