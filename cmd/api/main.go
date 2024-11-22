package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	_ "github.com/lib/pq"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/Segren/testTask/internal/data"
	"github.com/Segren/testTask/internal/jsonlog"
)

var (
	buildTime string
	version   string
)

type config struct {
	port int
	env  string
	db   struct {
		dsn          string
		maxOpenConns int
		maxIdleConns int
		maxIdleTime  string
	}
	//для конфигурации кол-ва запросов в секунду и во время бурста
	limiter struct {
		rps     float64
		burst   int
		enabled bool
	}
	displayVersion bool
}

var (
	instance *config
	once     sync.Once
)

// singleton
func GetConfig() *config {
	once.Do(func() {
		instance = &config{}
		flag.IntVar(&instance.port, "port", 8080, "API server port")
		flag.StringVar(&instance.env, "env", "development", "Environment (development|staging|production)")

		flag.StringVar(&instance.db.dsn, "db-dsn", "", "PostgreSQL DSN")

		flag.IntVar(&instance.db.maxOpenConns, "db-max-open-conns", 25, "PostgreSQL max open connections")
		flag.IntVar(&instance.db.maxIdleConns, "db-max-idle-conns", 25, "PostgreSQL max idle connections")
		flag.StringVar(&instance.db.maxIdleTime, "db-max-idle-time", "15m", "PostgreSQL max connection idle time")

		flag.Float64Var(&instance.limiter.rps, "limiter-rps", 2, "Rate limiter maximum requests per second")
		flag.IntVar(&instance.limiter.burst, "limiter-burst", 4, "Rate limiter maximum burst")
		flag.BoolVar(&instance.limiter.enabled, "limiter-enabled", true, "Enable rate limiter")

		// булево для отображения версии проекта и выхода
		flag.BoolVar(&instance.displayVersion, "version", false, "Display version information and exit")

		flag.Parse()
	})
	return instance
}

type application struct {
	config config
	logger *jsonlog.Logger
	models data.Models
	wg     sync.WaitGroup
}

func main() {
	cfg := GetConfig()

	if cfg.displayVersion {
		fmt.Printf("Greenlight version:\t%s\n", version)
		fmt.Printf("Build time:\t%s\n", buildTime)
		os.Exit(0)
	}

	logger := jsonlog.New(os.Stdout, jsonlog.LevelInfo)

	if cfg.db.dsn == "" {
		cfg.db.dsn = os.Getenv("MUSIC_DB_DSN")
		if cfg.db.dsn == "" {
			logger.PrintFatal(errors.New("должна быть установлена строка подключения к базе данных через флаг -db-dsn или переменную окружения MUSIC_DB_DSN"), nil)
		}
	}

	logger.PrintInfo("Connecting to database with DSN: "+cfg.db.dsn, nil)

	db, err := openDB(*cfg)
	if err != nil {
		logger.PrintFatal(err, nil)
	}

	defer db.Close()

	logger.PrintInfo("database connection pool established", nil)

	app := &application{
		config: *cfg,
		logger: logger,
		models: data.NewModels(db),
	}

	//запускаем мок внешнего api чтобы получать releaseDate, text, link
	startExternalMockServer()

	err = app.serve()
	if err != nil {
		logger.PrintFatal(err, nil)
	}
}

// возвращает пул подключений дб
func openDB(cfg config) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.db.dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(cfg.db.maxOpenConns)
	db.SetMaxIdleConns(cfg.db.maxIdleConns)

	duration, err := time.ParseDuration(cfg.db.maxIdleTime)
	if err != nil {
		return nil, err
	}

	db.SetConnMaxIdleTime(duration)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	if err != nil {
		return nil, err
	}

	return db, nil
}

func startExternalMockServer() {
	http.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
		group := r.URL.Query().Get("group")
		song := r.URL.Query().Get("song")

		if group == "" || song == "" {
			http.Error(w, "Missing parameters", http.StatusBadRequest)
			return
		}

		response := map[string]interface{}{
			"releaseDate": "2024-11-22",
			"text":        "Some lyrics here",
			"link":        "https://example.com/song",
		}

		json.NewEncoder(w).Encode(response)
	})

	go http.ListenAndServe(":8081", nil)
}
