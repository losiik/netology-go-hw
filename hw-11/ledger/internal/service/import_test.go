package service

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ledger/internal/domain"
	"ledger/internal/repository/memory"
)

type slowAddExpenses struct {
	*memory.ExpenseRepo
	delay     time.Duration
	active    atomic.Int32
	maxActive atomic.Int32
}

func (r *slowAddExpenses) Add(ctx context.Context, tx domain.Transaction) (domain.Transaction, error) {
	n := r.active.Add(1)
	defer r.active.Add(-1)
	for {
		m := r.maxActive.Load()
		if n <= m || r.maxActive.CompareAndSwap(m, n) {
			break
		}
	}
	select {
	case <-ctx.Done():
		return domain.Transaction{}, ctx.Err()
	case <-time.After(r.delay):
	}
	return r.ExpenseRepo.Add(ctx, tx)
}

func tx(amount float64, category string) domain.Transaction {
	return domain.Transaction{Amount: amount, Category: category, Date: "2025-09-10"}
}

func TestImportTransactions(t *testing.T) {
	ctx := context.Background()
	svc := newService()
	if err := svc.SetBudget(ctx, domain.Budget{Category: "кино", Limit: 100}); err != nil {
		t.Fatal(err)
	}

	input := []domain.Transaction{
		tx(10, "еда"),
		tx(-5, "еда"),
		tx(80, "кино"),
		{Amount: 1, Category: "еда", Date: "10.09.2025"},
		tx(150, "кино"),
		tx(20, "кино"),
		tx(1, ""),
	}
	got, err := svc.ImportTransactions(ctx, input, 3)
	if err != nil {
		t.Fatal(err)
	}

	if got.Total != 7 || got.Accepted != 3 || got.Rejected != 4 {
		t.Errorf("сводка %+v, ожидали total=7 accepted=3 rejected=4", got)
	}
	wantIdx := []int{1, 3, 4, 6}
	if len(got.Errors) != len(wantIdx) {
		t.Fatalf("ошибки %+v", got.Errors)
	}
	for i, idx := range wantIdx {
		if got.Errors[i].Index != idx || got.Errors[i].Error == "" {
			t.Errorf("errors[%d] = %+v, ожидали index=%d", i, got.Errors[i], idx)
		}
	}
	if got.Errors[2].Error != domain.ErrBudgetExceeded.Error() {
		t.Errorf("для #4 ожидали %q, получили %q", domain.ErrBudgetExceeded, got.Errors[2].Error)
	}
	if !strings.Contains(got.Errors[1].Error, "формат даты") {
		t.Errorf("для #3 ожидали ошибку даты, получили %q", got.Errors[1].Error)
	}

	list, _ := svc.ListTransactions(ctx)
	if len(list) != 3 {
		t.Errorf("в хранилище %d транзакций, ожидали 3", len(list))
	}
}

func TestImportEmpty(t *testing.T) {
	got, err := newService().ImportTransactions(context.Background(), nil, 4)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 0 || got.Accepted != 0 || got.Rejected != 0 || got.Errors == nil {
		t.Errorf("сводка %+v", got)
	}
}

func TestImportBudgetUnderLoad(t *testing.T) {
	ctx := context.Background()
	svc := newService()
	if err := svc.SetBudget(ctx, domain.Budget{Category: "еда", Limit: 100}); err != nil {
		t.Fatal(err)
	}

	input := make([]domain.Transaction, 200)
	for i := range input {
		input[i] = tx(1, "еда")
	}
	got, err := svc.ImportTransactions(ctx, input, 16)
	if err != nil {
		t.Fatal(err)
	}
	if got.Accepted != 100 || got.Rejected != 100 || len(got.Errors) != 100 {
		t.Errorf("accepted=%d rejected=%d errors=%d, ожидали 100/100/100", got.Accepted, got.Rejected, len(got.Errors))
	}
	for i := 1; i < len(got.Errors); i++ {
		if got.Errors[i-1].Index >= got.Errors[i].Index {
			t.Fatal("ошибки должны быть отсортированы по индексу")
		}
	}
}

func TestImportWorkers(t *testing.T) {
	tests := []struct {
		name    string
		workers int
		items   int
		want    int32
	}{
		{"one", 1, 8, 1},
		{"four", 4, 8, 4},
		{"default", 0, 10, DefaultImportWorkers},
		{"more_than_items", 10, 3, 3},
		{"max", 1000, 40, MaxImportWorkers},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			expenses := &slowAddExpenses{ExpenseRepo: memory.NewExpenseRepo(), delay: 30 * time.Millisecond}
			svc := New(memory.NewBudgetRepo(), expenses, nil)

			input := make([]domain.Transaction, tc.items)
			for i := range input {
				input[i] = tx(1, fmt.Sprintf("cat%d", i))
			}
			got, err := svc.ImportTransactions(context.Background(), input, tc.workers)
			if err != nil {
				t.Fatal(err)
			}
			if got.Accepted != tc.items {
				t.Errorf("accepted=%d, ожидали %d", got.Accepted, tc.items)
			}
			if m := expenses.maxActive.Load(); m != tc.want {
				t.Errorf("одновременно работало %d воркеров, ожидали %d", m, tc.want)
			}
		})
	}
}

func TestImportTimeout(t *testing.T) {
	before := runtime.NumGoroutine()

	expenses := &slowAddExpenses{ExpenseRepo: memory.NewExpenseRepo(), delay: 40 * time.Millisecond}
	svc := New(memory.NewBudgetRepo(), expenses, nil)
	input := make([]domain.Transaction, 100)
	for i := range input {
		input[i] = tx(1, fmt.Sprintf("cat%d", i))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	got, err := svc.ImportTransactions(ctx, input, 4)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ожидали context.DeadlineExceeded, получили %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("импорт остановился через %s", elapsed)
	}
	if got.Total != 100 || got.Accepted == 0 || got.Accepted >= 100 {
		t.Errorf("ожидали частичный результат, получили %+v", got)
	}
	if got.Rejected != 0 || len(got.Errors) != 0 {
		t.Errorf("отменённые элементы не должны попадать в rejected: %+v", got)
	}
	list, _ := svc.ListTransactions(context.Background())
	if len(list) != got.Accepted {
		t.Errorf("в хранилище %d, в сводке accepted=%d", len(list), got.Accepted)
	}

	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		t.Errorf("горутины не завершились: было %d, стало %d", before, n)
	}
}

func TestImportCanceledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc := newService()
	got, err := svc.ImportTransactions(ctx, []domain.Transaction{tx(1, "еда"), tx(2, "еда")}, 2)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ожидали context.Canceled, получили %v", err)
	}
	if got.Accepted != 0 || got.Total != 2 {
		t.Errorf("сводка %+v", got)
	}
}

func TestImportInternalError(t *testing.T) {
	svc := New(brokenRepo{}, memory.NewExpenseRepo(), nil)
	got, err := svc.ImportTransactions(context.Background(), []domain.Transaction{tx(1, "еда")}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Rejected != 1 || got.Errors[0].Error != "internal error" {
		t.Errorf("сводка %+v", got)
	}
	if strings.Contains(got.Errors[0].Error, errDB.Error()) {
		t.Error("текст ошибки БД не должен попадать в сводку")
	}
}
