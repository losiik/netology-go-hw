package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"gateway/internal/api"
	"gateway/internal/auth"
	"proto/authv1"
	"proto/ledgerv1"
)

const (
	testSecret      = "test-secret-test-secret-test-secret"
	jsonContentType = "application/json; charset=utf-8"
)

func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

type fakeLedger struct {
	ledgerv1.UnimplementedLedgerServiceServer

	mu      sync.Mutex
	budgets map[string]float64
	txs     []*ledgerv1.Transaction
	err     error
	block   bool

	lastUser    string
	lastWorkers int32
	lastCSV     []byte
	lastExport  *ledgerv1.ExportCSVRequest
	lastReport  *ledgerv1.GetReportSummaryRequest
}

func newFakeLedger() *fakeLedger {
	return &fakeLedger{budgets: map[string]float64{}}
}

func (f *fakeLedger) check(ctx context.Context) error {
	md, _ := metadata.FromIncomingContext(ctx)
	f.mu.Lock()
	f.lastUser = strings.Join(md.Get(auth.UserIDHeader), ",")
	f.mu.Unlock()
	if f.block {
		<-ctx.Done()
		return status.FromContextError(ctx.Err()).Err()
	}
	return f.err
}

func (f *fakeLedger) SetBudget(ctx context.Context, req *ledgerv1.SetBudgetRequest) (*ledgerv1.SetBudgetResponse, error) {
	if err := f.check(ctx); err != nil {
		return nil, err
	}
	b := req.GetBudget()
	if b.GetCategory() == "" || b.GetLimit() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "категория бюджета не может быть пустой")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.budgets[b.GetCategory()] = b.GetLimit()
	return &ledgerv1.SetBudgetResponse{Budget: b}, nil
}

func (f *fakeLedger) ListBudgets(ctx context.Context, _ *ledgerv1.ListBudgetsRequest) (*ledgerv1.ListBudgetsResponse, error) {
	if err := f.check(ctx); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	resp := &ledgerv1.ListBudgetsResponse{}
	for c, l := range f.budgets {
		resp.Budgets = append(resp.Budgets, &ledgerv1.Budget{Category: c, Limit: l})
	}
	sort.Slice(resp.Budgets, func(i, j int) bool { return resp.Budgets[i].Category < resp.Budgets[j].Category })
	return resp, nil
}

func (f *fakeLedger) AddTransaction(ctx context.Context, req *ledgerv1.AddTransactionRequest) (*ledgerv1.AddTransactionResponse, error) {
	if err := f.check(ctx); err != nil {
		return nil, err
	}
	tx := req.GetTransaction()
	if tx.GetAmount() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "сумма должна быть больше 0")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if limit, ok := f.budgets[tx.GetCategory()]; ok {
		var spent float64
		for _, t := range f.txs {
			if t.GetCategory() == tx.GetCategory() {
				spent += t.GetAmount()
			}
		}
		if spent+tx.GetAmount() > limit {
			return nil, status.Error(codes.FailedPrecondition, "budget exceeded")
		}
	}
	tx.Id = int64(len(f.txs) + 1)
	f.txs = append(f.txs, tx)
	return &ledgerv1.AddTransactionResponse{Transaction: tx}, nil
}

func (f *fakeLedger) ListTransactions(ctx context.Context, _ *ledgerv1.ListTransactionsRequest) (*ledgerv1.ListTransactionsResponse, error) {
	if err := f.check(ctx); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return &ledgerv1.ListTransactionsResponse{Transactions: f.txs}, nil
}

func (f *fakeLedger) GetReportSummary(ctx context.Context, req *ledgerv1.GetReportSummaryRequest) (*ledgerv1.GetReportSummaryResponse, error) {
	if err := f.check(ctx); err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.lastReport = req
	f.mu.Unlock()
	budget, percent := 1000.0, 45.0
	return &ledgerv1.GetReportSummaryResponse{
		From: req.GetFrom(), To: req.GetTo(), Total: 750,
		Categories: []*ledgerv1.CategoryReport{
			{Category: "еда", Total: 450, Budget: &budget, BudgetPercent: &percent},
			{Category: "кино", Total: 300},
		},
	}, nil
}

func (f *fakeLedger) ImportTransactions(ctx context.Context, req *ledgerv1.ImportTransactionsRequest) (*ledgerv1.ImportTransactionsResponse, error) {
	if err := f.check(ctx); err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.lastWorkers = req.GetWorkers()
	f.mu.Unlock()
	s := &ledgerv1.ImportSummary{Total: int32(len(req.GetTransactions()))}
	for i, tx := range req.GetTransactions() {
		if tx.GetAmount() <= 0 {
			s.Rejected++
			s.Errors = append(s.Errors, &ledgerv1.ImportError{Index: int32(i), Error: "сумма должна быть больше 0"})
			continue
		}
		s.Accepted++
	}
	return &ledgerv1.ImportTransactionsResponse{Summary: s}, nil
}

func (f *fakeLedger) ImportCSV(ctx context.Context, req *ledgerv1.ImportCSVRequest) (*ledgerv1.ImportCSVResponse, error) {
	if err := f.check(ctx); err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.lastCSV = req.GetData()
	f.lastWorkers = req.GetWorkers()
	f.mu.Unlock()
	if !strings.HasPrefix(string(req.GetData()), "date") {
		return nil, status.Error(codes.InvalidArgument, "в заголовке CSV нет колонки \"date\"")
	}
	return &ledgerv1.ImportCSVResponse{Summary: &ledgerv1.ImportSummary{
		Total: 2, Accepted: 1, Rejected: 1,
		Errors: []*ledgerv1.ImportError{{Index: 1, Line: 3, Error: "неверная сумма \"abc\""}},
	}}, nil
}

func (f *fakeLedger) ExportCSV(ctx context.Context, req *ledgerv1.ExportCSVRequest) (*ledgerv1.ExportCSVResponse, error) {
	if err := f.check(ctx); err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.lastExport = req
	f.mu.Unlock()
	return &ledgerv1.ExportCSVResponse{Data: []byte("id,date,amount,category,description\n1,2025-09-10,450.00,еда,обед\n")}, nil
}

type fakeAuth struct {
	authv1.UnimplementedAuthServiceServer
}

func (fakeAuth) Register(_ context.Context, req *authv1.RegisterRequest) (*authv1.RegisterResponse, error) {
	switch {
	case req.GetLogin() == "taken":
		return nil, status.Error(codes.AlreadyExists, "пользователь с таким логином уже существует")
	case len(req.GetPassword()) < 8:
		return nil, status.Error(codes.InvalidArgument, "пароль должен быть не короче 8 символов")
	}
	return &authv1.RegisterResponse{Id: 1, Login: req.GetLogin()}, nil
}

func (fakeAuth) Login(_ context.Context, req *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	if req.GetLogin() != "ivan" || req.GetPassword() != "password123" {
		return nil, status.Error(codes.Unauthenticated, "неверный логин или пароль")
	}
	exp := time.Now().Add(time.Hour)
	return &authv1.LoginResponse{Token: signToken(nil, testSecret, "ivan", exp), ExpiresAt: exp.Unix()}, nil
}

func signToken(t *testing.T, secret, login string, exp time.Time) string {
	claims := auth.Claims{
		Login: login,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "7",
			Issuer:    auth.Issuer,
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil && t != nil {
		t.Fatal(err)
	}
	return s
}

func dialBuf(t *testing.T, register func(*grpc.Server)) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	register(srv)
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(auth.UserMetadataInterceptor),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

type testEnv struct {
	h      http.Handler
	ledger *fakeLedger
	token  string
}

func newEnvWithTimeout(t *testing.T, timeout time.Duration) *testEnv {
	t.Helper()
	ledger := newFakeLedger()
	ledgerConn := dialBuf(t, func(s *grpc.Server) { ledgerv1.RegisterLedgerServiceServer(s, ledger) })
	authConn := dialBuf(t, func(s *grpc.Server) { authv1.RegisterAuthServiceServer(s, fakeAuth{}) })

	verifier, err := auth.NewVerifier(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	h := api.NewRouter(api.Deps{
		Ledger:   ledgerv1.NewLedgerServiceClient(ledgerConn),
		Auth:     authv1.NewAuthServiceClient(authConn),
		Verifier: verifier,
		Timeout:  timeout,
	})
	return &testEnv{h: h, ledger: ledger, token: signToken(t, testSecret, "ivan", time.Now().Add(time.Hour))}
}

func newEnv(t *testing.T) *testEnv {
	return newEnvWithTimeout(t, 2*time.Second)
}

func (e *testEnv) doRaw(t *testing.T, method, path, contentType, body string, token string) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func (e *testEnv) do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return e.doRaw(t, method, path, "application/json", body, e.token)
}

func checkStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Errorf("статус %d, ожидали %d, тело: %s", rec.Code, want, rec.Body.String())
	}
}

func checkError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantMsg string) {
	t.Helper()
	checkStatus(t, rec, wantStatus)
	if ct := rec.Header().Get("Content-Type"); ct != jsonContentType {
		t.Errorf("Content-Type = %q", ct)
	}
	var resp api.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("ответ не JSON: %v, тело: %s", err, rec.Body.String())
	}
	if resp.Error == "" || !strings.Contains(resp.Error, wantMsg) {
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
