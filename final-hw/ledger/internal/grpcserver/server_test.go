package grpcserver

import (
	"context"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"ledger/internal/domain"
	"ledger/internal/repository/memory"
	"ledger/internal/service"
	"proto/ledgerv1"
)

func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

func startServer(t *testing.T, svc service.Service) *bufconn.Listener {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(LoggingInterceptor, UserInterceptor))
	ledgerv1.RegisterLedgerServiceServer(srv, New(svc))
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)
	return lis
}

func dialAs(t *testing.T, lis *bufconn.Listener, userID string) ledgerv1.LedgerServiceClient {
	t.Helper()
	withUser := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if userID != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, UserIDHeader, userID)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(withUser),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return ledgerv1.NewLedgerServiceClient(conn)
}

func newClient(t *testing.T, svc service.Service) ledgerv1.LedgerServiceClient {
	return dialAs(t, startServer(t, svc), "1")
}

func newMemoryClient(t *testing.T) ledgerv1.LedgerServiceClient {
	return newClient(t, service.New(memory.NewBudgetRepo(), memory.NewExpenseRepo(), nil))
}

func pbTx(amount float64, category, date string) *ledgerv1.AddTransactionRequest {
	return &ledgerv1.AddTransactionRequest{Transaction: &ledgerv1.Transaction{Amount: amount, Category: category, Date: date}}
}

func TestBudgetsAndTransactions(t *testing.T) {
	ctx := context.Background()
	c := newMemoryClient(t)

	if _, err := c.SetBudget(ctx, &ledgerv1.SetBudgetRequest{Budget: &ledgerv1.Budget{Category: "еда", Limit: 500}}); err != nil {
		t.Fatal(err)
	}
	budgets, err := c.ListBudgets(ctx, &ledgerv1.ListBudgetsRequest{})
	if err != nil || len(budgets.GetBudgets()) != 1 || budgets.GetBudgets()[0].GetLimit() != 500 {
		t.Fatalf("ListBudgets = %v, %v", budgets, err)
	}

	created, err := c.AddTransaction(ctx, &ledgerv1.AddTransactionRequest{Transaction: &ledgerv1.Transaction{
		Amount: 450, Category: "еда", Description: "обед", Date: "2025-09-10",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if created.GetTransaction().GetId() != 1 || created.GetTransaction().GetDescription() != "обед" {
		t.Errorf("AddTransaction = %v", created)
	}

	list, err := c.ListTransactions(ctx, &ledgerv1.ListTransactionsRequest{})
	if err != nil || len(list.GetTransactions()) != 1 {
		t.Fatalf("ListTransactions = %v, %v", list, err)
	}
}

func TestErrorCodes(t *testing.T) {
	ctx := context.Background()
	c := newMemoryClient(t)
	if _, err := c.SetBudget(ctx, &ledgerv1.SetBudgetRequest{Budget: &ledgerv1.Budget{Category: "кино", Limit: 100}}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		call func() error
		code codes.Code
		msg  string
	}{
		{"negative_amount", func() error {
			_, err := c.AddTransaction(ctx, pbTx(-1, "еда", "2025-09-10"))
			return err
		}, codes.InvalidArgument, "сумма должна быть больше 0"},
		{"empty_request", func() error {
			_, err := c.AddTransaction(ctx, &ledgerv1.AddTransactionRequest{})
			return err
		}, codes.InvalidArgument, "сумма"},
		{"budget_exceeded", func() error {
			_, err := c.AddTransaction(ctx, pbTx(150, "кино", "2025-09-10"))
			return err
		}, codes.FailedPrecondition, "budget exceeded"},
		{"bad_budget", func() error {
			_, err := c.SetBudget(ctx, &ledgerv1.SetBudgetRequest{Budget: &ledgerv1.Budget{Category: "", Limit: 1}})
			return err
		}, codes.InvalidArgument, "категория"},
		{"bad_period", func() error {
			_, err := c.GetReportSummary(ctx, &ledgerv1.GetReportSummaryRequest{From: "2025-10-01", To: "2025-09-01"})
			return err
		}, codes.InvalidArgument, "неверный период"},
		{"bad_csv", func() error {
			_, err := c.ImportCSV(ctx, &ledgerv1.ImportCSVRequest{Data: []byte("a,b\n1,2\n")})
			return err
		}, codes.InvalidArgument, "нет колонки"},
		{"bad_export_period", func() error {
			_, err := c.ExportCSV(ctx, &ledgerv1.ExportCSVRequest{From: "2025-09-01"})
			return err
		}, codes.InvalidArgument, "неверный период"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			st, _ := status.FromError(err)
			if st.Code() != tc.code || !strings.Contains(st.Message(), tc.msg) {
				t.Errorf("получили %s %q, ожидали %s %q", st.Code(), st.Message(), tc.code, tc.msg)
			}
		})
	}

	list, _ := c.ListTransactions(ctx, &ledgerv1.ListTransactionsRequest{})
	if len(list.GetTransactions()) != 0 {
		t.Errorf("после ошибок в хранилище %d транзакций", len(list.GetTransactions()))
	}
}

func TestReport(t *testing.T) {
	ctx := context.Background()
	c := newMemoryClient(t)
	c.SetBudget(ctx, &ledgerv1.SetBudgetRequest{Budget: &ledgerv1.Budget{Category: "еда", Limit: 1000}})
	c.AddTransaction(ctx, pbTx(250, "еда", "2025-09-10"))
	c.AddTransaction(ctx, pbTx(100, "кино", "2025-09-11"))

	got, err := c.GetReportSummary(ctx, &ledgerv1.GetReportSummaryRequest{From: "2025-09-01", To: "2025-09-30"})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetTotal() != 350 || len(got.GetCategories()) != 2 {
		t.Fatalf("отчёт %v", got)
	}
	food, movie := got.GetCategories()[0], got.GetCategories()[1]
	if food.GetCategory() != "еда" || food.Budget == nil || food.GetBudget() != 1000 || food.GetBudgetPercent() != 25 {
		t.Errorf("еда: %v", food)
	}
	if movie.Budget != nil || movie.BudgetPercent != nil {
		t.Errorf("у кино нет бюджета: %v", movie)
	}
}

func TestImport(t *testing.T) {
	ctx := context.Background()
	c := newMemoryClient(t)

	resp, err := c.ImportTransactions(ctx, &ledgerv1.ImportTransactionsRequest{
		Workers: 2,
		Transactions: []*ledgerv1.Transaction{
			{Amount: 10, Category: "еда", Date: "2025-09-10"},
			{Amount: -1, Category: "еда", Date: "2025-09-10"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := resp.GetSummary()
	if s.GetTotal() != 2 || s.GetAccepted() != 1 || s.GetRejected() != 1 || s.GetErrors()[0].GetIndex() != 1 {
		t.Errorf("сводка %v", s)
	}

	csvResp, err := c.ImportCSV(ctx, &ledgerv1.ImportCSVRequest{
		Data:    []byte("date,amount,category\n2025-09-11,5,такси\n2025-09-12,abc,такси\n"),
		Workers: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	cs := csvResp.GetSummary()
	if cs.GetAccepted() != 1 || cs.GetRejected() != 1 || cs.GetErrors()[0].GetLine() != 3 {
		t.Errorf("сводка CSV %v", cs)
	}

	export, err := c.ExportCSV(ctx, &ledgerv1.ExportCSVRequest{})
	if err != nil {
		t.Fatal(err)
	}
	want := "id,date,amount,category,description\n1,2025-09-10,10.00,еда,\n2,2025-09-11,5.00,такси,\n"
	if string(export.GetData()) != want {
		t.Errorf("экспорт:\n%s", export.GetData())
	}
}

type slowExpenses struct {
	*memory.ExpenseRepo
}

func (slowExpenses) Categories(ctx context.Context, _ int64, _, _ string) ([]string, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (slowExpenses) List(context.Context, int64) ([]domain.Transaction, error) {
	return nil, io.ErrUnexpectedEOF
}

func TestDeadlineAndInternal(t *testing.T) {
	c := newClient(t, service.New(memory.NewBudgetRepo(), slowExpenses{memory.NewExpenseRepo()}, nil))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := c.GetReportSummary(ctx, &ledgerv1.GetReportSummaryRequest{From: "2025-09-01", To: "2025-09-30"})
	if status.Code(err) != codes.DeadlineExceeded {
		t.Errorf("ожидали DeadlineExceeded, получили %v", err)
	}

	_, err = c.ListTransactions(context.Background(), &ledgerv1.ListTransactionsRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal || st.Message() != "internal error" {
		t.Errorf("ожидали Internal без подробностей, получили %v", err)
	}
}

func TestUserIsolation(t *testing.T) {
	ctx := context.Background()
	lis := startServer(t, service.New(memory.NewBudgetRepo(), memory.NewExpenseRepo(), nil))
	ivan := dialAs(t, lis, "1")
	petr := dialAs(t, lis, "2")

	ivan.SetBudget(ctx, &ledgerv1.SetBudgetRequest{Budget: &ledgerv1.Budget{Category: "еда", Limit: 100}})
	if _, err := ivan.AddTransaction(ctx, pbTx(90, "еда", "2025-09-10")); err != nil {
		t.Fatal(err)
	}
	if _, err := petr.AddTransaction(ctx, pbTx(500, "еда", "2025-09-10")); err != nil {
		t.Errorf("бюджет ivan не должен ограничивать petr: %v", err)
	}

	ivanTx, _ := ivan.ListTransactions(ctx, &ledgerv1.ListTransactionsRequest{})
	petrTx, _ := petr.ListTransactions(ctx, &ledgerv1.ListTransactionsRequest{})
	petrBudgets, _ := petr.ListBudgets(ctx, &ledgerv1.ListBudgetsRequest{})
	if len(ivanTx.GetTransactions()) != 1 || len(petrTx.GetTransactions()) != 1 || len(petrBudgets.GetBudgets()) != 0 {
		t.Errorf("данные пользователей смешались: ivan=%v petr=%v petrBudgets=%v", ivanTx, petrTx, petrBudgets)
	}

	report, _ := petr.GetReportSummary(ctx, &ledgerv1.GetReportSummaryRequest{From: "2025-09-01", To: "2025-09-30"})
	if report.GetTotal() != 500 {
		t.Errorf("отчёт petr: %v", report)
	}

	for _, id := range []string{"", "abc", "0", "-5"} {
		c := dialAs(t, lis, id)
		_, err := c.ListBudgets(ctx, &ledgerv1.ListBudgetsRequest{})
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("x-user-id=%q: ожидали Unauthenticated, получили %v", id, err)
		}
	}
}
