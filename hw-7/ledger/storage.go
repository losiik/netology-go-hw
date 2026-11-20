package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/redis/go-redis/v9"

	"ledger/internal/cache"
	"ledger/internal/db"
)

var (
	pg  *sql.DB
	rdb *redis.Client
)

var ErrNotInitialized = errors.New("ledger не инициализирован, вызовите ledger.Init")

type Config struct {
	DatabaseURL string
	Redis       cache.Config
}

func ConfigFromEnv() (Config, error) {
	rc, err := cache.ConfigFromEnv()
	if err != nil {
		return Config{}, err
	}
	return Config{DatabaseURL: db.DSNFromEnv(), Redis: rc}, nil
}

func Init(ctx context.Context, cfg Config) error {
	conn, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	log.Printf("подключено к PostgreSQL: %s", db.SafeDSN(cfg.DatabaseURL))

	client, err := cache.New(ctx, cfg.Redis)
	if err != nil {
		conn.Close()
		return err
	}
	log.Printf("подключено к Redis: %s, db=%d", cfg.Redis.Addr, cfg.Redis.DB)

	pg = conn
	rdb = client
	return nil
}

func Close() {
	if pg != nil {
		pg.Close()
		pg = nil
	}
	if rdb != nil {
		rdb.Close()
		rdb = nil
	}
}

func ready() error {
	if pg == nil || rdb == nil {
		return ErrNotInitialized
	}
	return nil
}

func Reset() error {
	if err := ready(); err != nil {
		return err
	}
	ctx := context.Background()
	if _, err := pg.ExecContext(ctx, "TRUNCATE expenses, budgets RESTART IDENTITY"); err != nil {
		return err
	}
	return rdb.FlushDB(ctx).Err()
}

func cacheGet(ctx context.Context, key string, v any) bool {
	data, err := rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		log.Printf("cache miss: %s", key)
		return false
	}
	if err != nil {
		log.Printf("ошибка чтения кеша %s: %v", key, err)
		return false
	}
	if err := json.Unmarshal(data, v); err != nil {
		log.Printf("битое значение в кеше %s: %v", key, err)
		return false
	}
	log.Printf("cache hit: %s", key)
	return true
}

func cacheSet(ctx context.Context, key string, v any, ttl time.Duration) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("не удалось сериализовать %s: %v", key, err)
		return
	}
	if err := rdb.Set(ctx, key, data, ttl).Err(); err != nil {
		log.Printf("ошибка записи кеша %s: %v", key, err)
		return
	}
	log.Printf("cache set: %s (ttl %s)", key, ttl)
}

func cacheDel(ctx context.Context, keys ...string) {
	if err := rdb.Del(ctx, keys...).Err(); err != nil {
		log.Printf("ошибка удаления из кеша %v: %v", keys, err)
	}
}
