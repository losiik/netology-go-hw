package api_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"gateway/internal/api"
	"ledger"
)

type recordingService struct {
	brokenService
	mu      sync.Mutex
	workers int
	count   int
}

func (s *recordingService) ImportTransactions(_ context.Context, txs []ledger.Transaction, workers int) (ledger.ImportSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workers = workers
	s.count = len(txs)
	return ledger.ImportSummary{Total: len(txs), Accepted: len(txs)}, nil
}

type slowImportService struct {
	brokenService
}

func (slowImportService) ImportTransactions(ctx context.Context, txs []ledger.Transaction, _ int) (ledger.ImportSummary, error) {
	<-ctx.Done()
	return ledger.ImportSummary{
		Total:    len(txs),
		Accepted: 1,
		Rejected: 1,
		Errors:   []ledger.ImportError{{Index: 1, Error: "budget exceeded"}},
	}, fmt.Errorf("импорт прерван: %w", ctx.Err())
}

func TestBulkImport(t *testing.T) {
	h := newRouter(t)
	checkResponse(t, do(t, h, http.MethodPost, "/api/budgets", `{"category":"кино","limit":100}`), http.StatusCreated)

	body := `[
		{"amount":10,"category":"еда","description":"кофе","date":"2025-09-10"},
		{"amount":-5,"category":"еда","date":"2025-09-10"},
		{"amount":80,"category":"кино","date":"2025-09-11"},
		{"amount":150,"category":"кино","date":"2025-09-12"},
		{"amount":1,"category":"еда","date":"10.09.2025"}
	]`
	rec := do(t, h, http.MethodPost, "/api/transactions/bulk?workers=2", body)
	checkResponse(t, rec, http.StatusOK)

	got := decode[api.ImportSummaryResponse](t, rec)
	if got.Total != 5 || got.Accepted != 2 || got.Rejected != 3 {
		t.Errorf("сводка %+v", got)
	}
	wantIdx := []int{1, 3, 4}
	if len(got.Errors) != len(wantIdx) {
		t.Fatalf("ошибки %+v", got.Errors)
	}
	for i, idx := range wantIdx {
		if got.Errors[i].Index != idx {
			t.Errorf("errors[%d].index = %d, ожидали %d", i, got.Errors[i].Index, idx)
		}
	}
	if got.Errors[1].Error != "budget exceeded" {
		t.Errorf("errors[1] = %q", got.Errors[1].Error)
	}

	txs := decode[[]api.TransactionResponse](t, do(t, h, http.MethodGet, "/api/transactions", ""))
	if len(txs) != 2 {
		t.Errorf("после импорта %d транзакций, ожидали 2", len(txs))
	}
}

func TestBulkImportAllValid(t *testing.T) {
	h := newRouter(t)
	items := make([]string, 50)
	for i := range items {
		items[i] = fmt.Sprintf(`{"amount":%d,"category":"cat%d","date":"2025-09-10"}`, i+1, i%5)
	}
	rec := do(t, h, http.MethodPost, "/api/transactions/bulk?workers=8", "["+strings.Join(items, ",")+"]")
	checkResponse(t, rec, http.StatusOK)
	got := decode[api.ImportSummaryResponse](t, rec)
	if got.Accepted != 50 || got.Rejected != 0 || got.Errors == nil || len(got.Errors) != 0 {
		t.Errorf("сводка %+v", got)
	}
	if !strings.Contains(rec.Body.String(), `"errors":[]`) {
		t.Errorf("errors должен быть [], тело: %s", rec.Body.String())
	}
}

func TestBulkWorkersParam(t *testing.T) {
	tests := []struct {
		query string
		want  int
	}{
		{"", api.DefaultBulkWorkers},
		{"?workers=1", 1},
		{"?workers=16", 16},
	}
	for _, tc := range tests {
		t.Run("workers"+tc.query, func(t *testing.T) {
			svc := &recordingService{}
			rec := do(t, api.NewRouter(svc, time.Second), http.MethodPost, "/api/transactions/bulk"+tc.query,
				`[{"amount":1,"category":"еда","date":"2025-09-10"}]`)
			checkResponse(t, rec, http.StatusOK)
			if svc.workers != tc.want || svc.count != 1 {
				t.Errorf("workers=%d count=%d, ожидали workers=%d", svc.workers, svc.count, tc.want)
			}
		})
	}
	if api.DefaultBulkWorkers != 4 {
		t.Errorf("DefaultBulkWorkers = %d", api.DefaultBulkWorkers)
	}
}

func TestBulkBadRequest(t *testing.T) {
	tests := []struct {
		name  string
		query string
		body  string
		msg   string
	}{
		{"workers_zero", "?workers=0", `[]`, "workers"},
		{"workers_negative", "?workers=-1", `[]`, "workers"},
		{"workers_text", "?workers=abc", `[]`, "workers"},
		{"workers_too_many", "?workers=1000", `[]`, "workers"},
		{"not_array", "", `{"amount":1,"category":"еда","date":"2025-09-10"}`, "неверный JSON"},
		{"null", "", `null`, "массив"},
		{"empty_body", "", ``, "неверный JSON"},
		{"broken_json", "", `[{"amount":1,`, "неверный JSON"},
		{"unknown_field", "", `[{"amount":1,"category":"еда","date":"2025-09-10","extra":1}]`, "неверный JSON"},
		{"wrong_type", "", `[{"amount":"сто","category":"еда","date":"2025-09-10"}]`, "неверный JSON"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, newRouter(t), http.MethodPost, "/api/transactions/bulk"+tc.query, tc.body)
			checkError(t, rec, http.StatusBadRequest, tc.msg)
		})
	}
}

func TestBulkTooMany(t *testing.T) {
	body := "[" + strings.TrimSuffix(strings.Repeat(`{"amount":1,"category":"еда","date":"2025-09-10"},`, api.MaxBulkItems+1), ",") + "]"
	rec := do(t, newRouter(t), http.MethodPost, "/api/transactions/bulk", body)
	checkError(t, rec, http.StatusBadRequest, "слишком много")
}

func TestBulkEmpty(t *testing.T) {
	rec := do(t, newRouter(t), http.MethodPost, "/api/transactions/bulk", `[]`)
	checkResponse(t, rec, http.StatusOK)
	got := decode[api.ImportSummaryResponse](t, rec)
	if got.Total != 0 || got.Accepted != 0 || got.Rejected != 0 {
		t.Errorf("сводка %+v", got)
	}
}

func TestBulkTimeout(t *testing.T) {
	h := api.NewRouter(slowImportService{}, 30*time.Millisecond)
	rec := do(t, h, http.MethodPost, "/api/transactions/bulk",
		`[{"amount":1,"category":"еда","date":"2025-09-10"},{"amount":2,"category":"еда","date":"2025-09-10"}]`)
	checkError(t, rec, http.StatusGatewayTimeout, "request timeout")

	got := decode[api.ImportTimeoutResponse](t, rec)
	if got.Total != 2 || got.Accepted != 1 || got.Rejected != 1 || len(got.Errors) != 1 {
		t.Errorf("в ответе на таймаут нет частичной сводки: %s", rec.Body.String())
	}
}
