package storage

import (
	"testing"

	"buckshot-roulette-bot/game"
)

func newPlayer(id int64, name string) *game.Player {
	return &game.Player{TelegramID: id, DisplayName: name}
}

// Отказ от реванша: соперник предложил реванш и ждёт в комнате, второй
// нажимает «Закончить играть». Комната должна закрыться, предложивший —
// освободиться из «в игре», а отказ запомниться (чтобы реванш не создался
// повторно «в никуда»).
func TestDeclineRematchClosesPendingRoom(t *testing.T) {
	s := NewStorage()

	// Старая закончившаяся игра
	old, err := s.CreateGame(111, newPlayer(1, "A"))
	if err != nil {
		t.Fatalf("create old: %v", err)
	}
	old.Phase = game.PhaseGameOver

	// A жмёт «Реванш»: создаётся новая комната, A ждёт в ней
	pending, err := s.CreateGame(222, newPlayer(1, "A"))
	if err != nil {
		t.Fatalf("create pending: %v", err)
	}
	s.SetRematch(old.ID, pending)

	// Обратный поиск: комната реванша привязана к старой игре
	if got := s.GetRematchByPending(pending.ID); got != old.ID {
		t.Fatalf("GetRematchByPending = %q, want %q", got, old.ID)
	}

	// B жмёт «Закончить играть»
	s.SetDeclinedRematch(old.ID, 2)
	s.RemoveGame(pending.ID)
	s.RemoveRematch(old.ID)

	if s.GetRematch(old.ID) != nil {
		t.Fatalf("rematch room still open after decline")
	}
	if s.GetDeclinedRematch(old.ID) != 2 {
		t.Fatalf("decline not recorded, got %d", s.GetDeclinedRematch(old.ID))
	}
	// A освобождён — больше не «в игре», может создать новую
	if g := s.GetGameByPlayer(1); g != nil {
		t.Fatalf("waiting player A still marked as in-game (game %s) after room closed", g.ID)
	}
	if got := s.GetRematchByPending(pending.ID); got != "" {
		t.Fatalf("GetRematchByPending still resolves to %q after close", got)
	}
}

// Отказ запоминается, даже если соперник ещё не предлагал реванш, —
// чтобы позднее нажатие «Реванш» не привело к вечному ожиданию.
func TestDeclinedRematchRecordedWithoutPending(t *testing.T) {
	s := NewStorage()
	s.SetDeclinedRematch("GAME", 2)

	if s.GetDeclinedRematch("GAME") != 2 {
		t.Fatalf("decline not recorded")
	}

	// Игрок передумал — отметку можно снять
	s.RemoveDeclinedRematch("GAME")
	if s.GetDeclinedRematch("GAME") != 0 {
		t.Fatalf("decline not cleared")
	}
}

// Реестр ников: запомнили @username -> ID, находим по нику в любом регистре
// и с/без «@». Неизвестный ник и пустой username в реестр не попадают.
func TestNickRegistry(t *testing.T) {
	s := NewStorage()

	// Пустой username не запоминаем
	s.RememberUser(1, "")
	s.RememberUser(2, "@")
	if _, ok := s.FindUserByNick(""); ok {
		t.Fatalf("empty nick should not resolve")
	}

	s.RememberUser(42, "@player_one")
	s.RememberUser(7, "player_two")

	if id, ok := s.FindUserByNick("player_one"); !ok || id != 42 {
		t.Fatalf("lookup player_one = %d/%v, want 42", id, ok)
	}
	if id, ok := s.FindUserByNick("@PLAYER_ONE"); !ok || id != 42 {
		t.Fatalf("case/@-insensitive lookup failed: %d/%v", id, ok)
	}
	if id, ok := s.FindUserByNick("player_two"); !ok || id != 7 {
		t.Fatalf("lookup player_two = %d/%v, want 7", id, ok)
	}

	// Кого не было — не находим
	if _, ok := s.FindUserByNick("ghost"); ok {
		t.Fatalf("unknown nick should not resolve")
	}
}

// Реестр ников переживает перезапуск: после RememberUser и нового
// NewStorage() на том же файле приглашение по нику должно работать.
func TestNickRegistryPersistsAcrossRestart(t *testing.T) {
	oldPath := dataFilePath
	dataFilePath = t.TempDir() + "/users.json"
	defer func() { dataFilePath = oldPath }()

	s1 := NewStorage()
	s1.RememberUser(42, "@player_one")

	// «Перезапуск»: новый экземпляр хранилища читает тот же файл
	s2 := NewStorage()
	if id, ok := s2.FindUserByNick("player_one"); !ok || id != 42 {
		t.Fatalf("nick lost after restart: id=%d ok=%v, want 42", id, ok)
	}
}

// Cleanup чистит отметки об отказе по играм, которых больше нет
func TestCleanupRemovesOrphanDeclines(t *testing.T) {
	s := NewStorage()

	live, err := s.CreateGame(333, newPlayer(7, "C"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	s.SetDeclinedRematch(live.ID, 8)
	s.SetDeclinedRematch("GHOST", 2)

	s.Cleanup()

	if s.GetDeclinedRematch("GHOST") != 0 {
		t.Fatalf("ghost decline not cleaned")
	}
	if s.GetDeclinedRematch(live.ID) != 8 {
		t.Fatalf("decline for existing game should stay")
	}
}
