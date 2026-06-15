package main

import (
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	TelegramToken string
	AdminID       int64
	MaxUsers      int
	CheckInterval time.Duration
	DataDir       string
}

func LoadConfig() Config {
	godotenv.Load()

	token := os.Getenv("TG_TOKEN")
	if token == "" {
		log.Fatal("TG_TOKEN is required")
	}

	adminID, _ := strconv.ParseInt(os.Getenv("ADMIN_ID"), 10, 64)
	maxUsers, _ := strconv.Atoi(os.Getenv("MAX_USERS"))
	interval, _ := strconv.Atoi(os.Getenv("CHECK_INTERVAL"))
	if interval <= 0 {
		interval = 600
	}
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}

	return Config{
		TelegramToken: token,
		AdminID:       adminID,
		MaxUsers:      maxUsers,
		CheckInterval: time.Duration(interval) * time.Second,
		DataDir:       dataDir,
	}
}

func main() {
	cfg := LoadConfig()

	store, err := NewStorage(cfg.DataDir)
	if err != nil {
		log.Fatalf("storage init: %v", err)
	}

	bot, err := NewBot(cfg, store)
	if err != nil {
		log.Fatalf("bot init: %v", err)
	}

	go RunChecker(cfg, store, bot)

	log.Printf("bot started, interval=%s, max_users=%d", cfg.CheckInterval, cfg.MaxUsers)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down")
}
