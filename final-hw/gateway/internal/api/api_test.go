package api_test

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"gateway/internal/api"
)

func TestPingAndRouting(t *testing.T) {
	e := newEnv(t)
	tests := []struct {
		name   string
		method string
		path   string
		token  string
		status int
	}{
		{"ping", http.MethodGet, "/ping", "", http.StatusOK},
		{"ping_wrong_method", http.MethodPost, "/ping", "", http.StatusMethodNotAllowed},
		{"unknown_path", http.MethodGet, "/nope", "", http.StatusNotFound},
		{"unknown_api_path", http.MethodGet, "/api/unknown", e.token, http.StatusNotFound},
		{"unknown_auth_path", http.MethodGet, "/api/auth/unknown", "", http.StatusNotFound},
		{"transactions_wrong_method", http.MethodDelete, "/api/transactions", e.token, http.StatusMethodNotAllowed},
		{"budgets_wrong_method", http.MethodPut, "/api/budgets", e.token, http.StatusMethodNotAllowed},
		{"report_wrong_method", http.MethodPost, "/api/reports/summary", e.token, http.StatusMethodNotAllowed},
		{"bulk_wrong_method", http.MethodGet, "/api/transactions/bulk", e.token, http.StatusMethodNotAllowed},
		{"import_wrong_method", http.MethodGet, "/api/transactions/import", e.token, http.StatusMethodNotAllowed},
		{"export_wrong_method", http.MethodPost, "/api/transactions/export.csv", e.token, http.StatusMethodNotAllowed},
		{"login_wrong_method", http.MethodGet, "/api/auth/login", "", http.StatusMethodNotAllowed},
		{"register_wrong_method", http.MethodGet, "/api/auth/register", "", http.StatusMethodNotAllowed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := e.doRaw(t, tc.method, tc.path, "", "", tc.token)
			checkStatus(t, rec, tc.status)
			if rec.Header().Get("Content-Type") != jsonContentType {
				t.Errorf("Content-Type = %q", rec.Header().Get("Content-Type"))
			}
		})
	}
	if body := e.doRaw(t, http.MethodGet, "/ping", "", "", "").Body.String(); body != "\"pong\"\n" {
		t.Errorf("ping = %q", body)
	}
}

func TestAuthRequired(t *testing.T) {
	e := newEnv(t)
	protected := []struct{ method, path string }{
		{http.MethodGet, "/api/transactions"},
		{http.MethodPost, "/api/transactions"},
		{http.MethodGet, "/api/budgets"},
		{http.MethodPost, "/api/budgets"},
		{http.MethodGet, "/api/reports/summary?from=2025-09-01&to=2025-09-30"},
		{http.MethodPost, "/api/transactions/bulk"},
		{http.MethodPost, "/api/transactions/import"},
		{http.MethodGet, "/api/transactions/export.csv"},
		{http.MethodGet, "/api/unknown"},
	}
	tokens := []struct {
		name   string
		header string
		msg    string
	}{
		{"no_header", "", "требуется авторизация"},
		{"not_bearer", "Basic aXZhbjpwYXNz", "требуется авторизация"},
		{"empty_bearer", "Bearer ", "требуется авторизация"},
		{"garbage", "Bearer abc.def.ghi", "неверный или просроченный токен"},
		{"expired", "Bearer " + signToken(t, testSecret, "ivan", time.Now().Add(-time.Minute)), "неверный или просроченный токен"},
		{"wrong_secret", "Bearer " + signToken(t, "another-secret-another-secret-123", "ivan", time.Now().Add(time.Hour)), "неверный или просроченный токен"},
		{"alg_none", "Bearer eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJsb2dpbiI6Iml2YW4iLCJpc3MiOiJjYXNoY3JhZnQtYXV0aCIsImV4cCI6NDEwMjQ0NDgwMH0.", "неверный или просроченный токен"},
	}
	for _, p := range protected {
		for _, tk := range tokens {
			t.Run(p.method+p.path+"/"+tk.name, func(t *testing.T) {
				req := httptest.NewRequest(p.method, p.path, strings.NewReader("{}"))
				if tk.header != "" {
					req.Header.Set("Authorization", tk.header)
				}
				rec := httptest.NewRecorder()
				e.h.ServeHTTP(rec, req)
				checkError(t, rec, http.StatusUnauthorized, tk.msg)
				if !strings.HasPrefix(rec.Header().Get("WWW-Authenticate"), "Bearer") {
					t.Error("нет заголовка WWW-Authenticate")
				}
			})
		}
	}

	if len(e.ledger.txs) != 0 || len(e.ledger.budgets) != 0 {
		t.Error("без токена запросы не должны доходить до ledger")
	}
}

func TestAuthFlow(t *testing.T) {
	e := newEnv(t)

	rec := e.doRaw(t, http.MethodPost, "/api/auth/register", "application/json", `{"login":"ivan","password":"password123"}`, "")
	checkStatus(t, rec, http.StatusCreated)
	if got := decode[api.RegisterResponse](t, rec); got.ID != 1 || got.Login != "ivan" {
		t.Errorf("register = %+v", got)
	}

	errorsTests := []struct {
		name   string
		path   string
		body   string
		status int
		msg    string
	}{
		{"register_taken", "/api/auth/register", `{"login":"taken","password":"password123"}`, http.StatusConflict, "уже существует"},
		{"register_short_password", "/api/auth/register", `{"login":"petr","password":"123"}`, http.StatusBadRequest, "не короче 8"},
		{"register_empty", "/api/auth/register", `{"login":"","password":""}`, http.StatusBadRequest, "обязательны"},
		{"register_bad_json", "/api/auth/register", `{"login":`, http.StatusBadRequest, "неверный JSON"},
		{"login_wrong_password", "/api/auth/login", `{"login":"ivan","password":"wrong"}`, http.StatusUnauthorized, "неверный логин или пароль"},
		{"login_empty", "/api/auth/login", `{}`, http.StatusBadRequest, "обязательны"},
		{"login_unknown_field", "/api/auth/login", `{"login":"ivan","password":"password123","role":"admin"}`, http.StatusBadRequest, "неверный JSON"},
	}
	for _, tc := range errorsTests {
		t.Run(tc.name, func(t *testing.T) {
			checkError(t, e.doRaw(t, http.MethodPost, tc.path, "application/json", tc.body, ""), tc.status, tc.msg)
		})
	}

	rec = e.doRaw(t, http.MethodPost, "/api/auth/login", "application/json", `{"login":"ivan","password":"password123"}`, "")
	checkStatus(t, rec, http.StatusOK)
	login := decode[api.LoginResponse](t, rec)
	if login.Token == "" || login.TokenType != "Bearer" {
		t.Fatalf("login = %+v", login)
	}
	if exp, err := time.Parse(time.RFC3339, login.ExpiresAt); err != nil || exp.Before(time.Now()) {
		t.Errorf("expires_at = %q", login.ExpiresAt)
	}

	rec = e.doRaw(t, http.MethodGet, "/api/budgets", "", "", login.Token)
	checkStatus(t, rec, http.StatusOK)
}

func TestBudgetsAndTransactions(t *testing.T) {
	e := newEnv(t)

	rec := e.do(t, http.MethodPost, "/api/budgets", `{"category":"еда","limit":500}`)
	checkStatus(t, rec, http.StatusCreated)
	if got := decode[api.BudgetResponse](t, rec); got.Category != "еда" || got.Limit != 500 {
		t.Errorf("budget = %+v", got)
	}

	rec = e.do(t, http.MethodPost, "/api/transactions", `{"amount":450,"category":"еда","description":"обед","date":"2025-09-10"}`)
	checkStatus(t, rec, http.StatusCreated)
	if got := decode[api.TransactionResponse](t, rec); got.ID != 1 || got.Description != "обед" || got.Date != "2025-09-10" {
		t.Errorf("transaction = %+v", got)
	}

	tests := []struct {
		name   string
		path   string
		body   string
		status int
		msg    string
	}{
		{"budget_exceeded", "/api/transactions", `{"amount":100,"category":"еда","date":"2025-09-11"}`, http.StatusConflict, "budget exceeded"},
		{"negative_amount", "/api/transactions", `{"amount":-1,"category":"еда","date":"2025-09-11"}`, http.StatusBadRequest, "сумма должна быть больше 0"},
		{"bad_json", "/api/transactions", `{"amount":`, http.StatusBadRequest, "неверный JSON"},
		{"wrong_type", "/api/transactions", `{"amount":"сто"}`, http.StatusBadRequest, "неверный JSON"},
		{"unknown_field", "/api/transactions", `{"amount":1,"foo":1}`, http.StatusBadRequest, "неверный JSON"},
		{"empty_body", "/api/transactions", ``, http.StatusBadRequest, "неверный JSON"},
		{"bad_budget", "/api/budgets", `{"category":"","limit":1}`, http.StatusBadRequest, "категория"},
		{"too_big", "/api/budgets", `{"category":"` + strings.Repeat("a", api.MaxJSONBody) + `"}`, http.StatusRequestEntityTooLarge, "больше"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			checkError(t, e.do(t, http.MethodPost, tc.path, tc.body), tc.status, tc.msg)
		})
	}

	if e.ledger.lastUser != "7" {
		t.Errorf("в ledger ушёл x-user-id=%q, ожидали id из токена 7", e.ledger.lastUser)
	}

	txs := decode[[]api.TransactionResponse](t, e.do(t, http.MethodGet, "/api/transactions", ""))
	if len(txs) != 1 {
		t.Errorf("транзакции %+v", txs)
	}
	budgets := decode[[]api.BudgetResponse](t, e.do(t, http.MethodGet, "/api/budgets", ""))
	if len(budgets) != 1 || budgets[0].Limit != 500 {
		t.Errorf("бюджеты %+v", budgets)
	}
}

func TestEmptyLists(t *testing.T) {
	e := newEnv(t)
	for _, path := range []string{"/api/transactions", "/api/budgets"} {
		rec := e.do(t, http.MethodGet, path, "")
		checkStatus(t, rec, http.StatusOK)
		if strings.TrimSpace(rec.Body.String()) != "[]" {
			t.Errorf("%s: ожидали [], получили %s", path, rec.Body.String())
		}
	}
}

func TestReportSummary(t *testing.T) {
	e := newEnv(t)
	rec := e.do(t, http.MethodGet, "/api/reports/summary?from=2025-09-01&to=2025-09-30", "")
	checkStatus(t, rec, http.StatusOK)
	got := decode[api.ReportSummaryResponse](t, rec)
	if got.From != "2025-09-01" || got.Total != 750 || len(got.Categories) != 2 {
		t.Fatalf("отчёт %+v", got)
	}
	if c := got.Categories[0]; c.Budget == nil || *c.Budget != 1000 || *c.BudgetPercent != 45 {
		t.Errorf("еда: %+v", c)
	}
	if !strings.Contains(rec.Body.String(), `{"category":"кино","total":300}`) {
		t.Errorf("для категории без бюджета не должно быть полей budget: %s", rec.Body.String())
	}

	for _, q := range []string{"", "?from=2025-09-01", "?from=01.09.2025&to=2025-09-30", "?from=2025-10-01&to=2025-09-01"} {
		t.Run("bad"+q, func(t *testing.T) {
			e.ledger.lastReport = nil
			checkError(t, e.do(t, http.MethodGet, "/api/reports/summary"+q, ""), http.StatusBadRequest, "неверный период")
			if e.ledger.lastReport != nil {
				t.Error("неверный период должен отсекаться в gateway, не доходя до ledger")
			}
		})
	}
}

func TestBulk(t *testing.T) {
	e := newEnv(t)
	body := `[{"amount":10,"category":"еда","date":"2025-09-10"},{"amount":-5,"category":"еда","date":"2025-09-10"}]`

	rec := e.do(t, http.MethodPost, "/api/transactions/bulk?workers=8", body)
	checkStatus(t, rec, http.StatusOK)
	got := decode[api.ImportSummaryResponse](t, rec)
	if got.Total != 2 || got.Accepted != 1 || got.Rejected != 1 || got.Errors[0].Index != 1 {
		t.Errorf("сводка %+v", got)
	}
	if e.ledger.lastWorkers != 8 {
		t.Errorf("workers=%d", e.ledger.lastWorkers)
	}

	e.do(t, http.MethodPost, "/api/transactions/bulk", `[]`)
	if e.ledger.lastWorkers != api.DefaultWorkers {
		t.Errorf("workers по умолчанию = %d", e.ledger.lastWorkers)
	}
	if body := e.do(t, http.MethodPost, "/api/transactions/bulk", `[]`).Body.String(); !strings.Contains(body, `"errors":[]`) {
		t.Errorf("errors должен быть [], тело: %s", body)
	}

	bad := []struct{ name, query, body, msg string }{
		{"workers_zero", "?workers=0", `[]`, "workers"},
		{"workers_text", "?workers=abc", `[]`, "workers"},
		{"workers_too_many", "?workers=33", `[]`, "workers"},
		{"not_array", "", `{"amount":1}`, "неверный JSON"},
		{"null", "", `null`, "массив"},
		{"unknown_field", "", `[{"amount":1,"x":1}]`, "неверный JSON"},
		{"too_many", "", "[" + strings.TrimSuffix(strings.Repeat(`{"amount":1},`, api.MaxBulkItems+1), ",") + "]", "слишком много"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			checkError(t, e.do(t, http.MethodPost, "/api/transactions/bulk"+tc.query, tc.body), http.StatusBadRequest, tc.msg)
		})
	}
}

func TestImportCSV(t *testing.T) {
	e := newEnv(t)
	csv := "date,amount,category\n2025-09-10,10,еда\n2025-09-11,abc,еда\n"

	t.Run("raw_body", func(t *testing.T) {
		rec := e.doRaw(t, http.MethodPost, "/api/transactions/import?workers=2", "text/csv", csv, e.token)
		checkStatus(t, rec, http.StatusOK)
		got := decode[api.ImportSummaryResponse](t, rec)
		if got.Accepted != 1 || got.Rejected != 1 || got.Errors[0].Line != 3 {
			t.Errorf("сводка %+v", got)
		}
		if string(e.ledger.lastCSV) != csv || e.ledger.lastWorkers != 2 {
			t.Errorf("в ledger ушло %q, workers=%d", e.ledger.lastCSV, e.ledger.lastWorkers)
		}
	})

	t.Run("multipart", func(t *testing.T) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", "bank.csv")
		fw.Write([]byte(csv))
		mw.Close()

		rec := e.doRaw(t, http.MethodPost, "/api/transactions/import", mw.FormDataContentType(), buf.String(), e.token)
		checkStatus(t, rec, http.StatusOK)
		if string(e.ledger.lastCSV) != csv {
			t.Errorf("в ledger ушло %q", e.ledger.lastCSV)
		}
	})

	t.Run("multipart_without_file", func(t *testing.T) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		mw.WriteField("other", "1")
		mw.Close()
		checkError(t, e.doRaw(t, http.MethodPost, "/api/transactions/import", mw.FormDataContentType(), buf.String(), e.token),
			http.StatusBadRequest, "file")
	})

	t.Run("empty", func(t *testing.T) {
		checkError(t, e.doRaw(t, http.MethodPost, "/api/transactions/import", "text/csv", "  \n", e.token), http.StatusBadRequest, "пустой CSV")
	})

	t.Run("bad_header_from_ledger", func(t *testing.T) {
		checkError(t, e.doRaw(t, http.MethodPost, "/api/transactions/import", "text/csv", "a,b\n1,2\n", e.token), http.StatusBadRequest, "нет колонки")
	})

	t.Run("bad_workers", func(t *testing.T) {
		checkError(t, e.doRaw(t, http.MethodPost, "/api/transactions/import?workers=-1", "text/csv", csv, e.token), http.StatusBadRequest, "workers")
	})

	t.Run("too_big", func(t *testing.T) {
		big := "date,amount,category\n" + strings.Repeat("2025-09-10,1,еда\n", api.MaxCSVBody/16+1)
		checkError(t, e.doRaw(t, http.MethodPost, "/api/transactions/import", "text/csv", big, e.token), http.StatusRequestEntityTooLarge, "CSV больше")
	})
}

func TestExportCSV(t *testing.T) {
	e := newEnv(t)

	rec := e.do(t, http.MethodGet, "/api/transactions/export.csv", "")
	checkStatus(t, rec, http.StatusOK)
	if ct := rec.Header().Get("Content-Type"); ct != "text/csv; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="transactions.csv"` {
		t.Errorf("Content-Disposition = %q", cd)
	}
	want := "\xef\xbb\xbfid,date,amount,category,description\n1,2025-09-10,450.00,еда,обед\n"
	if rec.Body.String() != want {
		t.Errorf("тело %q", rec.Body.String())
	}

	rec = e.do(t, http.MethodGet, "/api/transactions/export.csv?from=2025-09-01&to=2025-09-30", "")
	checkStatus(t, rec, http.StatusOK)
	if e.ledger.lastExport.GetFrom() != "2025-09-01" || e.ledger.lastExport.GetTo() != "2025-09-30" {
		t.Errorf("в ledger ушёл период %v", e.ledger.lastExport)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "transactions_2025-09-01_2025-09-30.csv") {
		t.Errorf("Content-Disposition = %q", cd)
	}

	checkError(t, e.do(t, http.MethodGet, "/api/transactions/export.csv?from=2025-09-01", ""), http.StatusBadRequest, "неверный период")
}

func TestGRPCErrorMapping(t *testing.T) {
	tests := []struct {
		code   codes.Code
		status int
		msg    string
	}{
		{codes.InvalidArgument, http.StatusBadRequest, "ошибка из ledger"},
		{codes.FailedPrecondition, http.StatusConflict, "ошибка из ledger"},
		{codes.AlreadyExists, http.StatusConflict, "ошибка из ledger"},
		{codes.NotFound, http.StatusNotFound, "ошибка из ledger"},
		{codes.Unauthenticated, http.StatusUnauthorized, "ошибка из ledger"},
		{codes.PermissionDenied, http.StatusForbidden, "ошибка из ledger"},
		{codes.ResourceExhausted, http.StatusRequestEntityTooLarge, "ошибка из ledger"},
		{codes.DeadlineExceeded, http.StatusGatewayTimeout, "request timeout"},
		{codes.Canceled, http.StatusGatewayTimeout, "request timeout"},
		{codes.Unavailable, http.StatusServiceUnavailable, "service unavailable"},
		{codes.Internal, http.StatusInternalServerError, "internal server error"},
		{codes.Unknown, http.StatusInternalServerError, "internal server error"},
		{codes.Unimplemented, http.StatusInternalServerError, "internal server error"},
	}
	endpoints := []struct{ method, path, body string }{
		{http.MethodGet, "/api/transactions", ""},
		{http.MethodPost, "/api/transactions", `{"amount":1,"category":"еда","date":"2025-09-10"}`},
		{http.MethodGet, "/api/budgets", ""},
		{http.MethodPost, "/api/budgets", `{"category":"еда","limit":1}`},
		{http.MethodGet, "/api/reports/summary?from=2025-09-01&to=2025-09-30", ""},
		{http.MethodPost, "/api/transactions/bulk", `[]`},
		{http.MethodGet, "/api/transactions/export.csv", ""},
	}
	for _, tc := range tests {
		t.Run(tc.code.String(), func(t *testing.T) {
			if got := api.HTTPStatus(tc.code); got != tc.status {
				t.Errorf("HTTPStatus(%s) = %d, ожидали %d", tc.code, got, tc.status)
			}
			e := newEnv(t)
			e.ledger.err = status.Error(tc.code, "ошибка из ledger")
			for _, ep := range endpoints {
				rec := e.do(t, ep.method, ep.path, ep.body)
				checkError(t, rec, tc.status, tc.msg)
				if tc.status >= 500 && strings.Contains(rec.Body.String(), "ошибка из ledger") {
					t.Errorf("%s %s: текст внутренней ошибки ушёл клиенту", ep.method, ep.path)
				}
			}
			rec := e.doRaw(t, http.MethodPost, "/api/transactions/import", "text/csv", "date\n", e.token)
			checkError(t, rec, tc.status, tc.msg)
		})
	}

	if api.HTTPStatus(codes.OK) != http.StatusOK {
		t.Error("OK должен быть 200")
	}
}

func TestTimeout(t *testing.T) {
	e := newEnvWithTimeout(t, 50*time.Millisecond)
	e.ledger.block = true

	start := time.Now()
	rec := e.do(t, http.MethodGet, "/api/reports/summary?from=2025-09-01&to=2025-09-30", "")
	checkError(t, rec, http.StatusGatewayTimeout, "request timeout")
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("ответ через %s", elapsed)
	}

	rec = e.do(t, http.MethodPost, "/api/transactions/bulk", `[{"amount":1,"category":"еда","date":"2025-09-10"}]`)
	checkError(t, rec, http.StatusGatewayTimeout, "request timeout")
}

func TestLedgerDown(t *testing.T) {
	e := newEnv(t)
	e.ledger.err = errors.New("не gRPC-ошибка")
	checkError(t, e.do(t, http.MethodGet, "/api/budgets", ""), http.StatusInternalServerError, "internal server error")
}
