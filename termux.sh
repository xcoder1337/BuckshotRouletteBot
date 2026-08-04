#!/data/data/com.termux/files/usr/bin/bash
# Управление ботом на телефоне (Termux).
#   bash termux.sh start | stop | restart | status | log | update
#
# Первый запуск: скопируй termux_setup.sh на телефон и выполни его,
# дальше пользуйся этим скриптом.

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"
BIN="$DIR/buckshot"
LOG="$DIR/bot.log"

start() {
	# Wake-lock: не даём телефону уснуть (нужен Termux:API)
	termux-wake-lock 2>/dev/null || echo "(!) termux-api не установлен — телефон может уснуть. Сделай: pkg install termux-api"
	if pgrep -f "$BIN" >/dev/null 2>&1; then
		echo "⚠️ Бот уже запущен"
	else
		nohup "$BIN" > "$LOG" 2>&1 &
		echo "✅ Бот запущен (pid $!)"
	fi
}

stop() {
	if pgrep -f "$BIN" >/dev/null 2>&1; then
		pkill -f "$BIN"
		echo "⏹ Остановлен"
	else
		echo "Бот и так не запущен"
	fi
}

restart() { stop; sleep 1; start; }

status() {
	if pgrep -f "$BIN" >/dev/null 2>&1; then
		echo "✅ работает"
	else
		echo "⛔ не работает"
	fi
}

log() { tail -n 50 -f "$LOG"; }

update() {
	git pull
	CGO_ENABLED=0 go build -o "$BIN" .
	restart
}

cmd="${1:-status}"
case "$cmd" in
	start)   start ;;
	stop)    stop ;;
	restart) restart ;;
	status)  status ;;
	log)     log ;;
	update)  update ;;
	*) echo "Использование: bash termux.sh start | stop | restart | status | log | update" ;;
esac
