package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"gateway/internal/auth"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start))
	})
}

func JSONContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		next.ServeHTTP(w, r)
	})
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

func RequireAuth(v *auth.Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			token, ok := strings.CutPrefix(header, "Bearer ")
			if !ok || strings.TrimSpace(token) == "" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="cashcraft"`)
				writeError(w, http.StatusUnauthorized, "требуется авторизация: заголовок Authorization: Bearer <token>")
				return
			}

			claims, err := v.Verify(strings.TrimSpace(token))
			if err != nil {
				w.Header().Set("WWW-Authenticate", `Bearer realm="cashcraft", error="invalid_token"`)
				writeError(w, http.StatusUnauthorized, err.Error())
				return
			}

			next.ServeHTTP(w, r.WithContext(auth.WithClaims(r.Context(), claims)))
		})
	}
}
