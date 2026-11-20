package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"gateway/internal/api"
	"ledger"
)

type slowService struct {
	brokenService
	done chan struct{}
}

func (s slowService) GetReportSummary(ctx context.Context, from, to string) (ledger.ReportSummary, error) {
	defer close(s.done)
	select {
	case <-ctx.Done():
		return ledger.ReportSummary{}, ctx.Err()
	case <-time.After(5 * time.Second):
		return ledger.ReportSummary{From: from, To: to}, nil
	}
}

type ignoreCtxService struct {
	brokenService
}

func (ignoreCtxService) ListTransactions(context.Context) ([]ledger.Transaction, error) {
	time.Sleep(30 * time.Millisecond)
	return nil, errBroken
}

func TestTimeoutMiddleware(t *testing.T) {
	var deadline time.Time
	var ok bool
	h := api.Timeout(time.Second)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deadline, ok = r.Context().Deadline()
	}))
	start := time.Now()
	do(t, h, http.MethodGet, "/", "")

	if !ok {
		t.Fatal("у контекста запроса нет дедлайна")
	}
	if d := deadline.Sub(start); d < time.Second || d > time.Second+100*time.Millisecond {
		t.Errorf("дедлайн через %s, ожидали ~1s", d)
	}
}

func TestReportTimeout(t *testing.T) {
	svc := slowService{done: make(chan struct{})}
	h := api.NewRouter(svc, 50*time.Millisecond)

	start := time.Now()
	rec := do(t, h, http.MethodGet, "/api/reports/summary?from=2025-09-01&to=2025-09-30", "")
	checkError(t, rec, http.StatusGatewayTimeout, "request timeout")

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("ответ через %s, ожидали около 50ms", elapsed)
	}
	select {
	case <-svc.done:
	case <-time.After(time.Second):
		t.Error("сервис не завершился после таймаута")
	}
}

func TestTimeoutKeepsOtherErrors(t *testing.T) {
	h := api.NewRouter(slowService{done: make(chan struct{})}, 50*time.Millisecond)

	rec := do(t, h, http.MethodGet, "/api/reports/summary?from=bad&to=2025-09-30", "")
	checkError(t, rec, http.StatusBadRequest, "неверный период")

	checkResponse(t, do(t, h, http.MethodGet, "/ping", ""), http.StatusOK)
}

func TestTimeoutWhenServiceIgnoresCtx(t *testing.T) {
	h := api.NewRouter(ignoreCtxService{}, 10*time.Millisecond)
	rec := do(t, h, http.MethodGet, "/api/transactions", "")
	checkError(t, rec, http.StatusGatewayTimeout, "request timeout")
}

func TestTimeoutFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    time.Duration
		wantErr bool
	}{
		{"default", "", api.DefaultRequestTimeout, false},
		{"ms", "50ms", 50 * time.Millisecond, false},
		{"seconds", "5s", 5 * time.Second, false},
		{"bad", "две секунды", 0, true},
		{"zero", "0s", 0, true},
		{"negative", "-1s", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("REQUEST_TIMEOUT", tc.value)
			got, err := api.TimeoutFromEnv()
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("таймаут %s, ожидали %s", got, tc.want)
			}
		})
	}
	if api.DefaultRequestTimeout != 2*time.Second {
		t.Errorf("таймаут по умолчанию %s, ожидали 2s", api.DefaultRequestTimeout)
	}
}
