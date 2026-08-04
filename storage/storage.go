package storage

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"buckshot-roulette-bot/game"
)

// dataFilePath — файл, куда сохраняется долгоживущее состояние (реестр ников).
// Простая «локальная база»: JSON на диске переживает перезапуск бота.
// Переопределяется в тестах на временный файл.
var dataFilePath = "buckshot_users.json"

// absDataPath — абсолютный путь к файлу состояния (для логов). Сам файл лежит
// относительно текущей рабочей папки: если бот запущен из другой папки, он
// молча читает/пишет другой файл. Абсолютный путь в логе сразу это покажет.
func absDataPath() string {
	if filepath.IsAbs(dataFilePath) {
		return dataFilePath
	}
	if dir, err := os.Getwd(); err == nil {
		return filepath.Join(dir, dataFilePath)
	}
	return dataFilePath
}

// persistedData — то, что храним на диске.
type persistedData struct {
	UserNicks map[string]int64 `json:"user_nicks"`
}

// Storage — хранилище игр
type Storage struct {
	mu    sync.RWMutex
	games map[string]*game.Game

	// Маппинг chatID -> gameID для активных игр
	activeGames map[int64]string

	// Маппинг telegramID -> gameID для поиска игры игрока
	playerGames map[int64]string

	// Ожидающие реванши: ID закончившейся игры -> новая комната, в которой
	// уже сидит первый игрок и ждёт соперника
	rematchGames map[string]*game.Game

	// Отказы от реванша: ID закончившейся игры -> TelegramID игрока,
	// который завершил игру. Нужно, чтобы предложившему реванш не висеть
	// в ожидании, когда соперник уже ушёл.
	declinedRematch map[string]int64

	// Реестр ников: @username (в нижнем регистре) -> TelegramID.
	// Бот не может разрешить username -> ID для приватных пользователей
	// через Bot API, поэтому запоминаем всех, кто хоть раз взаимодействовал.
	// Это позволяет приглашать в комнату по @нику (с уведомлением в личку).
	userNicks map[string]int64
}

// NewStorage — создание хранилища
func NewStorage() *Storage {
	s := &Storage{
		games:           make(map[string]*game.Game),
		activeGames:     make(map[int64]string),
		playerGames:     make(map[int64]string),
		rematchGames:    make(map[string]*game.Game),
		declinedRematch: make(map[string]int64),
		userNicks:       make(map[string]int64),
	}
	s.loadUsers()
	return s
}

// loadUsers — загрузка реестра ников из файла при старте. Если файла нет —
// просто пустой реестр (никто ещё не регистрировался). Ошибки не фатальны:
// бот работает с пустым реестром, дальше он наполнится заново.
func (s *Storage) loadUsers() {
	data, err := os.ReadFile(dataFilePath)
	if err != nil {
		log.Printf("📭 Реестр ников пуст: файла %s ещё нет (первый запуск?)", absDataPath())
		return
	}
	var pd persistedData
	if err := json.Unmarshal(data, &pd); err != nil {
		log.Printf("⚠️ Не удалось прочитать %s: %v", dataFilePath, err)
		return
	}
	for nick, id := range pd.UserNicks {
		s.userNicks[nick] = id
	}
	// Логируем АБСОЛЮТНЫЙ путь — иначе при запуске из другой папки бот молча
	// читает/пишет другой файл и кажется, что состояние «потерялось».
	log.Printf("📥 Загружено %d пользователей из %s", len(s.userNicks), absDataPath())
}

// persistUsers — сохраняет реестр ников в файл (атомарно: через временный
// файл + переименование, чтобы перезапуск посреди записи не оставил мусор).
// Вызывается при изменении реестра, пока удерживаем lock.
func (s *Storage) persistUsers() {
	pd := persistedData{UserNicks: s.userNicks}
	data, err := json.Marshal(pd)
	if err != nil {
		log.Printf("⚠️ Ошибка сериализации реестра: %v", err)
		return
	}

	tmp := dataFilePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		log.Printf("⚠️ Не удалось сохранить %s: %v", dataFilePath, err)
		return
	}
	if err := os.Rename(tmp, dataFilePath); err != nil {
		log.Printf("⚠️ Не удалось переименовать %s: %v", dataFilePath, err)
	}
}

// RememberUser — запоминаем привязку @username -> TelegramID для всех,
// кто взаимодействует с ботом (сообщения и нажатия кнопок).
func (s *Storage) RememberUser(telegramID int64, username string) {
	username = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(username)), "@")
	if username == "" || telegramID == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// Пишем на диск только при реальном изменении (новый юзер или сменил ник),
	// чтобы не дёргать файл на каждое сообщение.
	if s.userNicks[username] == telegramID {
		return
	}
	s.userNicks[username] = telegramID
	s.persistUsers()
}

// FindUserByNick — поиск TelegramID по @нику (регистр не важен, @ не нужен).
// ok=false означает, что пользователь никогда не запускал бота.
func (s *Storage) FindUserByNick(nick string) (int64, bool) {
	nick = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(nick)), "@")
	if nick == "" {
		return 0, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.userNicks[nick]
	return id, ok
}

// GenerateID — генерация уникального ID
func (s *Storage) GenerateID() string {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

// CreateGame — создание игры
func (s *Storage) CreateGame(chatID int64, creator *game.Player) (*game.Game, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Проверяем, не играет ли уже игрок
	if gameID, exists := s.playerGames[creator.TelegramID]; exists {
		if g, ok := s.games[gameID]; ok && g.Phase != game.PhaseGameOver {
			// Подсказываем «особо тугим», как выйти из комнаты, а не просто
			// бросаем «вы уже в игре» — иначе человек застревает.
			if g.Phase == game.PhaseWaiting {
				return nil, fmt.Errorf("вы уже в комнате %s — ждём второго игрока. Нажмите ❌ Отменить, чтобы выйти из комнаты", gameID)
			}
			return nil, fmt.Errorf("вы уже играете в игре %s — она ещё не закончилась", gameID)
		}
	}

	id := s.GenerateID()
	for {
		if _, exists := s.games[id]; !exists {
			break
		}
		id = s.GenerateID()
	}

	g := game.NewGame(id, chatID, creator)
	s.games[id] = g
	s.activeGames[chatID] = id
	s.playerGames[creator.TelegramID] = id

	return g, nil
}

// JoinGame — присоединение к игре
func (s *Storage) JoinGame(gameID string, player *game.Player) (*game.Game, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	g, exists := s.games[gameID]
	if !exists {
		return nil, fmt.Errorf("игра %s не найдена", gameID)
	}

	// Проверяем, не играет ли уже игрок
	if existingGameID, exists := s.playerGames[player.TelegramID]; exists {
		if existingGame, ok := s.games[existingGameID]; ok && existingGame.Phase != game.PhaseGameOver {
			if existingGame.Phase == game.PhaseWaiting {
				return nil, fmt.Errorf("вы уже в комнате %s — сначала выйдите из неё (кнопка ❌ Отменить)", existingGameID)
			}
			return nil, fmt.Errorf("вы уже играете в игре %s — она ещё не закончилась", existingGameID)
		}
	}

	if err := g.Join(player); err != nil {
		return nil, err
	}

	s.playerGames[player.TelegramID] = gameID

	return g, nil
}

// GetGame — получение игры по ID
func (s *Storage) GetGame(gameID string) *game.Game {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.games[gameID]
}

// GetGameByChat — получение активной игры чата
func (s *Storage) GetGameByChat(chatID int64) *game.Game {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if gameID, exists := s.activeGames[chatID]; exists {
		return s.games[gameID]
	}
	return nil
}

// GetGameByPlayer — получение игры по ID игрока
func (s *Storage) GetGameByPlayer(telegramID int64) *game.Game {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if gameID, exists := s.playerGames[telegramID]; exists {
		g := s.games[gameID]
		if g != nil && g.Phase != game.PhaseGameOver {
			return g
		}
	}
	return nil
}

// RemoveGame — удаление игры
func (s *Storage) RemoveGame(gameID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	g, exists := s.games[gameID]
	if !exists {
		return
	}

	delete(s.activeGames, g.ChatID)
	for _, p := range g.Players {
		if p != nil {
			delete(s.playerGames, p.TelegramID)
		}
	}
	delete(s.games, gameID)
}

// SetRematch — поставить новую комнату реванша в ожидание по ID старой игры
func (s *Storage) SetRematch(oldGameID string, g *game.Game) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rematchGames[oldGameID] = g
}

// GetRematch — получить ожидающую комнату реванша
func (s *Storage) GetRematch(oldGameID string) *game.Game {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rematchGames[oldGameID]
}

// RemoveRematch — убрать комнату реванша
func (s *Storage) RemoveRematch(oldGameID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rematchGames, oldGameID)
}

// SetDeclinedRematch — отметить, что игрок завершил игру и отказался от реванша
func (s *Storage) SetDeclinedRematch(oldGameID string, telegramID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.declinedRematch[oldGameID] = telegramID
}

// GetDeclinedRematch — TelegramID игрока, отказавшегося от реванша (0 если никто)
func (s *Storage) GetDeclinedRematch(oldGameID string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.declinedRematch[oldGameID]
}

// RemoveDeclinedRematch — снять отметку об отказе (например, игрок передумал)
func (s *Storage) RemoveDeclinedRematch(oldGameID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.declinedRematch, oldGameID)
}

// GetRematchByPending — oldGameID по ID ожидающей комнаты реванша
func (s *Storage) GetRematchByPending(pendingID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for oldID, g := range s.rematchGames {
		if g != nil && g.ID == pendingID {
			return oldID
		}
	}
	return ""
}

// Cleanup — очистка старых игр
func (s *Storage) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for id, g := range s.games {
		if now.Sub(g.UpdatedAt) > 30*time.Minute {
			delete(s.activeGames, g.ChatID)
			for _, p := range g.Players {
				if p != nil {
					delete(s.playerGames, p.TelegramID)
				}
			}
			delete(s.games, id)
		}
	}

	// Убираем осиротевшие реванши (комната исчезла или устарела)
	for oldID, g := range s.rematchGames {
		if _, exists := s.games[g.ID]; !exists {
			delete(s.rematchGames, oldID)
		}
	}

	// Очищаем отметки об отказе от реванша по играм, которых больше нет
	for oldID := range s.declinedRematch {
		if _, exists := s.games[oldID]; !exists {
			delete(s.declinedRematch, oldID)
		}
	}
}
