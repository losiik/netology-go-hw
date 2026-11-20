package api_test

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"gateway/internal/api"
	"ledger"
)

const jsonContentType = "application/json; charset=utf-8"

func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

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
	ledger.Reset()
	t.Cleanup(ledger.Reset)
	return api.NewRouter()
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
		{"unknown_api_path", http.MethodGet, "/api/unknown", http.StatusNotFound},
		{"unknown_path", http.MethodGet, "/nope", http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newRouter(t)
			rec := do(t, h, tc.method, tc.path, "")
			checkResponse(t, rec, tc.status)
			if tc.status != http.StatusOK {
				checkError(t, rec, tc.status, "")
			}
		})
	}
}

func TestServer(t *testing.T) {
	ledger.Reset()
	t.Cleanup(ledger.Reset)

	srv := httptest.NewServer(api.NewRouter())
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
