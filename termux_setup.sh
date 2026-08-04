#!/data/data/com.termux/files/usr/bin/bash
# Первый запуск бота на телефоне через Termux.
#   Шаг 1. Установи Termux из F-Droid (НЕ из Play Market) и Termux:API из F-Droid.
#   Шаг 2. Создай этот файл:  nano termux_setup.sh  → вставь текст → Ctrl+X, Y, Enter.
#   Шаг 3. Выполни:  bash termux_setup.sh
set -e

echo "==> Обновляем пакеты"
pkg update -y && pkg upgrade -y

echo "==> Ставим git, Go и Termux:API"
pkg install -y git golang termux-api

echo "==> Клонируем репозиторий"
# Если репозиторий приватный — сначала авторизуйся:
#   gh auth login   (выбрать: Paste an authentication token)
# тогда клонирование пройдёт без пароля.
if [ ! -d BuckshotRouletteBot ]; then
	git clone https://github.com/ТВОЙ_ЛОГИН/BuckshotRouletteBot.git
fi
cd BuckshotRouletteBot

echo "==> Создаём .env"
if [ ! -f .env ]; then
	read -rp "Вставь TELEGRAM_BOT_TOKEN: " TOKEN
	printf 'TELEGRAM_BOT_TOKEN=%s\n' "$TOKEN" > .env
	echo "(Если api.telegram.org у тебя блокируется — добавь в .env строку TELEGRAM_PROXY=socks5://127.0.0.1:ПОРТ и запусти прокси на телефоне.)"
fi

echo "==> Собираем (CGO не нужен — чистый Go)"
CGO_ENABLED=0 go build -o buckshot .

echo "==> Запускаем в фоне"
bash termux.sh start
sleep 2
bash termux.sh status
echo "Лог: bash termux.sh log"
