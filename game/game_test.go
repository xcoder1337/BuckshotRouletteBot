package game

import (
	"strings"
	"testing"
)

func newTestGame() *Game {
	g := NewGame("TESTID", 100, &Player{TelegramID: 1, DisplayName: "A"})
	g.Join(&Player{TelegramID: 2, DisplayName: "B"})
	return g
}

// newStartedTestGame — начатая игра, ход за A (TelegramID 1).
// ItemCounter сбрасывается, чтобы setItems раздавал предсказуемые ID.
func newStartedTestGame() *Game {
	g := newTestGame()
	g.StartGame()
	g.CurrentTurn = Player1
	g.ItemCounter = 0
	return g
}

// setItems — заменить инвентарь игрока конкретными предметами с уникальными ID.
func setItems(g *Game, p PlayerID, types ...ItemType) {
	items := make([]Item, 0, len(types))
	for _, t := range types {
		g.ItemCounter++
		items = append(items, Item{Type: t, ID: g.ItemCounter})
	}
	g.Players[p].Items = items
}

// setMagazine — фиксированная обойма для детерминированных тестов.
func setMagazine(g *Game, shells ...ShellType) {
	g.Shotgun = &Shotgun{Magazine: shells}
}

// Старт: 2 жизни, 4 предмета, обойма 2-4 патрона — как в оригинале
func TestStartGameMechanics(t *testing.T) {
	g := newTestGame()
	g.StartGame()

	if g.Round != 1 {
		t.Fatalf("round = %d, want 1", g.Round)
	}
	for _, p := range g.Players {
		if p.HP != 2 || p.MaxHP != 2 {
			t.Fatalf("HP = %d/%d, want 2/2", p.HP, p.MaxHP)
		}
		if len(p.Items) != 4 {
			t.Fatalf("items = %d, want 4 (round 1)", len(p.Items))
		}
	}
	if g.Shotgun == nil || len(g.Shotgun.Magazine) < 2 || len(g.Shotgun.Magazine) > 4 {
		t.Fatalf("magazine size = %d, want 2-4", len(g.Shotgun.Magazine))
	}
}

// Новый раунд: +1 к жизни, полное лечение, +1 предмет
func TestNextRoundIncreasesLivesAndItems(t *testing.T) {
	g := newTestGame()
	g.StartGame()

	g.Round = 1
	g.nextRound() // теперь раунд 2

	if g.Round != 2 {
		t.Fatalf("round = %d, want 2", g.Round)
	}
	for _, p := range g.Players {
		if p.MaxHP != 3 {
			t.Fatalf("maxHP = %d, want 3 in round 2", p.MaxHP)
		}
		if p.HP != 3 {
			t.Fatalf("HP = %d, want healed to 3", p.HP)
		}
		if len(p.Items) != 5 {
			t.Fatalf("items = %d, want 5 in round 2", len(p.Items))
		}
	}
}

// Предметы не переносятся между раундами — каждый раунд свежий набор.
// «Неиспользованный» предмет из прошлого раунда обязан исчезнуть.
func TestItemsResetEachRound(t *testing.T) {
	g := newTestGame()
	g.StartGame()

	// «Остался неиспользованным» предмет из раунда 1
	g.Players[0].Items = []Item{{Type: ItemBeer, ID: 777}}

	g.Round = 2
	g.loadNewRound()

	for _, p := range g.Players {
		if len(p.Items) != 5 {
			t.Fatalf("items = %d, want exactly 5", len(p.Items))
		}
	}
	for _, it := range g.Players[0].Items {
		if it.ID == 777 {
			t.Fatalf("old item carried over to round 2")
		}
	}
}

// Количество предметов растёт 4/5/6/7/8 и упирается в 8
func TestItemsCountsPerRound(t *testing.T) {
	want := map[int]int{1: 4, 2: 5, 3: 6, 4: 7, 5: 8, 9: 8}

	for round, expected := range want {
		g := newTestGame()
		g.StartGame()
		g.Round = round
		g.loadNewRound()
		for _, p := range g.Players {
			if len(p.Items) != expected {
				t.Fatalf("round %d: items = %d, want %d", round, len(p.Items), expected)
			}
		}
	}
}

// Жизни растут до кап 5 и не больше
func TestLivesCapAtFive(t *testing.T) {
	g := newTestGame()
	g.StartGame()

	for i := 0; i < 6; i++ {
		g.Round = i + 1
		g.nextRound()
	}
	for _, p := range g.Players {
		if p.MaxHP > maxLives {
			t.Fatalf("maxHP = %d, cap is %d", p.MaxHP, maxLives)
		}
	}
}

// Состав патронов (🔴/🔵) НЕ должен светиться в статусе постоянно —
// в оригинале он показывается один раз в начале раунда (в логе) и скрывается
func TestShellBreakdownHiddenFromStatus(t *testing.T) {
	g := newTestGame()
	g.StartGame()
	text := g.GetStatusText(1)
	if contains(text, "🔴 +") {
		t.Fatalf("shell breakdown leaked into persistent status, got:\n%s", text)
	}
}

// С 3-го раунда, как в оригинале (финал: «провода перерезаны»), HP соперника
// скрыто: игрок видит свои жизни, а чужие — нет. До 3-го раунда видны оба.
func TestOpponentHPHiddenFromRound3(t *testing.T) {
	g := newTestGame()
	g.StartGame()
	g.RoundShells = 0 // убираем счётчик патронов, чтобы не сбивал подсчёт "/2"

	g.Round = 3
	text := g.GetStatusText(1) // вид игрока 1
	if !contains(text, "скрыт") {
		t.Fatalf("round 3: opponent HP must be hidden, got:\n%s", text)
	}
	// Своя жизнь (2/2) видна, чужая — нет: "/2" встречается ровно один раз
	if got := strings.Count(text, "/2"); got != 1 {
		t.Fatalf("round 3: only own HP (2/2) should be shown, got %d x \"/2\":\n%s", got, text)
	}

	g.Round = 1
	text = g.GetStatusText(1)
	if got := strings.Count(text, "/2"); got != 2 {
		t.Fatalf("round 1: both HP (2/2) should be visible, got %d x \"/2\":\n%s", got, text)
	}
}

// ─── Адреналин ───────────────────────────────────────────────────────────────

// Использование адреналина переводит игру в фазу кражи, ход не меняется,
// сам адреналин тратится
func TestAdrenalineSetsStealPhase(t *testing.T) {
	g := newStartedTestGame()
	setItems(g, Player1, ItemAdrenaline) // ID 1
	setItems(g, Player2, ItemCigarettes) // ID 2

	msg, err := g.UseItem(1, 1)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !contains(msg, "Выбери предмет") {
		t.Fatalf("msg = %q, want steal prompt", msg)
	}
	if g.Phase != PhaseAdrenaline {
		t.Fatalf("phase = %v, want PhaseAdrenaline", g.Phase)
	}
	if !g.AdrenalineActive {
		t.Fatal("AdrenalineActive = false, want true")
	}
	if g.CurrentTurn != Player1 {
		t.Fatalf("turn changed to %v, want Player1", g.CurrentTurn)
	}
	if len(g.Players[0].Items) != 0 {
		t.Fatalf("adrenaline should be consumed, items left: %v", g.Players[0].Items)
	}
}

// Кража переносит предмет из инвентаря соперника в свой, НЕ применяя эффект,
// и возвращает игру в обычную фазу
func TestAdrenalineStealTransfersItem(t *testing.T) {
	g := newStartedTestGame()
	setItems(g, Player1, ItemAdrenaline, ItemBeer) // ID 1, 2
	setItems(g, Player2, ItemCigarettes, ItemHandsaw) // ID 3, 4

	if _, err := g.UseItem(1, 1); err != nil {
		t.Fatalf("use adrenaline: %v", err)
	}

	msg, err := g.UseItem(1, 4) // крадём ножовку
	if err != nil {
		t.Fatalf("steal: %v", err)
	}
	if !contains(msg, "Ножовка") {
		t.Fatalf("msg = %q, want mention of Ножовка", msg)
	}
	if g.Phase != PhasePlaying {
		t.Fatalf("phase = %v, want PhasePlaying", g.Phase)
	}
	if g.AdrenalineActive {
		t.Fatal("AdrenalineActive = true after steal, want false")
	}
	if !g.Players[0].HasItem(ItemHandsaw) {
		t.Fatalf("stolen handsaw not in A inventory: %v", g.Players[0].Items)
	}
	if g.Players[1].HasItem(ItemHandsaw) {
		t.Fatalf("stolen handsaw still in B inventory: %v", g.Players[1].Items)
	}
	// Эффект краденого предмета НЕ применён сразу: ножовка не активна
	if g.SawedOff {
		t.Fatal("sawed off should not be active right after stealing it")
	}
}

// Нельзя украсть адреналин через адреналин
func TestCannotStealAdrenaline(t *testing.T) {
	g := newStartedTestGame()
	setItems(g, Player1, ItemAdrenaline)                // ID 1
	setItems(g, Player2, ItemCigarettes, ItemAdrenaline) // ID 2, 3

	if _, err := g.UseItem(1, 1); err != nil {
		t.Fatalf("use adrenaline: %v", err)
	}

	_, err := g.UseItem(1, 3) // пытаемся украсть чужой адреналин
	if err == nil {
		t.Fatal("stealing adrenaline should error")
	}
	if g.Phase != PhaseAdrenaline {
		t.Fatalf("phase = %v, want still PhaseAdrenaline", g.Phase)
	}
}

// Если у противника нечего красть — ошибка, и адреналин НЕ тратится
func TestAdrenalineNoOpponentItems(t *testing.T) {
	g := newStartedTestGame()
	setItems(g, Player1, ItemAdrenaline) // ID 1
	setItems(g, Player2)                 // пусто

	_, err := g.UseItem(1, 1)
	if err == nil {
		t.Fatal("should error when opponent has no items")
	}
	if g.Phase != PhasePlaying {
		t.Fatalf("phase = %v, want PhasePlaying", g.Phase)
	}
	if !g.Players[0].HasItem(ItemAdrenaline) {
		t.Fatal("adrenaline should not be consumed on failed use")
	}
}

// Кража несуществующего предмета — ошибка
func TestStealWrongItemID(t *testing.T) {
	g := newStartedTestGame()
	setItems(g, Player1, ItemAdrenaline) // ID 1
	setItems(g, Player2, ItemCigarettes) // ID 2

	if _, err := g.UseItem(1, 1); err != nil {
		t.Fatalf("use adrenaline: %v", err)
	}
	if _, err := g.UseItem(1, 999); err == nil {
		t.Fatal("stealing unknown item should error")
	}
}

// Если у противника ТОЛЬКО адреналин — красть нечего, использовать адреналин
// нельзя. Иначе игрок застрял бы в фазе кражи без единой кнопки выбора.
func TestAdrenalineNoStealableItems(t *testing.T) {
	g := newStartedTestGame()
	setItems(g, Player1, ItemAdrenaline)                 // ID 1
	setItems(g, Player2, ItemAdrenaline, ItemAdrenaline) // ID 2, 3

	_, err := g.UseItem(1, 1)
	if err == nil {
		t.Fatal("should error when opponent has only adrenaline items")
	}
	if g.Phase != PhasePlaying {
		t.Fatalf("phase = %v, want PhasePlaying", g.Phase)
	}
	if !g.Players[0].HasItem(ItemAdrenaline) {
		t.Fatal("adrenaline should not be consumed when there's nothing to steal")
	}
}

// Отмены в игре нет (как в шахматах): если эффект предмета невозможен, он НЕ
// тратится впустую. Сигареты при полном HP, наручники на уже скованном,
// телефон при одном патроне в стволе.
func TestItemNotConsumedWhenEffectImpossible(t *testing.T) {
	g := newStartedTestGame()

	// Сигареты при полном HP (2/2 после старта) — эффект невозможен
	setItems(g, Player1, ItemCigarettes) // ID 1
	if _, err := g.UseItem(1, 1); err == nil {
		t.Fatal("cigarettes at full HP should error")
	}
	if !g.Players[0].HasItem(ItemCigarettes) {
		t.Fatal("cigarettes should NOT be consumed when HP is full")
	}

	// Наручники, когда соперник уже в наручниках
	g.Players[0].Items = nil
	g.Players[1].Handcuffed = true
	setItems(g, Player1, ItemHandcuffs) // ID 2
	if _, err := g.UseItem(1, 2); err == nil {
		t.Fatal("handcuffs on already-cuffed opponent should error")
	}
	if !g.Players[0].HasItem(ItemHandcuffs) {
		t.Fatal("handcuffs should NOT be consumed when opponent is already cuffed")
	}

	// Телефон при одном патроне в стволе
	g.Players[0].Items = nil
	g.Players[1].Handcuffed = false
	setItems(g, Player1, ItemBurnerPhone) // ID 3
	setMagazine(g, ShellLive)
	if _, err := g.UseItem(1, 3); err == nil {
		t.Fatal("phone with a single shell should error")
	}
	if !g.Players[0].HasItem(ItemBurnerPhone) {
		t.Fatal("phone should NOT be consumed when only 1 shell is loaded")
	}
}

// ─── Статус: предметы обоих видны (как в оригинале — на столе) ─────────────

func TestStatusShowsBothPlayersItems(t *testing.T) {
	g := newStartedTestGame()
	setItems(g, Player1, ItemBeer, ItemCigarettes) // ID 1, 2
	setItems(g, Player2, ItemAdrenaline, ItemHandsaw) // ID 3, 4

	text := g.GetStatusText(1) // вид A

	// Свои предметы видны
	if !contains(text, "🍺") || !contains(text, "🚬") {
		t.Fatalf("own items should be visible, got:\n%s", text)
	}
	// Предметы соперника тоже видны — иначе не решить, что красть адреналином
	if !contains(text, "💉") || !contains(text, "🪚") {
		t.Fatalf("opponent items should be visible on the table, got:\n%s", text)
	}
}

// ─── Механики предметов и выстрелов ─────────────────────────────────────────

// Пиво извлекает текущий патрон
func TestBeerRacksShell(t *testing.T) {
	g := newStartedTestGame()
	setItems(g, Player1, ItemBeer) // ID 1
	setMagazine(g, ShellLive, ShellBlank)

	msg, err := g.UseItem(1, 1)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !contains(msg, "🍺") {
		t.Fatalf("msg = %q", msg)
	}
	if len(g.Shotgun.Magazine) != 1 {
		t.Fatalf("magazine size = %d, want 1", len(g.Shotgun.Magazine))
	}
	if g.Shotgun.CurrentShell() != ShellBlank {
		t.Fatalf("remaining shell = %v, want blank", g.Shotgun.CurrentShell())
	}
}

// Инвертер НЕ раскрывает результат: в сообщении нет типа патрона. В оригинале
// ты знаешь только то, что патрон в стволе сменился на противоположный.
func TestInverterDoesNotRevealShell(t *testing.T) {
	g := newStartedTestGame()
	setItems(g, Player1, ItemInverter) // ID 1
	setMagazine(g, ShellLive, ShellBlank)

	msg, err := g.UseItem(1, 1)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if contains(msg, "Теперь в стволе") || contains(msg, "Боевой") || contains(msg, "Холостой") {
		t.Fatalf("inverter revealed the shell: %q", msg)
	}
	if !contains(msg, "🔄") {
		t.Fatalf("msg = %q", msg)
	}
	// Патрон реально перевернулся
	if g.Shotgun.CurrentShell() != ShellBlank {
		t.Fatalf("current shell = %v, want inverted to blank", g.Shotgun.CurrentShell())
	}
}

// Результат лупы виден только использовавшему: соперник не должен видеть
// «В стволе: …» в своём статусе.
func TestMagnifierInfoNotLeakedToOpponent(t *testing.T) {
	g := newStartedTestGame()
	setItems(g, Player1, ItemMagnifyingGlass) // ID 1
	setMagazine(g, ShellLive, ShellBlank)

	if _, err := g.UseItem(1, 1); err != nil {
		t.Fatalf("err: %v", err)
	}

	if !contains(g.GetStatusText(1), "🔍 В стволе") {
		t.Fatalf("owner should see magnifier info:\n%s", g.GetStatusText(1))
	}
	if contains(g.GetStatusText(2), "🔍 В стволе") {
		t.Fatalf("opponent should NOT see magnifier info:\n%s", g.GetStatusText(2))
	}
}

// Результат телефона виден только использовавшему.
func TestPhoneInfoNotLeakedToOpponent(t *testing.T) {
	g := newStartedTestGame()
	setItems(g, Player1, ItemBurnerPhone) // ID 1
	setMagazine(g, ShellLive, ShellBlank, ShellBlank)

	if _, err := g.UseItem(1, 1); err != nil {
		t.Fatalf("err: %v", err)
	}

	if !contains(g.GetStatusText(1), "📱 Патрон #") {
		t.Fatalf("owner should see phone info:\n%s", g.GetStatusText(1))
	}
	if contains(g.GetStatusText(2), "📱 Патрон #") {
		t.Fatalf("opponent should NOT see phone info:\n%s", g.GetStatusText(2))
	}
}

// Сигареты лечат +1 HP
func TestCigarettesHeal(t *testing.T) {
	g := newStartedTestGame()
	setItems(g, Player1, ItemCigarettes) // ID 1
	g.Players[0].HP = 1

	if _, err := g.UseItem(1, 1); err != nil {
		t.Fatalf("err: %v", err)
	}
	if g.Players[0].HP != 2 {
		t.Fatalf("HP = %d, want 2", g.Players[0].HP)
	}
}

// Ножовка удваивает урон следующего выстрела
func TestSawedOffDoublesDamage(t *testing.T) {
	g := newStartedTestGame()
	setItems(g, Player1, ItemHandsaw) // ID 1
	setMagazine(g, ShellLive)

	if _, err := g.UseItem(1, 1); err != nil {
		t.Fatalf("err: %v", err)
	}
	msg, err := g.Shoot(1, false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if g.Players[1].HP != 0 {
		t.Fatalf("opponent HP = %d, want 0", g.Players[1].HP)
	}
	if !contains(msg, "-2❤️") {
		t.Fatalf("msg = %q, want -2 damage", msg)
	}
	if g.Phase != PhaseGameOver {
		t.Fatalf("phase = %v, want PhaseGameOver", g.Phase)
	}
}

// Холостой в себя — ходишь ещё раз
func TestShootSelfBlankKeepsTurn(t *testing.T) {
	g := newStartedTestGame()
	setMagazine(g, ShellBlank, ShellLive)

	if _, err := g.Shoot(1, true); err != nil {
		t.Fatalf("err: %v", err)
	}
	if g.CurrentTurn != Player1 {
		t.Fatalf("turn = %v, want Player1 (extra turn)", g.CurrentTurn)
	}
	if g.Players[0].HP != 2 {
		t.Fatalf("HP = %d, want 2", g.Players[0].HP)
	}
}

// Боевой в себя — урон и переход хода
func TestShootSelfLiveDamagesSelf(t *testing.T) {
	g := newStartedTestGame()
	setMagazine(g, ShellLive, ShellBlank)

	if _, err := g.Shoot(1, true); err != nil {
		t.Fatalf("err: %v", err)
	}
	if g.Players[0].HP != 1 {
		t.Fatalf("HP = %d, want 1", g.Players[0].HP)
	}
	if g.CurrentTurn != Player2 {
		t.Fatalf("turn = %v, want Player2", g.CurrentTurn)
	}
}

// Наручники — противник пропускает следующий ход
func TestHandcuffsSkipTurn(t *testing.T) {
	g := newStartedTestGame()
	setItems(g, Player1, ItemHandcuffs) // ID 1

	if _, err := g.UseItem(1, 1); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !g.Players[1].Handcuffed {
		t.Fatal("opponent not handcuffed")
	}

	setMagazine(g, ShellBlank, ShellLive)
	if _, err := g.Shoot(1, false); err != nil {
		t.Fatalf("err: %v", err)
	}
	// Ход должен был перейти к B, но B в наручниках — снова ход A
	if g.CurrentTurn != Player1 {
		t.Fatalf("turn = %v, want Player1 (handcuffed opponent skips)", g.CurrentTurn)
	}
	if g.Players[1].Handcuffed {
		t.Fatal("handcuffs should be consumed after skipping")
	}
}

// Нельзя использовать предмет не в свой ход
func TestUseItemNotYourTurn(t *testing.T) {
	g := newStartedTestGame()
	setItems(g, Player2, ItemCigarettes) // предмет B

	if _, err := g.UseItem(2, 1); err == nil {
		t.Fatal("using item out of turn should error")
	}
}

// ─── Раунды и рандомизация патронов ─────────────────────────────────────────

// Когда обойма опустела без чьей-либо смерти — начинается новый раунд:
// +1 к максимуму жизней, полное лечение, новая обойма (3-6 на раунде 2)
func TestEmptyMagazineAdvancesRound(t *testing.T) {
	g := newStartedTestGame()
	setMagazine(g, ShellLive) // один патрон — выстрел опустошит обойму

	if _, err := g.Shoot(1, false); err != nil {
		t.Fatalf("err: %v", err)
	}
	if g.Round != 2 {
		t.Fatalf("round = %d, want 2", g.Round)
	}
	if g.Players[0].MaxHP != 3 {
		t.Fatalf("maxHP = %d, want 3", g.Players[0].MaxHP)
	}
	if g.Players[0].HP != 3 {
		t.Fatalf("HP = %d, want full heal to 3", g.Players[0].HP)
	}
	if n := len(g.Shotgun.Magazine); n < 3 || n > 6 {
		t.Fatalf("round 2 magazine size = %d, want 3-6", n)
	}
}

// Диапазон размера обоймы по раундам — как в оригинале: 2-4 / 3-6 / 4-8
func TestShellCountsPerRoundRange(t *testing.T) {
	want := map[int][2]int{1: {2, 4}, 2: {3, 6}, 3: {4, 8}, 5: {4, 8}}

	for round, rng := range want {
		for i := 0; i < 100; i++ {
			g := newTestGame()
			g.Round = round
			g.loadNewRound()
			n := len(g.Shotgun.Magazine)
			if n < rng[0] || n > rng[1] {
				t.Fatalf("round %d: magazine = %d, want %d-%d", round, n, rng[0], rng[1])
			}
		}
	}
}

// Нейтральный счётчик патронов в статусе: показывает остаток из всех
// заряженных, но НЕ раскрывает типы (🔴/🔵). Раскрытие типов в начале
// раунда — это отдельный «намёк запомнить» из лога (как поддон на компе,
// который показывают один раз и закрывают); сам счётчик обязан быть нейтральным.
func TestShellCounterShowsRemaining(t *testing.T) {
	g := newStartedTestGame()
	g.RoundShells = 5
	g.Shotgun = &Shotgun{Magazine: []ShellType{ShellLive, ShellBlank, ShellLive}}

	text := g.GetStatusText(1)
	counter := counterLine(text)
	if !contains(counter, "🔫 Патроны: 🔋🔋🔋🪫🪫  (3/5)") {
		t.Fatalf("counter line wrong, got:\n%s", text)
	}
	// Сама строка счётчика НЕ должна раскрывать типы патронов
	if contains(counter, "🔴") || contains(counter, "🔵") {
		t.Fatalf("shell types leaked into counter line, got:\n%s", counter)
	}

	// После выстрела остаток уменьшается
	g.Shotgun = &Shotgun{Magazine: []ShellType{ShellLive, ShellBlank}}
	text = g.GetStatusText(1)
	counter = counterLine(text)
	if !contains(counter, "(2/5)") {
		t.Fatalf("counter did not decrement, got:\n%s", text)
	}
}

// counterLine — строка «🔫 Патроны: ...» из статуса
func counterLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "🔫 Патроны:") {
			return line
		}
	}
	return ""
}

// Игра проходит через несколько раундов: 1 → 2 → 3, жизни растут
func TestRoundsProgressThroughThree(t *testing.T) {
	g := newStartedTestGame()

	// Дважды опустошаем обойму (холостой в себя — безопасно и не меняет ход),
	// каждый раз должен запускаться следующий раунд. Монетка в начале раунда
	// могла отдать ход игроку 2 — принудительно возвращаем ход игроку 1.
	for wantRound := 2; wantRound <= 3; wantRound++ {
		g.Shotgun = &Shotgun{Magazine: []ShellType{ShellBlank}}
		g.CurrentTurn = Player1
		if _, err := g.Shoot(1, true); err != nil {
			t.Fatalf("err: %v", err)
		}
		if g.Round != wantRound {
			t.Fatalf("round = %d, want %d", g.Round, wantRound)
		}
	}

	// На раунде 3 максимум жизней = 4 (старт 2, +1 за каждый раунд)
	if g.Players[0].MaxHP != 4 {
		t.Fatalf("round 3 maxHP = %d, want 4", g.Players[0].MaxHP)
	}
	if g.Players[0].HP != 4 {
		t.Fatalf("round 3 HP = %d, want full heal to 4", g.Players[0].HP)
	}
}

// Монетка в начале каждого раунда: первый ход разыгрывается заново,
// а не переходит «по наследству» (как в оригинале). Хост/первый игрок
// не должен сохранять преимущество между раундами.
func TestCoinFlipEachRound(t *testing.T) {
	seen := map[PlayerID]bool{}
	for i := 0; i < 50; i++ {
		g := newStartedTestGame()
		g.CurrentTurn = Player1
		setMagazine(g, ShellBlank) // холостой в себя — обойма опустошается без урона

		if _, err := g.Shoot(1, true); err != nil {
			t.Fatalf("err: %v", err)
		}
		if g.Round != 2 {
			t.Fatalf("round = %d, want 2", g.Round)
		}
		seen[g.CurrentTurn] = true
	}
	// За 50 перебросов обязаны встретиться оба игрока — иначе ход «залипает»
	if !seen[Player1] || !seen[Player2] {
		t.Fatalf("coin flip not random, saw only: %v", seen)
	}
}

// Монетка в первом раунде тоже случайна и сообщается в логе
func TestCoinFlipLoggedAtStart(t *testing.T) {
	g := newStartedTestGame()
	found := false
	for _, entry := range g.Log {
		if strings.Contains(entry.Message, "🎲 Монетка:") {
			found = true
		}
	}
	if !found {
		t.Fatalf("coin flip not announced at game start, log:\n%+v", g.Log)
	}
}

// Позиции патронов перемешиваются: и 🔴, и 🔵 бывают первыми в стволе
func TestShellArrangementIsShuffled(t *testing.T) {
	firstLive, firstBlank := false, false
	for i := 0; i < 200; i++ {
		s := NewShotgun(2, 4)
		if s.CurrentShell() == ShellLive {
			firstLive = true
		} else {
			firstBlank = true
		}
		if firstLive && firstBlank {
			return // обе позиции встречались — перемешивание работает
		}
	}
	t.Fatal("first shell was always the same type over 200 runs — no shuffle")
}

// Инвариант дробовика: всегда минимум 1 боевой и 1 холостой
func TestShotgunInvariant(t *testing.T) {
	for i := 0; i < 200; i++ {
		s := NewShotgun(2, 4)
		if s.LiveCount() < 1 || s.BlankCount() < 1 {
			t.Fatalf("bad magazine: live=%d blank=%d", s.LiveCount(), s.BlankCount())
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// Пиво извлекает текущий патрон: знание лупы о нём сгорает. Раньше KnownShell
// «зависал» и игрок действовал по информации о патроне, которого уже нет.
func TestBeerClearsKnownShell(t *testing.T) {
	g := newStartedTestGame()
	setItems(g, Player1, ItemMagnifyingGlass, ItemBeer) // ID 1, 2
	setMagazine(g, ShellLive, ShellBlank, ShellLive)

	if _, err := g.UseItem(1, 1); err != nil {
		t.Fatalf("magnifier: %v", err)
	}
	if g.KnownShell == nil {
		t.Fatal("magnifier should set KnownShell")
	}
	if _, err := g.UseItem(1, 2); err != nil {
		t.Fatalf("beer: %v", err)
	}
	if g.KnownShell != nil {
		t.Fatal("beer must clear KnownShell — the observed shell was extracted")
	}
}

// Пиво сдвигает раскрытый телефоном патрон на позицию ближе к стволу,
// а не сжигает информацию целиком (телефон показал будущий патрон, не текущий).
func TestPhoneShiftsAfterBeer(t *testing.T) {
	g := newStartedTestGame()
	setItems(g, Player1, ItemBurnerPhone, ItemBeer) // ID 1, 2
	setMagazine(g, ShellLive, ShellBlank, ShellLive, ShellBlank)

	if _, err := g.UseItem(1, 1); err != nil {
		t.Fatalf("phone: %v", err)
	}
	if g.PhoneShell == nil || g.PhoneShellPos <= 1 {
		t.Fatalf("phone should reveal a future shell, got pos %d", g.PhoneShellPos)
	}
	before := g.PhoneShellPos

	if _, err := g.UseItem(1, 2); err != nil {
		t.Fatalf("beer: %v", err)
	}
	if g.PhoneShell == nil {
		t.Fatal("phone info should survive beer")
	}
	if g.PhoneShellPos != before-1 {
		t.Fatalf("phone position = %d, want %d (shifted by beer)", g.PhoneShellPos, before-1)
	}
}

// Ножовка не тратится впустую при повторном применении: ствол уже укорочен.
func TestHandsawNoDoubleUse(t *testing.T) {
	g := newStartedTestGame()
	setItems(g, Player1, ItemHandsaw, ItemHandsaw) // ID 1, 2
	setMagazine(g, ShellLive, ShellBlank)

	if _, err := g.UseItem(1, 1); err != nil {
		t.Fatalf("first handsaw: %v", err)
	}
	if _, err := g.UseItem(1, 2); err == nil {
		t.Fatal("second handsaw while already sawed off should error")
	}
	// Предмет НЕ должен быть потрачен: в инвентаре всё ещё одна ножовка (ID 2).
	if len(g.Players[0].Items) != 1 {
		t.Fatalf("items = %d, want 1 (failed use must not consume)", len(g.Players[0].Items))
	}
}

// Опустевшая обойма → новый раунд, и первый ход СТРОГО по монетке. Раньше
// выстрел вызывал nextTurn() после монетки, и ходил не тот игрок, что в логе.
func TestNewRoundTurnMatchesCoinFlipLog(t *testing.T) {
	g := newStartedTestGame()
	setMagazine(g, ShellLive) // один патрон

	if _, err := g.Shoot(1, false); err != nil {
		t.Fatalf("shoot: %v", err)
	}
	if g.Round != 2 {
		t.Fatalf("round = %d, want 2", g.Round)
	}

	want := ""
	for _, e := range g.Log {
		if strings.HasPrefix(e.Message, "🎲 Монетка:") {
			want = strings.TrimSuffix(strings.TrimPrefix(e.Message, "🎲 Монетка: "), " ходит первым!")
		}
	}
	if want == "" {
		t.Fatal("coin flip log not found")
	}
	if got := g.CurrentPlayer().DisplayName; got != want {
		t.Fatalf("current player = %q, coin flip says %q (nextTurn flipped the turn)", got, want)
	}
}
