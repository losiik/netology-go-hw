package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func JSONContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		next.ServeHTTP(w, r)
	})
}

const DefaultRequestTimeout = 2 * time.Second

func TimeoutFromEnv() (time.Duration, error) {
	v := os.Getenv("REQUEST_TIMEOUT")
	if v == "" {
		return DefaultRequestTimeout, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("неверный REQUEST_TIMEOUT=%q: %w", v, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("REQUEST_TIMEOUT должен быть больше 0, получено %s", d)
	}
	return d, nil
}

func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()

			next.ServeHTTP(w, r.WithContext(ctx))

			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				log.Printf("%s %s: превышен таймаут %s", r.Method, r.URL.Path, d)
			}
		})
	}
}
