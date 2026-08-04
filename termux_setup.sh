#!/data/data/com.termux/files/usr/bin/bash
# Первый запуск бота на телефоне через Termux.
#
# Ты НЕ создаёшь этот файл руками — он уже лежит в репозитории.
# Сначала склонируй репо и зайди в папку (команды ниже), потом запусти:
#
#   git clone https://github.com/ТВОЙ_ЛОГИН/BuckshotRouletteBot.git
#   cd BuckshotRouletteBot
#   bash termux_setup.sh
set -e

# Защита: скрипт должен запускаться из папки репозитория
if [ ! -f go.mod ]; then
	echo "Склонируй репо и зайди в папку (см. шапку файла), потом запускай снова."
	exit 1
fi

echo "==> Создаём .env"
if [ ! -f .env ]; then
	read -rp "Вставь TELEGRAM_BOT_TOKEN: " TOKEN
	printf 'TELEGRAM_BOT_TOKEN=%s\n' "$TOKEN" > .env
	echo "(Если api.telegram.org блокируется — добавь в .env строку TELEGRAM_PROXY=socks5://127.0.0.1:ПОРТ)"
fi

echo "==> Собираем (CGO не нужен — чистый Go)"
CGO_ENABLED=0 go build -o buckshot .

echo "==> Запускаем в фоне"
bash termux.sh start
sleep 2
bash termux.sh status
echo "Лог: bash termux.sh log"
