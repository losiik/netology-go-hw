package cache

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestRedisCache(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR не задан, тест с Redis пропущен")
	}
	ctx := context.Background()
	rdb, err := New(ctx, Config{Addr: addr, DB: 15})
	if err != nil {
		t.Fatal(err)
	}
	defer rdb.Close()
	rdb.FlushDB(ctx)
	c := NewRedisCache(rdb)

	if _, ok, err := c.Get(ctx, "report:summary:a"); ok || err != nil {
		t.Fatalf("пустой кеш: ok=%v err=%v", ok, err)
	}

	for _, k := range []string{"report:summary:a", "report:summary:b", "budgets:all"} {
		if err := c.Set(ctx, k, []byte(k), time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	data, ok, err := c.Get(ctx, "report:summary:a")
	if err != nil || !ok || string(data) != "report:summary:a" {
		t.Fatalf("Get = %q %v %v", data, ok, err)
	}
	if ttl := rdb.TTL(ctx, "report:summary:a").Val(); ttl <= 0 || ttl > time.Minute {
		t.Errorf("TTL = %s", ttl)
	}

	if err := c.DeleteByPrefix(ctx, "report:summary:"); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]bool{"report:summary:a": false, "report:summary:b": false, "budgets:all": true} {
		if _, ok, _ := c.Get(ctx, k); ok != want {
			t.Errorf("%s: есть в кеше = %v, ожидали %v", k, ok, want)
		}
	}
	if err := c.DeleteByPrefix(ctx, "nothing:"); err != nil {
		t.Errorf("DeleteByPrefix без ключей: %v", err)
	}

	if err := c.Delete(ctx, "budgets:all"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := c.Get(ctx, "budgets:all"); ok {
		t.Error("ключ не удалён")
	}
}
