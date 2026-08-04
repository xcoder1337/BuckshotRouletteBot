package main

import (
	"log"
	"os"
	"strings"

	"buckshot-roulette-bot/bot"
)

// loadEnvFile — подгружает переменные из файла .env в окружение процесса
// (не перезаписывая уже заданные). docker-compose читает .env сам,
// а для локального запуска go run main.go это делает приложение.
func loadEnvFile() {
	data, err := os.ReadFile(".env")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found || key == "" {
			continue
		}
		if os.Getenv(key) == "" {
			os.Setenv(key, strings.TrimSpace(value))
		}
	}
}

func main() {
	loadEnvFile()
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable is required")
	}

	b, err := bot.New(token)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("🔫 Buckshot Roulette Bot started!")
	b.Run()
}
