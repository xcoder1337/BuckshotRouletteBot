package game

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// maxLives — максимум жизней (как в оригинале: старт 2, +1 за раунд, кап 5)
const maxLives = 5

// GamePhase — фаза игры
type GamePhase int

const (
	PhaseWaiting     GamePhase = iota // Ожидание второго игрока
	PhaseStarting                     // Начало раунда
	PhasePlaying                      // Игровой процесс
	PhaseUsingItem                    // Игрок использует предмет
	PhaseAdrenaline                   // Выбор предмета для кражи
	PhaseShootChoice                  // Выбор: стрелять в себя или противника
	PhaseGameOver                     // Игра окончена
)

// PlayerID — идентификатор игрока
type PlayerID int

const (
	Player1 PlayerID = 0
	Player2 PlayerID = 1
)

func (p PlayerID) Other() PlayerID {
	if p == Player1 {
		return Player2
	}
	return Player1
}

// Player — игрок
type Player struct {
	TelegramID   int64
	Username     string
	DisplayName  string
	HP           int
	MaxHP        int
	Items        []Item
	Handcuffed   bool // наручники — пропуск хода
	SkipNextTurn bool // второй ход наручников
}

func (p *Player) IsAlive() bool {
	return p.HP > 0
}

func (p *Player) HasItem(itemType ItemType) bool {
	for _, item := range p.Items {
		if item.Type == itemType {
			return true
		}
	}
	return false
}

func (p *Player) RemoveItem(itemID int) *Item {
	for i, item := range p.Items {
		if item.ID == itemID {
			removed := p.Items[i]
			p.Items = append(p.Items[:i], p.Items[i+1:]...)
			return &removed
		}
	}
	return nil
}

func (p *Player) RemoveItemByType(itemType ItemType) *Item {
	for i, item := range p.Items {
		if item.Type == itemType {
			removed := p.Items[i]
			p.Items = append(p.Items[:i], p.Items[i+1:]...)
			return &removed
		}
	}
	return nil
}

func (p *Player) ItemsDisplay() string {
	if len(p.Items) == 0 {
		return "пусто"
	}
	var parts []string
	for _, item := range p.Items {
		parts = append(parts, item.Emoji())
	}
	return strings.Join(parts, " ")
}

// GameLog — запись в лог
type GameLog struct {
	Message string
	Time    time.Time
}

// Game — основная структура игры
type Game struct {
	mu sync.RWMutex

	ID          string
	ChatID      int64
	Players     [2]*Player
	CurrentTurn PlayerID
	Phase       GamePhase
	Round       int
	Shotgun     *Shotgun
	ItemCounter int

	// RoundShells — сколько патронов было заряжено в этом раунде.
	// Остаток = len(Shotgun.Magazine). Нужно для нейтрального счётчика
	// в статусе (сколько осталось из всех), без раскрытия 🔴/🔵.
	RoundShells int

	// Состояние текущего хода
	KnownShell    *ShellType // Известный патрон (лупа)
	PhoneShell    *ShellType // Раскрытый телефоном патрон (nil — нет данных)
	PhoneShellPos int        // Позиция раскрытого патрона в обойме (1-based), 0 — нет данных

	// TelegramID игрока, которому известна эта информация. В оригинале лупа
	// и телефон показывают результат ТОЛЬКО использовавшему: соперник не должен
	// видеть «в стволе 🔴» или «патрон #2 🔵» в чужом статусе. 0 — никто.
	KnownShellOwner int64
	PhoneInfoOwner  int64

	SawedOff      bool       // Ножовка активна
	HandcuffsUsed bool       // Наручники использованы в этом ходу

	// Для адреналина
	AdrenalineActive bool
	AdrenalineTarget PlayerID

	Log []GameLog

	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewGame создаёт новую игру
func NewGame(id string, chatID int64, creator *Player) *Game {
	return &Game{
		ID:        id,
		ChatID:    chatID,
		Players:   [2]*Player{creator, nil},
		Phase:     PhaseWaiting,
		Round:     0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// Join — присоединение второго игрока
func (g *Game) Join(player *Player) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.Phase != PhaseWaiting {
		return fmt.Errorf("игра уже началась")
	}

	if g.Players[0].TelegramID == player.TelegramID {
		return fmt.Errorf("нельзя играть с самим собой")
	}

	g.Players[1] = player
	g.UpdatedAt = time.Now()
	return nil
}

// StartGame — начало игры
func (g *Game) StartGame() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.Round = 1
	g.Phase = PhasePlaying

	// Начальные жизни (как в оригинале — 2, дальше +1 каждый раунд)
	g.Players[0].MaxHP = 2
	g.Players[0].HP = 2
	g.Players[1].MaxHP = 2
	g.Players[1].HP = 2

	// Кто ходит первый — случайно (как в оригинале: переброс монетки)
	g.CurrentTurn = PlayerID(rand.Intn(2))

	// Заряжаем дробовик и раздаём предметы
	g.loadNewRound()

	g.addLog("🎮 Игра началась!")
	g.addLog(fmt.Sprintf("🎲 Монетка: %s ходит первым!", g.Players[g.CurrentTurn].DisplayName))
	g.addLog("❤️ У каждого по 2 HP")

	g.UpdatedAt = time.Now()
}

// loadNewRound — зарядка и раздача предметов
func (g *Game) loadNewRound() {
	// Новая обойма
	minShells := 2
	maxShells := 8
	if g.Round == 1 {
		minShells = 2
		maxShells = 4
	} else if g.Round == 2 {
		minShells = 3
		maxShells = 6
	} else {
		minShells = 4
		maxShells = 8
	}

	g.Shotgun = NewShotgun(minShells, maxShells)
	g.RoundShells = len(g.Shotgun.Magazine)

	g.addLog(fmt.Sprintf("🔫 Обойма заряжена: %d🔴 боевых, %d🔵 холостых",
		g.Shotgun.LiveCount(), g.Shotgun.BlankCount()))

	// Раздаём предметы (как в оригинале: 4 в 1-м раунде, +1 каждый раунд, максимум 8).
	// В оригинале предметы между раундами НЕ переносятся — выдаём свежий набор.
	itemCount := 3 + g.Round
	if itemCount > 8 {
		itemCount = 8
	}
	for _, player := range g.Players {
		player.Items = nil
		for i := 0; i < itemCount; i++ {
			item := g.randomItem()
			player.Items = append(player.Items, item)
		}
	}

	// Сброс состояний
	g.KnownShell = nil
	g.KnownShellOwner = 0
	g.PhoneShell = nil
	g.PhoneShellPos = 0
	g.PhoneInfoOwner = 0
	g.SawedOff = false
	g.Shotgun.SawedOff = false
	g.HandcuffsUsed = false
	g.AdrenalineActive = false

	for _, p := range g.Players {
		p.Handcuffed = false
		p.SkipNextTurn = false
	}
}

// randomItem — случайный предмет
func (g *Game) randomItem() Item {
	g.ItemCounter++
	types := AllItemTypes()
	return Item{
		Type: types[rand.Intn(len(types))],
		ID:   g.ItemCounter,
	}
}

// CurrentPlayer — текущий игрок
func (g *Game) CurrentPlayer() *Player {
	return g.Players[g.CurrentTurn]
}

// OpponentPlayer — противник текущего игрока
func (g *Game) OpponentPlayer() *Player {
	return g.Players[g.CurrentTurn.Other()]
}

// UseItem — использование предмета
func (g *Game) UseItem(playerID int64, itemID int) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.Phase != PhasePlaying && g.Phase != PhaseAdrenaline {
		return "", fmt.Errorf("сейчас нельзя использовать предметы")
	}

	currentPlayer := g.CurrentPlayer()
	if currentPlayer.TelegramID != playerID {
		return "", fmt.Errorf("сейчас не ваш ход")
	}

	// Ищем предмет
	var item *Item
	var owner *Player

	if g.AdrenalineActive {
		// При адреналине берём у противника
		owner = g.OpponentPlayer()
	} else {
		owner = currentPlayer
	}

	for i, it := range owner.Items {
		if it.ID == itemID {
			item = &owner.Items[i]
			break
		}
	}

	if item == nil {
		return "", fmt.Errorf("предмет не найден")
	}

	// Нельзя использовать адреналин через адреналин
	if g.AdrenalineActive && item.Type == ItemAdrenaline {
		return "", fmt.Errorf("нельзя украсть адреналин через адреналин")
	}

	// Нельзя начать кражу, если у противника нечего красть (адреналин не крадётся)
	if !g.AdrenalineActive && item.Type == ItemAdrenaline && !g.hasStealableItem(g.OpponentPlayer()) {
		return "", fmt.Errorf("у противника нет предметов для кражи")
	}

	// Кража через адреналин: предмет переходит в инвентарь текущего игрока,
	// эффект НЕ применяется сразу — игрок решает, когда его использовать
	// (как в оригинале).
	if g.AdrenalineActive {
		stolen := *item
		currentPlayer.Items = append(currentPlayer.Items, stolen)
		owner.RemoveItem(item.ID)

		g.AdrenalineActive = false
		g.Phase = PhasePlaying

		msg := fmt.Sprintf("%s украл у %s: %s", currentPlayer.DisplayName, owner.DisplayName, stolen.Name())
		g.addLog(msg)
		g.UpdatedAt = time.Now()
		return msg, nil
	}

	result, err := g.applyItem(item.Type, currentPlayer, owner, item.ID)
	if err != nil {
		// Эффект невозможен (сигареты при полном HP и т.п.) — предмет НЕ тратится.
		// Отмены в игре нет, но и сгорать предмету впустую нельзя.
		return result, err
	}

	// Убираем предмет у владельца
	owner.RemoveItem(item.ID)

	g.UpdatedAt = time.Now()

	return result, nil
}

// hasStealableItem — есть ли у игрока предмет, который реально можно украсть
// (адреналин украсть нельзя, как в оригинале). Если красть нечего, адреналин
// использовать нельзя вовсе — иначе игрок застрял бы в фазе кражи без выбора.
func (g *Game) hasStealableItem(p *Player) bool {
	for _, it := range p.Items {
		if it.Type != ItemAdrenaline {
			return true
		}
	}
	return false
}

// applyItem — применение эффекта предмета. Возвращает сообщение и ошибку.
// Если err != nil, предмет НЕ тратится: эффект невозможен (сигареты при полном
// HP, телефон при одном патроне и т.п.). Отмены, как в шахматах, в игре нет,
// но и сгорать предмету впустую — нельзя.
func (g *Game) applyItem(itemType ItemType, user *Player, owner *Player, itemID int) (string, error) {
	switch itemType {
	case ItemBeer:
		shell := g.Shotgun.Rack()
		msg := fmt.Sprintf("🍺 Извлечён патрон: %s", shell.Name())
		g.addLog(fmt.Sprintf("%s использовал 🍺: извлечён %s", user.DisplayName, shell.Emoji()))

		// Извлечение патрона «сдвигает» обойму: патрон, который подсмотрел игрок
		// лупой, ушёл в небытие (в стволе теперь другой), а раскрытый телефоном
		// патрон стал на позицию ближе к стволу. Иначе игрок действует по
		// устаревшей информации — это баг.
		g.KnownShell = nil
		g.KnownShellOwner = 0
		if g.PhoneShell != nil {
			if g.PhoneShellPos <= 1 {
				// Раскрытый патрон был текущим — он и извлечён, информация сгорает.
				g.PhoneShell = nil
				g.PhoneShellPos = 0
				g.PhoneInfoOwner = 0
			} else {
				g.PhoneShellPos--
			}
		}

		if g.Shotgun.IsEmpty() {
			g.nextRound()
			msg += "\n📦 Обойма пуста! Новый раунд!"
		}
		return msg, nil

	case ItemCigarettes:
		if user.HP < user.MaxHP {
			user.HP++
			msg := "🚬 Восстановлено 1❤️"
			if g.showHPTotals() {
				msg = fmt.Sprintf("🚬 Восстановлено 1❤️ (теперь %d/%d)", user.HP, user.MaxHP)
			}
			g.addLog(fmt.Sprintf("%s использовал 🚬", user.DisplayName))
			return msg, nil
		}
		return "", fmt.Errorf("🚬 Здоровье уже полное!")

	case ItemMagnifyingGlass:
		shell := g.Shotgun.CurrentShell()
		g.KnownShell = &shell
		g.KnownShellOwner = user.TelegramID
		msg := fmt.Sprintf("🔍 Текущий патрон: %s", shell.Name())
		g.addLog(fmt.Sprintf("%s использовал 🔍", user.DisplayName))
		return msg, nil

	case ItemHandsaw:
		if g.SawedOff {
			return "", fmt.Errorf("🪚 Ствол уже укорочен!")
		}
		g.SawedOff = true
		g.Shotgun.SawedOff = true
		msg := "🪚 Ствол укорочен! Следующий выстрел нанесёт двойной урон!"
		g.addLog(fmt.Sprintf("%s использовал 🪚", user.DisplayName))
		return msg, nil

	case ItemHandcuffs:
		opponent := g.OpponentPlayer()
		if opponent.Handcuffed {
			return "", fmt.Errorf("⛓️ Противник уже в наручниках!")
		}
		opponent.Handcuffed = true
		g.HandcuffsUsed = true
		msg := fmt.Sprintf("⛓️ %s закован в наручники! Пропустит следующий ход.", opponent.DisplayName)
		g.addLog(fmt.Sprintf("%s использовал ⛓️ на %s", user.DisplayName, opponent.DisplayName))
		return msg, nil

	case ItemBurnerPhone:
		if len(g.Shotgun.Magazine) <= 1 {
			return "", fmt.Errorf("📱 В обойме только один патрон — используй лупу!")
		}
		pos, shell := g.Shotgun.GetRandomShellInfo()
		g.PhoneShell = &shell
		g.PhoneShellPos = pos
		g.PhoneInfoOwner = user.TelegramID
		msg := g.phoneInfoText()
		g.addLog(fmt.Sprintf("%s использовал 📱", user.DisplayName))
		return msg, nil

	case ItemInverter:
		g.Shotgun.InvertCurrent()
		newShell := g.Shotgun.CurrentShell()
		// В оригинале инвертер НЕ показывает результат: ты знаешь только то,
		// что патрон в стволе сменился на противоположный. Кто видел патрон
		// лупой — сам догадается, во что он превратился (KnownShellOwner тот же).
		msg := "🔄 Инвертер! Текущий патрон сменился на противоположный."
		if g.KnownShell != nil {
			g.KnownShell = &newShell
		}
		// Если телефон раскрыл именно текущий патрон (позиция 1), инвертер его
		// сменил — обновляем и телефон, иначе игрок увидит устаревший тип.
		if g.PhoneShell != nil && g.PhoneShellPos == 1 {
			g.PhoneShell = &newShell
		}
		g.addLog(fmt.Sprintf("%s использовал 🔄", user.DisplayName))
		return msg, nil

	case ItemExpiredMedicine:
		roll := rand.Intn(100)
		if roll < 40 {
			// +2 HP
			user.HP += 2
			if user.HP > user.MaxHP {
				user.HP = user.MaxHP
			}
			msg := "💊 Повезло! +2❤️"
			if g.showHPTotals() {
				msg = fmt.Sprintf("💊 Повезло! +2❤️ (теперь %d/%d)", user.HP, user.MaxHP)
			}
			g.addLog(fmt.Sprintf("%s использовал 💊: +2❤️", user.DisplayName))
			return msg, nil
		} else {
			// -1 HP
			user.HP--
			if user.HP < 0 {
				user.HP = 0
			}
			msg := "💊 Не повезло! -1❤️"
			if g.showHPTotals() {
				msg = fmt.Sprintf("💊 Не повезло! -1❤️ (теперь %d/%d)", user.HP, user.MaxHP)
			}
			g.addLog(fmt.Sprintf("%s использовал 💊: -1❤️", user.DisplayName))
			if user.HP <= 0 {
				g.Phase = PhaseGameOver
				msg += "\n☠️ Таблетки оказались смертельными!"
			}
			return msg, nil
		}

	case ItemAdrenaline:
		// Проверку «есть что красть» (не адреналиновое) делает UseItem — здесь
		// просто включаем фазу кражи. Отмены нет: раз нажал — кради.
		opponent := g.OpponentPlayer()
		g.AdrenalineActive = true
		g.AdrenalineTarget = g.CurrentTurn.Other()
		g.Phase = PhaseAdrenaline
		msg := fmt.Sprintf("💉 Адреналин! Выбери предмет %s для кражи:", opponent.DisplayName)
		g.addLog(fmt.Sprintf("%s использовал 💉", user.DisplayName))
		return msg, nil
	}

	return "", fmt.Errorf("❓ Неизвестный предмет")
}

// Shoot — выстрел
func (g *Game) Shoot(playerID int64, shootSelf bool) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.Phase != PhasePlaying {
		return "", fmt.Errorf("сейчас нельзя стрелять")
	}

	currentPlayer := g.CurrentPlayer()
	if currentPlayer.TelegramID != playerID {
		return "", fmt.Errorf("сейчас не ваш ход")
	}

	shell := g.Shotgun.Shoot()
	damage := 1
	if g.SawedOff {
		damage = 2
	}

	var result strings.Builder

	if shootSelf {
		// Выстрел в себя
		if shell == ShellLive {
			currentPlayer.HP -= damage
			if currentPlayer.HP < 0 {
				currentPlayer.HP = 0
			}
			result.WriteString(fmt.Sprintf("💥 %s стреляет в себя!\n%s — Боевой! -%d❤️\n",
				currentPlayer.DisplayName, shell.Emoji(), damage))
			if g.showHPTotals() {
				result.WriteString(fmt.Sprintf("❤️ %s: %d/%d\n",
					currentPlayer.DisplayName, currentPlayer.HP, currentPlayer.MaxHP))
			}
			g.addLog(fmt.Sprintf("%s выстрелил в себя: 🔴 -%d❤️", currentPlayer.DisplayName, damage))
		} else {
			result.WriteString(fmt.Sprintf("💨 %s стреляет в себя!\n%s — Холостой! Ходит ещё раз!\n",
				currentPlayer.DisplayName, shell.Emoji()))
			g.addLog(fmt.Sprintf("%s выстрелил в себя: 🔵 холостой", currentPlayer.DisplayName))
		}
	} else {
		// Выстрел в противника
		opponent := g.OpponentPlayer()
		if shell == ShellLive {
			opponent.HP -= damage
			if opponent.HP < 0 {
				opponent.HP = 0
			}
			result.WriteString(fmt.Sprintf("💥 %s стреляет в %s!\n%s — Боевой! -%d❤️\n",
				currentPlayer.DisplayName, opponent.DisplayName, shell.Emoji(), damage))
			if g.showHPTotals() {
				result.WriteString(fmt.Sprintf("❤️ %s: %d/%d\n",
					opponent.DisplayName, opponent.HP, opponent.MaxHP))
			}
			g.addLog(fmt.Sprintf("%s выстрелил в %s: 🔴 -%d❤️",
				currentPlayer.DisplayName, opponent.DisplayName, damage))
		} else {
			result.WriteString(fmt.Sprintf("💨 %s стреляет в %s!\n%s — Холостой!\n",
				currentPlayer.DisplayName, opponent.DisplayName, shell.Emoji()))
			g.addLog(fmt.Sprintf("%s выстрелил в %s: 🔵 холостой",
				currentPlayer.DisplayName, opponent.DisplayName))
		}
	}

	// Сброс ножовки
	g.SawedOff = false
	g.Shotgun.SawedOff = false
	g.KnownShell = nil
	g.KnownShellOwner = 0
	g.PhoneShell = nil
	g.PhoneShellPos = 0
	g.PhoneInfoOwner = 0

	// Проверка на смерть
	if !g.Players[0].IsAlive() || !g.Players[1].IsAlive() {
		g.Phase = PhaseGameOver
		var winner *Player
		if g.Players[0].IsAlive() {
			winner = g.Players[0]
		} else {
			winner = g.Players[1]
		}
		result.WriteString(fmt.Sprintf("\n🏆 %s побеждает! 🎉", winner.DisplayName))
		g.addLog(fmt.Sprintf("🏆 %s победил!", winner.DisplayName))
		g.UpdatedAt = time.Now()
		return result.String(), nil
	}

	// Проверка пустой обоймы
	if g.Shotgun.IsEmpty() {
		// Раунд переходит: nextRound сам кидает монетку и объявляет первого
		// игрока. Выше нельзя вызывать nextTurn — иначе он «перевернёт» монетку,
		// и в логе написано одно («Монетка: A ходит первым!»), а ходит другой.
		g.nextRound()
		result.WriteString("\n📦 Обойма пуста! Новый раунд!")
		result.WriteString(fmt.Sprintf("\n🔫 Заряжено: %d🔴 боевых, %d🔵 холостых",
			g.Shotgun.LiveCount(), g.Shotgun.BlankCount()))
		g.UpdatedAt = time.Now()
		return result.String(), nil
	}

	// Переход хода
	if shootSelf && shell == ShellBlank {
		// При холостом в себя — ходишь снова
		// ход не меняется
	} else {
		g.nextTurn()
	}

	g.UpdatedAt = time.Now()
	return result.String(), nil
}

// nextTurn — следующий ход
func (g *Game) nextTurn() {
	nextPlayer := g.CurrentTurn.Other()
	opponent := g.Players[nextPlayer]

	if opponent.Handcuffed {
		// Противник в наручниках — пропускает ход
		opponent.Handcuffed = false
		g.addLog(fmt.Sprintf("⛓️ %s пропускает ход (наручники)", opponent.DisplayName))
		// Текущий игрок ходит снова
		return
	}

	g.CurrentTurn = nextPlayer
}

// nextRound — новый раунд
func (g *Game) nextRound() {
	g.Round++
	g.loadNewRound()

	// Как в оригинале: каждый раунд начинается с переброса монетки —
	// первый ход разыгрывается заново, а не переходит «по наследству».
	g.CurrentTurn = PlayerID(rand.Intn(2))
	g.addLog(fmt.Sprintf("🎲 Монетка: %s ходит первым!", g.Players[g.CurrentTurn].DisplayName))

	// Как в оригинале: каждый новый раунд +1 к максимуму жизней и полное лечение
	for _, p := range g.Players {
		if p.MaxHP < maxLives {
			p.MaxHP++
		}
		p.HP = p.MaxHP
	}

	g.addLog(fmt.Sprintf("📦 Раунд %d! Максимум жизней: %d❤️", g.Round, g.Players[0].MaxHP))

	g.UpdatedAt = time.Now()
}

// addLog — добавление записи в лог
func (g *Game) addLog(msg string) {
	g.Log = append(g.Log, GameLog{
		Message: msg,
		Time:    time.Now(),
	})
}

// showHPTotals — показывать ли числовые значения HP в общих сообщениях.
// С 3-го раунда, как в оригинале, «провода на полоске соперника перерезаны»:
// HP соперника скрыто, и тоталы не должны всплывать в результатах выстрелов,
// лечении и т.п. — иначе скрытие превратится в пустую косметику.
func (g *Game) showHPTotals() bool {
	return g.Round < 3
}

// phoneInfoText — текстовое представление раскрытого телефоном патрона.
// Позицию храним структурно (PhoneShellPos), чтобы пиво могло её корректно
// сдвинуть, и рендерим заново — а не держим устаревшую готовую строку.
func (g *Game) phoneInfoText() string {
	if g.PhoneShell == nil || g.PhoneShellPos <= 0 {
		return ""
	}
	return fmt.Sprintf("📱 Патрон #%d: %s", g.PhoneShellPos, g.PhoneShell.Name())
}

// GetStatusText — текстовый статус игры с точки зрения игрока viewerID.
// Только эмодзи и структура без разметки — финальное сообщение проходит
// через escapeMarkdown в bot.go, поэтому не должно содержать MarkdownV2.
func (g *Game) GetStatusText(viewerID int64) string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// До начала игры (комната ожидания) статуса ещё нет — не показываем
	// «Раунд 0» с пустыми полосками HP.
	if g.Phase == PhaseWaiting {
		return fmt.Sprintf("⏳ Ожидание второго игрока…\n🆔 Код: %s", g.ID)
	}

	var sb strings.Builder

	sb.WriteString("🎯 BUCKSHOT ROULETTE\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━\n")

	// Состав патронов (🔴/🔵) показывается ОДИН раз в начале раунда (в логе:
	// «Обойма заряжена: 2🔴 боевых, 2🔵 холостых»), а потом скрывается.
	// Иначе игроку незачем запоминать и считать выстрелы — это и есть суть.
	sb.WriteString(fmt.Sprintf("📍 Раунд %d\n", g.Round))
	sb.WriteString("\n")

	for i, p := range g.Players {
		if p == nil {
			continue
		}

		role := "Соперник"
		if p.TelegramID == viewerID {
			role = "Вы"
		}

		turnMark := "     "
		if PlayerID(i) == g.CurrentTurn && g.Phase == PhasePlaying {
			turnMark = "👉  "
		}

		sb.WriteString(fmt.Sprintf("%s%s (%s)\n", turnMark, p.DisplayName, role))

		// Жизни: свои видны всегда. С 3-го раунда полоска соперника «перерезана»
		// (как в оригинале в финале) — его HP скрыто, считаешь урон по логам.
		if g.Round >= 3 && p.TelegramID != viewerID {
			sb.WriteString("      ✂️  HP скрыт\n")
		} else {
			hpBar := strings.Repeat("❤️", p.HP) + strings.Repeat("🖤", p.MaxHP-p.HP)
			sb.WriteString(fmt.Sprintf("      %s  %d/%d\n", hpBar, p.HP, p.MaxHP))
		}

		// Предметы видны у ОБОИХ игроков — как в оригинале, всё лежит на столе.
		// Без этого непонятно, что вообще можно украсть адреналином.
		if len(p.Items) > 0 {
			sb.WriteString(fmt.Sprintf("      🎒 %s\n", p.ItemsDisplay()))
		}
		if p.Handcuffed {
			sb.WriteString("      ⛓️ В наручниках!\n")
		}
		sb.WriteString("\n")
	}

	// Нейтральный счётчик патронов: сколько осталось из заряженных.
	// 🔋 = патрон ещё в стволе, 🪫 = уже выстрелян/извлечён.
	// Не раскрывает 🔴/🔵 — только подсказка, как поддон в оригинале,
	// чтобы игрок не забывал, сколько всего было и сколько осталось.
	if g.RoundShells > 0 {
		remaining := len(g.Shotgun.Magazine)
		if remaining > g.RoundShells {
			remaining = g.RoundShells
		}
		sb.WriteString(fmt.Sprintf("🔫 Патроны: %s%s  (%d/%d)\n",
			strings.Repeat("🔋", remaining),
			strings.Repeat("🪫", g.RoundShells-remaining),
			remaining, g.RoundShells))
	}

	if g.SawedOff {
		sb.WriteString("🪚 Ствол укорочен — следующий выстрел 2x урон!\n")
	}
	// Лупа и телефон — приватная информация ТОЛЬКО использовавшего.
	// Раньше она попадала в статус обоих: соперник видел чужой секрет.
	if g.KnownShell != nil && g.KnownShellOwner == viewerID {
		sb.WriteString(fmt.Sprintf("🔍 В стволе: %s\n", g.KnownShell.Name()))
	}
	if g.PhoneShell != nil && g.PhoneInfoOwner == viewerID {
		sb.WriteString(g.phoneInfoText() + "\n")
	}

	logStart := len(g.Log) - 4
	if logStart < 0 {
		logStart = 0
	}
	if len(g.Log) > 0 {
		sb.WriteString("━━━━━━━━━━━━━━━━━━━━\n")
		sb.WriteString("📋 Лог\n")
		for _, entry := range g.Log[logStart:] {
			sb.WriteString(fmt.Sprintf("• %s\n", entry.Message))
		}
	}

	return sb.String()
}

// IsPlayerTurn — проверка, ход ли игрока
func (g *Game) IsPlayerTurn(telegramID int64) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.CurrentPlayer().TelegramID == telegramID
}

// GetPlayerID — получить PlayerID по Telegram ID
func (g *Game) GetPlayerID(telegramID int64) (PlayerID, bool) {
	for i, p := range g.Players {
		if p != nil && p.TelegramID == telegramID {
			return PlayerID(i), true
		}
	}
	return 0, false
}

// GetWinner — получить победителя
func (g *Game) GetWinner() *Player {
	if g.Phase != PhaseGameOver {
		return nil
	}
	for _, p := range g.Players {
		if p != nil && p.IsAlive() {
			return p
		}
	}
	return nil
}
