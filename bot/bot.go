package bot

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"buckshot-roulette-bot/game"
	"buckshot-roulette-bot/storage"
	"buckshot-roulette-bot/ui"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"golang.org/x/net/proxy"
)

// botVersion — версия бота (не игры!). Это ПЕРВАЯ стабильная версия (major = 1),
// число правок — 17 итераций «почини/доделай + рестарт», которые мы с тобой прошли.
// Пользователям показываем только саму версию, без счётчика итераций.
const botVersion = "1.17"

// Bot — телеграм бот
type Bot struct {
	api     *tgbotapi.BotAPI
	storage *storage.Storage

	// Состояния пользователей для ввода кода
	waitingForCode map[int64]bool

	// Состояния пользователей для ввода @ника приглашаемого
	waitingForInvite map[int64]string
}

// httpClient — HTTP-клиент, настроенный на прокси из переменной TELEGRAM_PROXY.
// Поддерживаются форматы: http://host:port и socks5://host:port.
// Прокси подключается ДО запроса авторизации (getMe), потому что иначе
// первый же запрос к api.telegram.org упадёт в заблокированной сети.
func httpClient() *http.Client {
	transport := &http.Transport{
		Proxy:           nil,
		DialContext:     (&net.Dialer{Timeout: 15 * time.Second}).DialContext,
		MaxIdleConns:    10,
		IdleConnTimeout: 60 * time.Second,
	}

	if proxyURL := os.Getenv("TELEGRAM_PROXY"); proxyURL != "" {
		u, err := url.Parse(proxyURL)
		if err == nil {
			switch u.Scheme {
			case "socks5", "socks5h":
				if d, derr := proxy.SOCKS5("tcp", u.Host, nil, proxy.Direct); derr == nil {
					transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
						return d.Dial(network, addr)
					}
					log.Printf("🌐 SOCKS5 прокси: %s", u.Host)
				}
			case "http", "https":
				transport.Proxy = http.ProxyURL(u)
				log.Printf("🌐 HTTP прокси: %s", u.Host)
			default:
				log.Printf("⚠️ Неизвестный тип прокси %q (нужно http:// или socks5://)", u.Scheme)
			}
		} else {
			log.Printf("⚠️ Не удалось разобрать TELEGRAM_PROXY=%s: %v", proxyURL, err)
		}
	}

	return &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
	}
}

// New — создание бота
func New(token string) (*Bot, error) {
	// APIEndpoint — это формат-строка "https://api.telegram.org/bot%s/%s",
	// которую библиотека подставляет в fmt.Sprintf(token, endpoint).
	api, err := tgbotapi.NewBotAPIWithClient(token, tgbotapi.APIEndpoint, httpClient())
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	api.Debug = false
	log.Printf("Authorized on account %s", api.Self.UserName)
	log.Printf("🏷️ Buckshot Roulette Bot v%s", botVersion)

	b := &Bot{
		api:              api,
		storage:          storage.NewStorage(),
		waitingForCode:   make(map[int64]bool),
		waitingForInvite: make(map[int64]string),
	}
	b.setupCommands()

	return b, nil
}

// setupCommands — регистрирует команды бота (setMyCommands). Благодаря этому
// в каждом чате у пользователя появляется постоянная кнопка «Меню» внизу слева:
// не нужно каждый раз писать /start, достаточно нажать кнопку.
func (b *Bot) setupCommands() {
	config := tgbotapi.SetMyCommandsConfig{
		Commands: []tgbotapi.BotCommand{
			{Command: "start", Description: "🏠 Главное меню"},
			{Command: "new", Description: "🎮 Создать игру"},
			{Command: "join", Description: "🔗 Присоединиться по коду"},
			{Command: "status", Description: "📊 Статус моей игры"},
			{Command: "rules", Description: "📖 Правила"},
			{Command: "version", Description: "🏷️ Версия бота"},
		},
	}
	if _, err := b.api.Request(config); err != nil {
		log.Printf("⚠️ Не удалось установить команды бота: %v", err)
	} else {
		log.Printf("✅ Команды бота установлены (кнопка «Меню» в чатах)")
	}
}

// Run — запуск бота
func (b *Bot) Run() {
	// Очистка старых игр
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			b.storage.Cleanup()
		}
	}()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			b.handleMessage(update.Message)
		}
		if update.CallbackQuery != nil {
			b.handleCallback(update.CallbackQuery)
		}
	}
}

// getDisplayName — получение имени пользователя
func getDisplayName(user *tgbotapi.User) string {
	if user.UserName != "" {
		return "@" + user.UserName
	}
	name := user.FirstName
	if user.LastName != "" {
		name += " " + user.LastName
	}
	return name
}

// rememberUser — запоминаем @username -> TelegramID для всех, кто
// взаимодействует с ботом. Bot API не умеет находить приватных
// пользователей по нику, поэтому без этого реестра нельзя будет
// пригласить человека по @нику (см. storage.RememberUser).
func (b *Bot) rememberUser(user *tgbotapi.User) {
	if user == nil {
		return
	}
	b.storage.RememberUser(user.ID, user.UserName)
}

// handleMessage — обработка сообщений
func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	// Запоминаем ники всех, кто пишет боту — без этого нельзя будет
	// пригласить человека по @нику (см. RememberUser).
	b.rememberUser(msg.From)

	// Команда (с «/») всегда выводит из состояния ожидания ввода. Иначе
	// человек «застревает»: например, /new посреди ввода кода трактуется
	// как код комнаты, и бот отвечает «❌ игра /NEW не найдена».
	if msg.Command() != "" {
		delete(b.waitingForCode, msg.From.ID)
		delete(b.waitingForInvite, msg.From.ID)
	}

	// Проверяем, ожидаем ли код
	if b.waitingForCode[msg.From.ID] {
		b.handleJoinByCode(msg)
		return
	}

	// Проверяем, ожидаем ли @ник приглашаемого
	if gameID, ok := b.waitingForInvite[msg.From.ID]; ok {
		delete(b.waitingForInvite, msg.From.ID)
		b.handleInviteByNick(msg, gameID)
		return
	}

	switch msg.Command() {
	case "start":
		b.handleStart(msg)
	case "new", "newgame", "create":
		b.handleNewGame(msg)
	case "join":
		b.handleJoinCommand(msg)
	case "status":
		b.handleStatus(msg)
	case "cancel":
		b.handleCancel(msg)
	case "menu", "main":
		b.sendMainMenu(msg)
	case "rules", "help":
		b.handleRules(msg)
	case "version":
		b.handleVersion(msg)
	default:
		if msg.Text != "" && !strings.HasPrefix(msg.Text, "/") {
			// Может быть код игры
			code := strings.TrimSpace(strings.ToUpper(msg.Text))
			if len(code) == 6 {
				g := b.storage.GetGame(code)
				if g != nil {
					b.joinGameDirect(msg, code)
					return
				}
			}

			// Любое другое сообщение без команды и без активной игры —
			// показываем меню кнопками, чтобы не приходилось писать /start.
			// Во время игры на текст не реагируем: игроки управляют кнопками.
			if b.storage.GetGameByPlayer(msg.From.ID) == nil {
				b.sendMainMenu(msg)
			}
		}
	}
}

// handleStart — /start
func (b *Bot) handleStart(msg *tgbotapi.Message) {
	// Проверяем deep link
	if msg.CommandArguments() != "" {
		arg := msg.CommandArguments()
		if strings.HasPrefix(arg, "join_") {
			gameID := strings.TrimPrefix(arg, "join_")
			b.joinGameDirect(msg, gameID)
			return
		}
	}

	b.sendMainMenu(msg)
}

// sendMainMenu — приветствие с главным меню (кнопками).
// Вызывается из /start, из кнопки «Меню» и на любое не-командное сообщение,
// чтобы новичку не пришлось разбираться с командами.
func (b *Bot) sendMainMenu(msg *tgbotapi.Message) {
	text := fmt.Sprintf(`
🎯 *BUCKSHOT ROULETTE*
══════════════════════════

Сборка %s

Добро пожаловать в Buckshot Roulette\!
Русская рулетка с дробовиком, предметами и стратегией\.

🔫 Заряжай\. Стреляй\. Выживай\.

Выбери действие:`, escapeMarkdown(botVersion))

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "MarkdownV2"
	reply.ReplyMarkup = ui.MainMenuKeyboard()
	b.api.Send(reply)
}

// handleNewGame — создание новой игры
func (b *Bot) handleNewGame(msg *tgbotapi.Message) {
	player := &game.Player{
		TelegramID:  msg.From.ID,
		Username:    msg.From.UserName,
		DisplayName: getDisplayName(msg.From),
	}

	g, err := b.storage.CreateGame(msg.Chat.ID, player)
	if err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("❌ %s", err.Error()))
		b.api.Send(reply)
		return
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, b.waitingRoomText(g))
	reply.ParseMode = "MarkdownV2"
	reply.ReplyMarkup = ui.WaitingKeyboard(g.ID)
	b.api.Send(reply)
}

// waitingRoomText — текст сообщения созданной игры (комната ожидания).
// Единый формат для /new, кнопки «Создать игру» и возврата в комнату после
// «Отменить» в окне ввода @ника (sendWaitingRoom).
func (b *Bot) waitingRoomText(g *game.Game) string {
	botUsername := b.api.Self.UserName
	inviteLink := fmt.Sprintf("https://t.me/%s?start=join_%s", botUsername, g.ID)
	return fmt.Sprintf(`🎮 *Игра создана\!*

🆔 Код: *%s*

📎 Ссылка:
%s

⏳ Ожидание второго игрока\.\.\.

Отправь ссылку или код другу\!`,
		escapeMarkdown(g.ID), escapeMarkdown(inviteLink))
}

// sendWaitingRoom — повторно показывает комнату ожидания (например, после
// «Отменить» в окне ввода @ника). Человек снова видит кнопки «Пригласить по
// нику» и «❌ Отменить».
func (b *Bot) sendWaitingRoom(chatID int64, g *game.Game) {
	msg := tgbotapi.NewMessage(chatID, b.waitingRoomText(g))
	msg.ParseMode = "MarkdownV2"
	msg.ReplyMarkup = ui.WaitingKeyboard(g.ID)
	b.api.Send(msg)
}

// handleJoinCommand — /join
func (b *Bot) handleJoinCommand(msg *tgbotapi.Message) {
	args := msg.CommandArguments()
	if args != "" {
		b.joinGameDirect(msg, strings.TrimSpace(strings.ToUpper(args)))
		return
	}

	b.waitingForCode[msg.From.ID] = true
	reply := tgbotapi.NewMessage(msg.Chat.ID, "🔗 Введи код комнаты (6 символов):")
	reply.ReplyMarkup = ui.CancelInputKeyboard()
	b.api.Send(reply)
}

// handleJoinByCode — обработка ввода кода
func (b *Bot) handleJoinByCode(msg *tgbotapi.Message) {
	delete(b.waitingForCode, msg.From.ID)
	code := strings.TrimSpace(strings.ToUpper(msg.Text))
	b.joinGameDirect(msg, code)
}

// joinGameDirect — присоединение по коду
func (b *Bot) joinGameDirect(msg *tgbotapi.Message, gameID string) {
	player := &game.Player{
		TelegramID:  msg.From.ID,
		Username:    msg.From.UserName,
		DisplayName: getDisplayName(msg.From),
	}

	g, err := b.storage.JoinGame(gameID, player)
	if err != nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("❌ %s", err.Error()))
		b.api.Send(reply)
		return
	}

	// Уведомляем обоих
	reply := tgbotapi.NewMessage(msg.Chat.ID,
		fmt.Sprintf("✅ Вы присоединились к игре %s!\n🎮 Игра начинается...", gameID))
	b.api.Send(reply)

	// Начинаем игру
	g.StartGame()

	// Отправляем статус обоим игрокам
	b.sendGameState(g)
}

// handleStatus — /status
func (b *Bot) handleStatus(msg *tgbotapi.Message) {
	g := b.storage.GetGameByPlayer(msg.From.ID)
	if g == nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ У вас нет активной игры")
		b.api.Send(reply)
		return
	}

	b.sendGameStateToPlayer(g, msg.From.ID, msg.Chat.ID)
}

// handleCancel — /cancel
func (b *Bot) handleCancel(msg *tgbotapi.Message) {
	g := b.storage.GetGameByPlayer(msg.From.ID)
	if g == nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "❌ У вас нет активной игры")
		b.api.Send(reply)
		return
	}

	// Уведомляем обоих
	for _, p := range g.Players {
		if p != nil {
			notification := tgbotapi.NewMessage(0, fmt.Sprintf("🚪 %s покинул игру. Игра отменена.", getDisplayName(msg.From)))
			notification.ChatID = p.TelegramID // В личку
			b.api.Send(notification)
		}
	}

	b.storage.RemoveGame(g.ID)
}

// handleRules — правила
func (b *Bot) handleRules(msg *tgbotapi.Message) {
	reply := tgbotapi.NewMessage(msg.Chat.ID, ui.RulesText())
	reply.ParseMode = "MarkdownV2"
	reply.ReplyMarkup = ui.BackToMenuKeyboard()
	b.api.Send(reply)
}

// handleVersion — /version: версия сборки.
func (b *Bot) handleVersion(msg *tgbotapi.Message) {
	text := fmt.Sprintf("🏷️ *Buckshot Roulette Bot*\nСборка `%s`\n\nСтабильная версия по правилам оригинала\\.", escapeMarkdown(botVersion))
	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = "MarkdownV2"
	b.api.Send(reply)
}

// handleCallback — обработка inline кнопок
func (b *Bot) handleCallback(cb *tgbotapi.CallbackQuery) {
	// Запоминаем ник нажавшего — пригодится для приглашений по @нику
	b.rememberUser(cb.From)

	// Отвечаем на callback чтобы убрать "часики"
	callback := tgbotapi.NewCallback(cb.ID, "")

	data := cb.Data

	switch {
	case data == "create_game":
		b.callbackCreateGame(cb)
		return

	case data == "join_game":
		b.waitingForCode[cb.From.ID] = true
		callback.Text = "Введите код комнаты"
		b.api.Request(callback)
		msg := tgbotapi.NewMessage(cb.Message.Chat.ID, "🔗 Введи код комнаты (6 символов):")
		msg.ReplyMarkup = ui.CancelInputKeyboard()
		b.api.Send(msg)
		return

	case data == "rules":
		callback.Text = "📖 Правила"
		b.api.Request(callback)
		msg := tgbotapi.NewMessage(cb.Message.Chat.ID, ui.RulesText())
		msg.ParseMode = "MarkdownV2"
		msg.ReplyMarkup = ui.BackToMenuKeyboard()
		b.api.Send(msg)
		return

	case strings.HasPrefix(data, "join_"):
		gameID := strings.TrimPrefix(data, "join_")
		b.callbackJoinGame(cb, gameID)
		return

	case strings.HasPrefix(data, "invite_"):
		gameID := strings.TrimPrefix(data, "invite_")
		b.callbackInvite(cb, gameID)
		return

	case data == "cancel_game":
		b.callbackCancelGame(cb)
		return

	case data == "cancel_input":
		// «Отменить» из состояния ввода кода/@ника. Из ввода кода — в меню,
		// из ввода @ника — обратно в комнату ожидания (если она ещё открыта).
		gameID := b.waitingForInvite[cb.From.ID]
		delete(b.waitingForCode, cb.From.ID)
		delete(b.waitingForInvite, cb.From.ID)
		callback.Text = "❌ Отменено"
		b.api.Request(callback)
		if gameID != "" {
			if g := b.storage.GetGame(gameID); g != nil && g.Phase == game.PhaseWaiting {
				b.sendWaitingRoom(cb.Message.Chat.ID, g)
				return
			}
		}
		b.sendMainMenu(cb.Message)
		return

	case data == "shoot_opponent":
		b.callbackShoot(cb, false)
		return

	case data == "shoot_self":
		b.callbackShoot(cb, true)
		return

	case strings.HasPrefix(data, "use_item_"):
		idStr := strings.TrimPrefix(data, "use_item_")
		itemID, err := strconv.Atoi(idStr)
		if err != nil {
			callback.Text = "❌ Ошибка"
			b.api.Request(callback)
			return
		}
		b.callbackUseItem(cb, itemID)
		return

	case strings.HasPrefix(data, "steal_item_"):
		idStr := strings.TrimPrefix(data, "steal_item_")
		itemID, err := strconv.Atoi(idStr)
		if err != nil {
			callback.Text = "❌ Ошибка"
			b.api.Request(callback)
			return
		}
		b.callbackStealItem(cb, itemID)
		return

	case strings.HasPrefix(data, "rematch_"):
		oldGameID := strings.TrimPrefix(data, "rematch_")
		b.callbackRematch(cb, oldGameID)
		return

	case strings.HasPrefix(data, "end_game_"):
		oldGameID := strings.TrimPrefix(data, "end_game_")
		b.callbackEndGame(cb, oldGameID)
		return

	case data == "main_menu":
		// Кнопка «Меню» со старого экрана конца игры: тоже считаем это
		// отказом от реванша, чтобы соперник не ждал вечно.
		if g := b.storage.GetGameByPlayer(cb.From.ID); g != nil {
			if oldID := b.storage.GetRematchByPending(g.ID); oldID != "" {
				b.declineRematch(cb, oldID)
			}
		}
		callback.Text = ""
		b.api.Request(callback)
		b.handleStart(cb.Message)
		return
	}
}

// callbackCreateGame — создание игры через кнопку
func (b *Bot) callbackCreateGame(cb *tgbotapi.CallbackQuery) {
	player := &game.Player{
		TelegramID:  cb.From.ID,
		Username:    cb.From.UserName,
		DisplayName: getDisplayName(cb.From),
	}

	g, err := b.storage.CreateGame(cb.Message.Chat.ID, player)
	if err != nil {
		callback := tgbotapi.NewCallback(cb.ID, fmt.Sprintf("❌ %s", err.Error()))
		b.api.Request(callback)
		return
	}

	edit := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, b.waitingRoomText(g))
	edit.ParseMode = "MarkdownV2"
	kb := ui.WaitingKeyboard(g.ID)
	edit.ReplyMarkup = &kb
	b.api.Send(edit)

	// Отвечаем на callback, чтобы кнопка не «крутилась»
	b.api.Request(tgbotapi.NewCallback(cb.ID, "✅"))
}

// callbackJoinGame — присоединение через кнопку
func (b *Bot) callbackJoinGame(cb *tgbotapi.CallbackQuery, gameID string) {
	player := &game.Player{
		TelegramID:  cb.From.ID,
		Username:    cb.From.UserName,
		DisplayName: getDisplayName(cb.From),
	}

	g, err := b.storage.JoinGame(gameID, player)
	if err != nil {
		callback := tgbotapi.NewCallback(cb.ID, fmt.Sprintf("❌ %s", err.Error()))
		b.api.Request(callback)
		return
	}

	// Начинаем игру
	g.StartGame()

	callback := tgbotapi.NewCallback(cb.ID, "🎮 Игра начинается!")
	b.api.Request(callback)

	// Отправляем статус обоим
	b.sendGameState(g)

	// Уведомляем соперника, что приглашённый зашёл в комнату
	for _, p := range g.Players {
		if p != nil && p.TelegramID != cb.From.ID {
			msg := tgbotapi.NewMessage(p.TelegramID,
				fmt.Sprintf("✅ %s присоединился — игра начинается!", escapeMarkdown(getDisplayName(cb.From))))
			msg.ParseMode = "MarkdownV2"
			b.api.Send(msg)
		}
	}
}

// callbackInvite — «Пригласить по нику» в комнате ожидания.
// Просим ввести @ник; дальше handleInviteByNick отправит личное приглашение.
func (b *Bot) callbackInvite(cb *tgbotapi.CallbackQuery, gameID string) {
	g := b.storage.GetGame(gameID)
	if g == nil {
		b.api.Request(tgbotapi.NewCallback(cb.ID, "❌ Игра не найдена"))
		return
	}
	if g.Phase != game.PhaseWaiting {
		b.api.Request(tgbotapi.NewCallback(cb.ID, "❌ Игра уже началась"))
		return
	}

	b.waitingForInvite[cb.From.ID] = gameID

	b.api.Request(tgbotapi.NewCallback(cb.ID, "👤"))
	msg := tgbotapi.NewMessage(cb.Message.Chat.ID, "👤 Введи @ник друга (без @), и я отправлю ему приглашение в личку:")
	msg.ReplyMarkup = ui.CancelInputKeyboard()
	b.api.Send(msg)
}

// handleInviteByNick — обработка введённого @ника для приглашения в комнату.
// Если человек хоть раз запускал бота — шлём ему личное приглашение с кнопкой
// (он увидит уведомление и зайдёт в один тап). Если нет — Telegram запрещает
// боту инициировать диалог с тем, кто не запускал бота, поэтому отдаём
// ссылку-приглашение, которую можно переслать самому.
func (b *Bot) handleInviteByNick(msg *tgbotapi.Message, gameID string) {
	g := b.storage.GetGame(gameID)
	if g == nil {
		b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ Игра не найдена."))
		return
	}
	if g.Phase != game.PhaseWaiting {
		b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, "❌ Игра уже началась."))
		return
	}

	// Нормализуем ник: люди часто пишут «@madcxd», а не «madcxd». Срезаем @,
	// чтобы в ответе не получилось «@@madcxd».
	nick := strings.TrimPrefix(strings.TrimSpace(msg.Text), "@")
	targetID, found := b.storage.FindUserByNick(nick)
	if !found {
		// В реестре нет: человек никогда не запускал бота, писать ему нельзя.
		botUsername := b.api.Self.UserName
		link := fmt.Sprintf("https://t.me/%s?start=join_%s", botUsername, g.ID)
		reply := tgbotapi.NewMessage(msg.Chat.ID,
			fmt.Sprintf("😕 `@%s` ещё не запускал бота — я не могу написать ему в личку\\.\n\n📎 Вот ссылка\\-приглашение, перешли её сам:\n%s",
				escapeMarkdown(nick), escapeMarkdown(link)))
		reply.ParseMode = "MarkdownV2"
		b.api.Send(reply)
		return
	}

	// Отправляем личное приглашение с кнопкой «Присоединиться»
	inviteText := fmt.Sprintf("🎮 %s приглашает тебя в игру\\!\n\n🆔 Код комнаты: %s\n\nНажми кнопку — и ты внутри\\.",
		escapeMarkdown(getDisplayName(msg.From)), escapeMarkdown(g.ID))
	inv := tgbotapi.NewMessage(targetID, inviteText)
	inv.ParseMode = "MarkdownV2"
	inv.ReplyMarkup = ui.JoinInviteKeyboard(g.ID)
	if _, err := b.api.Send(inv); err != nil {
		b.api.Send(tgbotapi.NewMessage(msg.Chat.ID, fmt.Sprintf("❌ Не удалось отправить приглашение: %v", err)))
		return
	}

	b.api.Send(tgbotapi.NewMessage(msg.Chat.ID,
		fmt.Sprintf("✅ Приглашение отправлено @%s — он получит уведомление в личку.", escapeMarkdown(nick))))
}

// callbackCancelGame — отмена игры. Кнопка «❌ Отменить» живёт в сообщении
// комнаты ожидания и остаётся на экране даже после старта игры — если её
// нажать в разгар матча, отменяем как /cancel (с уведомлением соперника),
// а не молча удаляем игру, оставляя соперника с «мёртвыми» кнопками.
func (b *Bot) callbackCancelGame(cb *tgbotapi.CallbackQuery) {
	g := b.storage.GetGameByPlayer(cb.From.ID)
	if g == nil {
		b.api.Request(tgbotapi.NewCallback(cb.ID, "❌ Игра не найдена"))
		return
	}

	// Уведомляем соперника (себя не надо — сам нажал)
	for _, p := range g.Players {
		if p != nil && p.TelegramID != cb.From.ID {
			notification := tgbotapi.NewMessage(p.TelegramID,
				fmt.Sprintf("🚪 %s покинул игру. Игра отменена.", getDisplayName(cb.From)))
			b.api.Send(notification)
		}
	}

	b.storage.RemoveGame(g.ID)

	b.api.Request(tgbotapi.NewCallback(cb.ID, "🚪"))
	edit := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID,
		"🚪 Игра отменена.")
	b.api.Send(edit)
}

// callbackShoot — выстрел
func (b *Bot) callbackShoot(cb *tgbotapi.CallbackQuery, shootSelf bool) {
	g := b.storage.GetGameByPlayer(cb.From.ID)
	if g == nil {
		callback := tgbotapi.NewCallback(cb.ID, "❌ Игра не найдена")
		b.api.Request(callback)
		return
	}

	result, err := g.Shoot(cb.From.ID, shootSelf)
	if err != nil {
		callback := tgbotapi.NewCallback(cb.ID, fmt.Sprintf("❌ %s", err.Error()))
		b.api.Request(callback)
		return
	}

	callback := tgbotapi.NewCallback(cb.ID, "💥")
	b.api.Request(callback)

	// Отправляем результат выстрела обоим
	b.sendToAllPlayers(g, result)

	// Задержка для драматичности
	time.Sleep(500 * time.Millisecond)

	if g.Phase == game.PhaseGameOver {
		b.sendGameOver(g)
	} else {
		b.sendGameState(g)
	}
}

// callbackUseItem — использование предмета
func (b *Bot) callbackUseItem(cb *tgbotapi.CallbackQuery, itemID int) {
	g := b.storage.GetGameByPlayer(cb.From.ID)
	if g == nil {
		callback := tgbotapi.NewCallback(cb.ID, "❌ Игра не найдена")
		b.api.Request(callback)
		return
	}

	result, err := g.UseItem(cb.From.ID, itemID)
	if err != nil {
		callback := tgbotapi.NewCallback(cb.ID, fmt.Sprintf("❌ %s", err.Error()))
		b.api.Request(callback)
		return
	}

	callback := tgbotapi.NewCallback(cb.ID, "✅")
	b.api.Request(callback)

	// Отправляем результат
	if g.Phase == game.PhaseAdrenaline {
		// Редактируем ТО ЖЕ сообщение, кнопку которого нажали: кнопки своих
		// предметов (use_item_*) исчезают, вместо них появляется меню кражи
		// с предметами соперника (steal_item_*). Иначе старые кнопки висят
		// рядом и игрок путает свои предметы с теми, что можно украсть.
		b.editAdrenalineMenu(cb, g)
	} else if g.Phase == game.PhaseGameOver {
		b.sendToAllPlayers(g, result)
		b.sendGameOver(g)
	} else {
		// Результат предмета видит только игрок (лупа, телефон) или оба
		needPrivate := false
		// Лупа и телефон — приватно
		if strings.Contains(result, "🔍") || strings.Contains(result, "📱") {
			needPrivate = true
		}

		if needPrivate {
			// Приватно текущему игроку
			msg := tgbotapi.NewMessage(cb.Message.Chat.ID, "🤫 "+result)
			b.api.Send(msg)
			// Остальным — просто факт использования
			opponent := g.OpponentPlayer()
			opMsg := tgbotapi.NewMessage(opponent.TelegramID,
				fmt.Sprintf("🔮 %s использовал предмет...", g.CurrentPlayer().DisplayName))
			b.api.Send(opMsg)
		} else {
			b.sendToAllPlayers(g, result)
		}

		// Обновляем состояние
		time.Sleep(300 * time.Millisecond)
		b.sendGameState(g)
	}
}

// callbackStealItem — кража предмета (адреналин)
func (b *Bot) callbackStealItem(cb *tgbotapi.CallbackQuery, itemID int) {
	g := b.storage.GetGameByPlayer(cb.From.ID)
	if g == nil {
		b.api.Request(tgbotapi.NewCallback(cb.ID, "❌ Игра не найдена"))
		return
	}

	result, err := g.UseItem(cb.From.ID, itemID)
	if err != nil {
		callback := tgbotapi.NewCallback(cb.ID, fmt.Sprintf("❌ %s", err.Error()))
		b.api.Request(callback)
		return
	}

	callback := tgbotapi.NewCallback(cb.ID, "💉 Украдено!")
	b.api.Request(callback)

	b.sendToAllPlayers(g, "💉 "+result)

	time.Sleep(300 * time.Millisecond)

	if g.Phase == game.PhaseGameOver {
		b.sendGameOver(g)
	} else {
		b.sendGameState(g)
	}
}

// callbackRematch — реванш с тем же соперником.
// Кнопка несёт ID закончившейся игры: первый нажавший создаёт новую комнату
// и ждёт, второй автоматически попадает в неё. Код комнаты вводить не нужно.
func (b *Bot) callbackRematch(cb *tgbotapi.CallbackQuery, oldGameID string) {
	player := &game.Player{
		TelegramID:  cb.From.ID,
		Username:    cb.From.UserName,
		DisplayName: getDisplayName(cb.From),
	}

	// Соперник уже завершил игру (нажал «Закончить играть»/«Меню») —
	// не даём уйти в бесконечное ожидание реванша.
	if decliner := b.storage.GetDeclinedRematch(oldGameID); decliner != 0 {
		if decliner != cb.From.ID {
			b.api.Request(tgbotapi.NewCallback(cb.ID, "🙅 Соперник завершил игру"))
			msg := tgbotapi.NewMessage(cb.Message.Chat.ID,
				"🙅 Соперник уже завершил игру — реванш невозможен. Создай новую игру и позови кого-нибудь!")
			msg.ReplyMarkup = ui.MainMenuKeyboard()
			b.api.Send(msg)
			return
		}
		// Тот же игрок передумал после отказа — снимаем отметку и продолжаем
		b.storage.RemoveDeclinedRematch(oldGameID)
	}

	old := b.storage.GetGame(oldGameID)

	// Соперник уже ждёт в комнате реванша — входим в неё
	if pending := b.storage.GetRematch(oldGameID); pending != nil {
		if _, err := b.storage.JoinGame(pending.ID, player); err == nil {
			b.storage.RemoveRematch(oldGameID)
			b.storage.RemoveDeclinedRematch(oldGameID)
			if old != nil {
				b.storage.RemoveGame(old.ID)
			}
			b.api.Request(tgbotapi.NewCallback(cb.ID, "🔄 Реванш!"))

			pending.StartGame()
			b.sendGameState(pending)
			return
		}
	}

	// Убираем старую игру, создаём новую комнату
	if old != nil {
		b.storage.RemoveGame(old.ID)
	}

	newGame, err := b.storage.CreateGame(cb.Message.Chat.ID, player)
	if err != nil {
		b.api.Request(tgbotapi.NewCallback(cb.ID, fmt.Sprintf("❌ %s", err.Error())))
		return
	}

	b.api.Request(tgbotapi.NewCallback(cb.ID, "🔄"))

	// Если реванш-игру можно создать только после этой — ставим её в ожидание
	b.storage.SetRematch(oldGameID, newGame)

	text := fmt.Sprintf("🔄 *Ты готов к реваншу\\!*\n\n⏳ Ждём, пока %s нажмёт «Реванш»\\.\\.\\.",
		escapeMarkdown(b.opponentName(old, cb.From.ID)))
	b.editMessage(cb, text, nil)

	// Пингуем соперника — ему достаточно нажать кнопку реванша на своём экране
	if old != nil {
		for _, p := range old.Players {
			if p != nil && p.TelegramID != cb.From.ID {
				msg := tgbotapi.NewMessage(p.TelegramID,
					fmt.Sprintf("🏓 %s хочет реванш! Нажми «Реванш», чтобы продолжить с тем же соперником без новой комнаты.",
						getDisplayName(cb.From)))
				msg.ReplyMarkup = ui.GameOverKeyboard(oldGameID)
				b.api.Send(msg)
			}
		}
	}
}

// callbackEndGame — «Закончить играть» после матча. Отменяет ожидающий реванш,
// уведомляет предложившего его соперника и возвращает в меню.
func (b *Bot) callbackEndGame(cb *tgbotapi.CallbackQuery, oldGameID string) {
	b.declineRematch(cb, oldGameID)

	b.api.Request(tgbotapi.NewCallback(cb.ID, "⏹️ Игра завершена"))
	b.sendMainMenu(cb.Message)
}

// declineRematch — общая логика отказа от реванша: отмечаем отказ, закрываем
// ожидающую комнату (если соперник уже предложил реванш) и уведомляем его,
// чтобы тот не ждал вечно.
func (b *Bot) declineRematch(cb *tgbotapi.CallbackQuery, oldGameID string) {
	b.storage.SetDeclinedRematch(oldGameID, cb.From.ID)

	pending := b.storage.GetRematch(oldGameID)
	if pending == nil {
		return
	}

	// Закрываем ожидающую комнату: соперник сидит в ней и ждёт ответа
	b.storage.RemoveGame(pending.ID)
	b.storage.RemoveRematch(oldGameID)

	for _, p := range pending.Players {
		if p != nil && p.TelegramID != cb.From.ID {
			msg := tgbotapi.NewMessage(p.TelegramID,
				fmt.Sprintf("🙅 %s завершил игру — реванш отменён.",
					getDisplayName(cb.From)))
			msg.ReplyMarkup = ui.MainMenuKeyboard()
			b.api.Send(msg)
		}
	}
}

// opponentName — имя соперника в закончившейся игре
func (b *Bot) opponentName(g *game.Game, playerID int64) string {
	if g == nil {
		return "соперник"
	}
	for _, p := range g.Players {
		if p != nil && p.TelegramID != playerID {
			return p.DisplayName
		}
	}
	return "соперник"
}

// editMessage — редактирование сообщения по callback
func (b *Bot) editMessage(cb *tgbotapi.CallbackQuery, text string, kb *tgbotapi.InlineKeyboardMarkup) {
	edit := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, text)
	edit.ParseMode = "MarkdownV2"
	if kb != nil {
		edit.ReplyMarkup = kb
	}
	b.api.Send(edit)
}

// sendGameState — отправка состояния игры обоим игрокам
func (b *Bot) sendGameState(g *game.Game) {
	for _, p := range g.Players {
		if p == nil {
			continue
		}
		b.sendGameStateToPlayer(g, p.TelegramID, p.TelegramID)
	}
}

// sendGameStateToPlayer — отправка состояния конкретному игроку
func (b *Bot) sendGameStateToPlayer(g *game.Game, telegramID int64, chatID int64) {
	// Добавляем индикатор хода
	turnHint := "\n⏳ Ход противника\\.\\.\\."
	if g.IsPlayerTurn(telegramID) {
		turnHint = "\n👆 *Ваш ход\\!* Выберите действие:"
	}

	// Экранируем только динамический текст, а индикатор хода уже в валидной
	// MarkdownV2 — иначе двойное экранирование ломает разметку и Telegram
	// возвращает 400, из-за чего сообщение с кнопками не доходит.
	msg := tgbotapi.NewMessage(chatID, escapeMarkdown(g.GetStatusText(telegramID))+turnHint)
	msg.ParseMode = "MarkdownV2"

	if g.IsPlayerTurn(telegramID) {
		if g.Phase == game.PhaseAdrenaline {
			msg.ReplyMarkup = ui.AdrenalineKeyboard(g)
		} else {
			msg.ReplyMarkup = ui.GameActionKeyboard(g)
		}
	}

	b.api.Send(msg)
}

// editAdrenalineMenu — превращаем сообщение с кнопками действий в меню кражи.
// Редактируем ИМЕННО то сообщение, кнопку которого нажали, чтобы кнопки своих
// предметов не остались на экране и не путали игрока с предметами для кражи.
func (b *Bot) editAdrenalineMenu(cb *tgbotapi.CallbackQuery, g *game.Game) {
	opponent := g.OpponentPlayer()
	kb := ui.AdrenalineKeyboard(g)

	text := fmt.Sprintf("💉 Адреналин\\! Выбери предмет %s для кражи:",
		escapeMarkdown(opponent.DisplayName))
	b.editMessage(cb, text, &kb)

	// Сопернику — подсказка, что его инвентарь сейчас смотрят
	for _, p := range g.Players {
		if p != nil && p.TelegramID != cb.From.ID {
			b.api.Send(tgbotapi.NewMessage(p.TelegramID,
				fmt.Sprintf("⏳ %s выбирает предмет для кражи...", getDisplayName(cb.From))))
		}
	}
}

// sendToAllPlayers — отправка сообщения обоим игрокам
func (b *Bot) sendToAllPlayers(g *game.Game, text string) {
	for _, p := range g.Players {
		if p == nil {
			continue
		}
		msg := tgbotapi.NewMessage(p.TelegramID, text)
		b.api.Send(msg)
	}
}

// sendGameOver — конец игры
func (b *Bot) sendGameOver(g *game.Game) {
	winner := g.GetWinner()
	var text string

	if winner != nil {
		text = fmt.Sprintf(`
🎯 *ИГРА ОКОНЧЕНА*
══════════════

🏆 Победитель: *%s*

💀 Игра длилась %d раундов\.

Хорошая игра\!`,
			escapeMarkdown(winner.DisplayName), g.Round)
	} else {
		text = "🎯 *ИГРА ОКОНЧЕНА*\n\nНичья?"
	}

	for _, p := range g.Players {
		if p == nil {
			continue
		}
		msg := tgbotapi.NewMessage(p.TelegramID, text)
		msg.ParseMode = "MarkdownV2"
		msg.ReplyMarkup = ui.GameOverKeyboard(g.ID)
		b.api.Send(msg)
	}
}

// escapeMarkdown — экранирование символов для MarkdownV2
func escapeMarkdown(text string) string {
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(text)
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
