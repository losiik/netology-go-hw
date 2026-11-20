package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"gateway/internal/api"
	"ledger"
)

const jsonContentType = "application/json; charset=utf-8"

func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

type brokenService struct{}

var errBroken = errors.New("db is down")

func (brokenService) SetBudget(context.Context, ledger.Budget) error { return errBroken }
func (brokenService) ListBudgets(context.Context) ([]ledger.Budget, error) {
	return nil, errBroken
}
func (brokenService) LoadBudgets(context.Context, io.Reader) error { return errBroken }
func (brokenService) AddTransaction(context.Context, ledger.Transaction) (ledger.Transaction, error) {
	return ledger.Transaction{}, errBroken
}
func (brokenService) ListTransactions(context.Context) ([]ledger.Transaction, error) {
	return nil, errBroken
}
func (brokenService) GetReportSummary(context.Context, string, string) (ledger.ReportSummary, error) {
	return ledger.ReportSummary{}, errBroken
}

var _ ledger.Service = brokenService{}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func checkResponse(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Errorf("статус %d, ожидали %d, тело: %s", rec.Code, wantStatus, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != jsonContentType {
		t.Errorf("Content-Type = %q, ожидали %q", ct, jsonContentType)
	}
}

func checkError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantMsg string) {
	t.Helper()
	checkResponse(t, rec, wantStatus)

	var resp api.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("ответ не JSON: %v, тело: %s", err, rec.Body.String())
	}
	if resp.Error == "" {
		t.Errorf("пустое поле error, тело: %s", rec.Body.String())
	}
	if wantMsg != "" && !strings.Contains(resp.Error, wantMsg) {
		t.Errorf("error = %q, ожидали содержит %q", resp.Error, wantMsg)
	}
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("не удалось разобрать ответ: %v, тело: %s", err, rec.Body.String())
	}
	return v
}

func newRouter(t *testing.T) http.Handler {
	t.Helper()
	return api.NewRouter(ledger.NewInMemory(), time.Second)
}

func TestBudgetsAPI(t *testing.T) {
	t.Run("empty_list", func(t *testing.T) {
		h := newRouter(t)
		rec := do(t, h, http.MethodGet, "/api/budgets", "")
		checkResponse(t, rec, http.StatusOK)
		if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
			t.Errorf("тело %q, ожидали []", body)
		}
	})

	t.Run("create_and_list", func(t *testing.T) {
		h := newRouter(t)

		rec := do(t, h, http.MethodPost, "/api/budgets", `{"category":"еда","limit":5000}`)
		checkResponse(t, rec, http.StatusCreated)
		created := decode[api.BudgetResponse](t, rec)
		if created.Category != "еда" || created.Limit != 5000 {
			t.Errorf("создан бюджет %+v", created)
		}

		rec = do(t, h, http.MethodGet, "/api/budgets", "")
		checkResponse(t, rec, http.StatusOK)
		list := decode[[]api.BudgetResponse](t, rec)
		if len(list) != 1 || list[0] != created {
			t.Errorf("список бюджетов %+v, ожидали [%+v]", list, created)
		}
	})

	t.Run("update_limit", func(t *testing.T) {
		h := newRouter(t)
		do(t, h, http.MethodPost, "/api/budgets", `{"category":"еда","limit":1000}`)
		rec := do(t, h, http.MethodPost, "/api/budgets", `{"category":"еда","limit":2000}`)
		checkResponse(t, rec, http.StatusCreated)

		list := decode[[]api.BudgetResponse](t, do(t, h, http.MethodGet, "/api/budgets", ""))
		if len(list) != 1 || list[0].Limit != 2000 {
			t.Errorf("список бюджетов %+v, ожидали один с лимитом 2000", list)
		}
	})

	bad := []struct {
		name string
		body string
		msg  string
	}{
		{"zero_limit", `{"category":"еда","limit":0}`, "лимит"},
		{"negative_limit", `{"category":"еда","limit":-10}`, "лимит"},
		{"empty_category", `{"category":"","limit":100}`, "категория"},
		{"bad_json", `{"category":"еда","limit":`, "неверный JSON"},
		{"wrong_type", `{"category":"еда","limit":"много"}`, "неверный JSON"},
		{"unknown_field", `{"category":"еда","limit":100,"foo":1}`, "неверный JSON"},
		{"empty_body", ``, "неверный JSON"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			h := newRouter(t)
			rec := do(t, h, http.MethodPost, "/api/budgets", tc.body)
			checkError(t, rec, http.StatusBadRequest, tc.msg)

			list := decode[[]api.BudgetResponse](t, do(t, h, http.MethodGet, "/api/budgets", ""))
			if len(list) != 0 {
				t.Errorf("бюджет сохранился: %+v", list)
			}
		})
	}
}

func TestTransactionsAPI(t *testing.T) {
	t.Run("empty_list", func(t *testing.T) {
		h := newRouter(t)
		rec := do(t, h, http.MethodGet, "/api/transactions", "")
		checkResponse(t, rec, http.StatusOK)
		if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
			t.Errorf("тело %q, ожидали []", body)
		}
	})

	t.Run("ok", func(t *testing.T) {
		h := newRouter(t)
		checkResponse(t, do(t, h, http.MethodPost, "/api/budgets", `{"category":"еда","limit":1000}`), http.StatusCreated)

		rec := do(t, h, http.MethodPost, "/api/transactions",
			`{"amount":450,"category":"еда","description":"ланч","date":"2025-09-10"}`)
		checkResponse(t, rec, http.StatusCreated)
		created := decode[api.TransactionResponse](t, rec)
		want := api.TransactionResponse{ID: 1, Amount: 450, Category: "еда", Description: "ланч", Date: "2025-09-10"}
		if created != want {
			t.Errorf("создана транзакция %+v, ожидали %+v", created, want)
		}

		rec = do(t, h, http.MethodGet, "/api/transactions", "")
		checkResponse(t, rec, http.StatusOK)
		list := decode[[]api.TransactionResponse](t, rec)
		if len(list) != 1 || list[0] != want {
			t.Errorf("список %+v, ожидали [%+v]", list, want)
		}
	})

	t.Run("exceeded", func(t *testing.T) {
		h := newRouter(t)
		do(t, h, http.MethodPost, "/api/budgets", `{"category":"еда","limit":1000}`)
		checkResponse(t, do(t, h, http.MethodPost, "/api/transactions",
			`{"amount":800,"category":"еда","date":"2025-09-10"}`), http.StatusCreated)

		rec := do(t, h, http.MethodPost, "/api/transactions",
			`{"amount":300,"category":"еда","date":"2025-09-11"}`)
		checkError(t, rec, http.StatusConflict, "")
		if body := strings.TrimSpace(rec.Body.String()); body != `{"error":"budget exceeded"}` {
			t.Errorf("тело %s, ожидали {\"error\":\"budget exceeded\"}", body)
		}

		list := decode[[]api.TransactionResponse](t, do(t, h, http.MethodGet, "/api/transactions", ""))
		if len(list) != 1 {
			t.Errorf("транзакций %d, ожидали 1", len(list))
		}
	})

	t.Run("no_budget_category", func(t *testing.T) {
		h := newRouter(t)
		rec := do(t, h, http.MethodPost, "/api/transactions",
			`{"amount":100000,"category":"кино","date":"2025-09-10"}`)
		checkResponse(t, rec, http.StatusCreated)
	})

	bad := []struct {
		name string
		body string
		msg  string
	}{
		{"bad_json", `{"amount":450,"category":`, "неверный JSON"},
		{"wrong_type", `{"amount":"450","category":"еда","date":"2025-09-10"}`, "неверный JSON"},
		{"unknown_field", `{"amount":450,"category":"еда","date":"2025-09-10","foo":1}`, "неверный JSON"},
		{"empty_body", ``, "неверный JSON"},
		{"zero_amount", `{"amount":0,"category":"еда","date":"2025-09-10"}`, "сумма"},
		{"negative_amount", `{"amount":-5,"category":"еда","date":"2025-09-10"}`, "сумма"},
		{"empty_category", `{"amount":10,"category":"","date":"2025-09-10"}`, "категория"},
		{"bad_date", `{"amount":10,"category":"еда","date":"10.09.2025"}`, "формат даты"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			h := newRouter(t)
			rec := do(t, h, http.MethodPost, "/api/transactions", tc.body)
			checkError(t, rec, http.StatusBadRequest, tc.msg)

			list := decode[[]api.TransactionResponse](t, do(t, h, http.MethodGet, "/api/transactions", ""))
			if len(list) != 0 {
				t.Errorf("транзакция сохранилась: %+v", list)
			}
		})
	}
}

func TestRouting(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		status int
	}{
		{"ping", http.MethodGet, "/ping", http.StatusOK},
		{"ping_wrong_method", http.MethodPost, "/ping", http.StatusMethodNotAllowed},
		{"transactions_wrong_method", http.MethodDelete, "/api/transactions", http.StatusMethodNotAllowed},
		{"budgets_wrong_method", http.MethodPut, "/api/budgets", http.StatusMethodNotAllowed},
		{"report_wrong_method", http.MethodPost, "/api/reports/summary", http.StatusMethodNotAllowed},
		{"unknown_api_path", http.MethodGet, "/api/unknown", http.StatusNotFound},
		{"unknown_path", http.MethodGet, "/nope", http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := api.NewRouter(ledger.NewInMemory(), time.Second)
			rec := do(t, h, tc.method, tc.path, "")
			checkResponse(t, rec, tc.status)
			if tc.status != http.StatusOK {
				checkError(t, rec, tc.status, "")
			}
		})
	}
}

func TestServer(t *testing.T) {
	srv := httptest.NewServer(newRouter(t))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/budgets", "application/json",
		strings.NewReader(`{"category":"еда","limit":5000}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("статус %d, ожидали 201", resp.StatusCode)
	}

	resp, err = http.Get(srv.URL + "/api/budgets")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != jsonContentType {
		t.Errorf("Content-Type = %q", ct)
	}
	var list []api.BudgetResponse
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Category != "еда" {
		t.Errorf("список %+v", list)
	}
}

func TestReportSummaryAPI(t *testing.T) {
	h := newRouter(t)

	for _, body := range []string{
		`{"amount":100.5,"category":"еда","date":"2025-09-01"}`,
		`{"amount":200,"category":"еда","date":"2025-09-20"}`,
		`{"amount":300,"category":"кино","date":"2025-09-30"}`,
		`{"amount":999,"category":"еда","date":"2025-10-01"}`,
	} {
		checkResponse(t, do(t, h, http.MethodPost, "/api/transactions", body), http.StatusCreated)
	}

	want := api.ReportSummaryResponse{
		From:       "2025-09-01",
		To:         "2025-09-30",
		Total:      600.5,
		Categories: map[string]float64{"еда": 300.5, "кино": 300},
	}
	check := func(t *testing.T, got api.ReportSummaryResponse) {
		t.Helper()
		if got.From != want.From || got.To != want.To || got.Total != want.Total || !maps.Equal(got.Categories, want.Categories) {
			t.Errorf("отчёт %+v, ожидали %+v", got, want)
		}
	}

	t.Run("first_request", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/api/reports/summary?from=2025-09-01&to=2025-09-30", "")
		checkResponse(t, rec, http.StatusOK)
		check(t, decode[api.ReportSummaryResponse](t, rec))
	})

	t.Run("empty_period", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/api/reports/summary?from=2020-01-01&to=2020-01-31", "")
		checkResponse(t, rec, http.StatusOK)
		got := decode[api.ReportSummaryResponse](t, rec)
		if got.Total != 0 || got.Categories == nil || len(got.Categories) != 0 {
			t.Errorf("ожидали пустой отчёт, получили %+v", got)
		}
		if !strings.Contains(rec.Body.String(), `"categories":{}`) {
			t.Errorf("categories должен быть {}, тело: %s", rec.Body.String())
		}
	})

	bad := []struct {
		name  string
		query string
	}{
		{"no_params", ""},
		{"no_to", "?from=2025-09-01"},
		{"bad_from", "?from=01.09.2025&to=2025-09-30"},
		{"from_after_to", "?from=2025-10-01&to=2025-09-01"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, h, http.MethodGet, "/api/reports/summary"+tc.query, "")
			checkError(t, rec, http.StatusBadRequest, "неверный период")
		})
	}
}

func TestServiceSharedBetweenRouters(t *testing.T) {
	svc := ledger.NewInMemory()
	h := api.NewRouter(svc, time.Second)
	checkResponse(t, do(t, h, http.MethodPost, "/api/budgets", `{"category":"еда","limit":500}`), http.StatusCreated)
	checkResponse(t, do(t, h, http.MethodPost, "/api/transactions",
		`{"amount":100,"category":"еда","description":"ланч","date":"2025-09-10"}`), http.StatusCreated)

	h2 := api.NewRouter(svc, time.Second)
	txs := decode[[]api.TransactionResponse](t, do(t, h2, http.MethodGet, "/api/transactions", ""))
	if len(txs) != 1 || txs[0].Description != "ланч" {
		t.Errorf("транзакции %+v", txs)
	}
	budgets := decode[[]api.BudgetResponse](t, do(t, h2, http.MethodGet, "/api/budgets", ""))
	if len(budgets) != 1 || budgets[0].Limit != 500 {
		t.Errorf("бюджеты %+v", budgets)
	}
}

func TestInternalErrors(t *testing.T) {
	h := api.NewRouter(brokenService{}, time.Second)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"create_budget", http.MethodPost, "/api/budgets", `{"category":"еда","limit":100}`},
		{"list_budgets", http.MethodGet, "/api/budgets", ""},
		{"create_transaction", http.MethodPost, "/api/transactions", `{"amount":1,"category":"еда","date":"2025-09-10"}`},
		{"list_transactions", http.MethodGet, "/api/transactions", ""},
		{"report", http.MethodGet, "/api/reports/summary?from=2025-09-01&to=2025-09-30", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, h, tc.method, tc.path, tc.body)
			checkError(t, rec, http.StatusInternalServerError, "internal server error")
			if strings.Contains(rec.Body.String(), errBroken.Error()) {
				t.Errorf("текст внутренней ошибки не должен уходить клиенту: %s", rec.Body.String())
			}
		})
	}
}
