package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

func setupTimezone() {
	tz := os.Getenv("TZ")
	if tz == "" {
		tz = "UTC"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		log.Printf("invalid TZ=%q: %v, using UTC", tz, err)
		return
	}
	time.Local = loc
	log.Printf("timezone: %s", tz)
}

func setupLogging(dataDir string) {
	logsDir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		log.Printf("cannot create logs dir: %v, logging to stdout only", err)
		return
	}

	date := time.Now().Format("02-01-2006")
	n := 1
	for {
		path := filepath.Join(logsDir, fmt.Sprintf("botLog-%s-%d.log", date, n))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				log.Printf("cannot open log file: %v, logging to stdout only", err)
				return
			}
			multi := io.MultiWriter(os.Stdout, f)
			log.SetOutput(multi)
			log.Printf("log file: %s", path)
			return
		}
		n++
	}
}

type Config struct {
	TelegramToken    string
	AdminID          int64
	MaxUsers         int
	MaxRPS           int
	SiteDomain       string
	DataDir          string
	CheckConcurrency int
}

func LoadConfig() Config {
	godotenv.Load()

	token := os.Getenv("TG_TOKEN")
	if token == "" {
		log.Fatal("TG_TOKEN is required")
	}

	adminID, _ := strconv.ParseInt(os.Getenv("ADMIN_ID"), 10, 64)
	maxUsers, _ := strconv.Atoi(os.Getenv("MAX_USERS"))
	maxRPS, _ := strconv.Atoi(os.Getenv("MAX_RPS"))
	if maxRPS <= 0 {
		maxRPS = 2
	}
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}
	siteDomain := os.Getenv("SITE_DOMAIN")
	if siteDomain == "" {
		siteDomain = "ege-kostroma.ru"
	}
	checkConcurrency, _ := strconv.Atoi(os.Getenv("CHECK_CONCURRENCY"))
	if checkConcurrency <= 0 {
		checkConcurrency = 5
	}

	return Config{
		TelegramToken:    token,
		AdminID:          adminID,
		MaxUsers:         maxUsers,
		MaxRPS:           maxRPS,
		SiteDomain:       siteDomain,
		DataDir:          dataDir,
		CheckConcurrency: checkConcurrency,
	}
}

func validateDataDir(dir string) error {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0770); err != nil {
			return fmt.Errorf("cannot create data dir %q: %w\n"+
				"  Fix: mkdir -p %s && chmod 770 %s", dir, err, dir, dir)
		}
		info, err = os.Stat(dir)
		if err != nil {
			return err
		}
	}
	if !info.IsDir() {
		return fmt.Errorf("%q is not a directory", dir)
	}

	testFile := filepath.Join(dir, ".write_test")
	f, err := os.OpenFile(testFile, os.O_CREATE|os.O_WRONLY, 0660)
	if err != nil {
		return fmt.Errorf("cannot write to %q: %w\n"+
			"  Current dir permissions: %s\n"+
			"  Fix: chmod 770 %s || chown -R $(id -u):$(id -g) %s",
			dir, err, info.Mode(), dir, dir)
	}
	f.Close()
	os.Remove(testFile)

	log.Printf("data dir: %s (mode: %s, uid: ok)", dir, info.Mode())
	return nil
}

func main() {
	cfg := LoadConfig()

	setupTimezone()

	if err := validateDataDir(cfg.DataDir); err != nil {
		log.Fatalf("DATA DIR ERROR:\n%v", err)
	}

	setupLogging(cfg.DataDir)

	InitRateLimiter(cfg.MaxRPS)
	InitSiteURL(cfg.SiteDomain)
	log.Printf("rate limit: %d req/s, site: %s", cfg.MaxRPS, cfg.SiteDomain)

	store, err := NewStorage(cfg.DataDir)
	if err != nil {
		log.Fatalf("storage init: %v", err)
	}

	var bot *Bot
	for attempt := 1; ; attempt++ {
		bot, err = NewBot(cfg, store)
		if err == nil {
			break
		}
		backoff := time.Duration(attempt*5) * time.Second
		if backoff > 2*time.Minute {
			backoff = 2 * time.Minute
		}
		log.Printf("bot init failed (attempt %d): %v — retrying in %s", attempt, err, backoff)
		time.Sleep(backoff)
	}

	go RunChecker(store, bot, cfg.CheckConcurrency)
	go RunInactivityManager(store, bot)

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		uptime := stats.UptimeDuration()
		fmt.Fprintf(w, `{"status":"ok","uptime":"%s","users":%d}`, uptime.Round(time.Second), store.TotalUsers())
	})
	go func() {
		log.Println("healthcheck: :8080/health")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Printf("healthcheck server stopped: %v", err)
		}
	}()

	log.Printf("bot started, max_users=%d, check_concurrency=%d", cfg.MaxUsers, cfg.CheckConcurrency)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("received signal %v, shutting down...", sig)

	bot.Stop()
	log.Println("bot stopped")
}
