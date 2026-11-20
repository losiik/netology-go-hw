package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ledger/internal/domain"
	"ledger/internal/repository/memory"
)

type slowExpenses struct {
	*memory.ExpenseRepo
	delay   time.Duration
	failOn  string
	active  atomic.Int32
	started atomic.Int32
}

func (r *slowExpenses) SumByCategoryPeriod(ctx context.Context, userID int64, category, from, to string) (float64, error) {
	r.started.Add(1)
	r.active.Add(1)
	defer r.active.Add(-1)

	if category == r.failOn {
		return 0, errDB
	}
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-time.After(r.delay):
	}
	return r.ExpenseRepo.SumByCategoryPeriod(ctx, userID, category, from, to)
}

func newSlowService(t *testing.T, delay time.Duration, categories ...string) (Service, *slowExpenses) {
	t.Helper()
	expenses := &slowExpenses{ExpenseRepo: memory.NewExpenseRepo(), delay: delay}
	svc := New(memory.NewBudgetRepo(), expenses, nil)
	for i, c := range categories {
		tx := domain.Transaction{Amount: float64(i + 1), Category: c, Date: "2025-09-10"}
		if _, err := svc.AddTransaction(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
	}
	return svc, expenses
}

func TestReportSummaryParallel(t *testing.T) {
	categories := []string{"еда", "кино", "такси", "связь", "спорт"}
	svc, _ := newSlowService(t, 100*time.Millisecond, categories...)

	start := time.Now()
	got, err := svc.GetReportSummary(context.Background(), "2025-09-01", "2025-09-30")
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)

	if len(got.Categories) != len(categories) || got.Total != 15 {
		t.Errorf("отчёт %+v", got)
	}
	if elapsed > 400*time.Millisecond {
		t.Errorf("расчёт занял %s, категории должны считаться параллельно", elapsed)
	}
}

func TestReportSummaryTimeout(t *testing.T) {
	svc, expenses := newSlowService(t, 5*time.Second, "еда", "кино", "такси")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := svc.GetReportSummary(ctx, "2025-09-01", "2025-09-30")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ожидали context.DeadlineExceeded, получили %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("отмена заняла %s, горутины должны завершаться сразу", elapsed)
	}
	if n := expenses.active.Load(); n != 0 {
		t.Errorf("после возврата работают ещё %d горутин", n)
	}
}

func TestReportSummaryCanceledBeforeStart(t *testing.T) {
	svc, expenses := newSlowService(t, time.Second, "еда", "кино")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := svc.GetReportSummary(ctx, "2025-09-01", "2025-09-30"); !errors.Is(err, context.Canceled) {
		t.Fatalf("ожидали context.Canceled, получили %v", err)
	}
	if n := expenses.started.Load(); n != 0 {
		t.Errorf("с отменённым контекстом запущено %d запросов к репозиторию", n)
	}
}

func TestReportSummaryRepoError(t *testing.T) {
	svc, expenses := newSlowService(t, 5*time.Second, "еда", "кино", "такси")
	expenses.failOn = "кино"

	start := time.Now()
	_, err := svc.GetReportSummary(context.Background(), "2025-09-01", "2025-09-30")
	if !errors.Is(err, errDB) {
		t.Fatalf("ожидали ошибку БД, получили %v", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Error("наружу должна уходить исходная ошибка, а не отмена соседних горутин")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("при ошибке остальные горутины должны отменяться, прошло %s", elapsed)
	}
}

func TestHeartbeat(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(io.Discard)

	old := HeartbeatInterval
	HeartbeatInterval = 10 * time.Millisecond
	defer func() { HeartbeatInterval = old }()

	t.Run("finished", func(t *testing.T) {
		buf.Reset()
		svc, _ := newSlowService(t, 60*time.Millisecond, "еда")
		if _, err := svc.GetReportSummary(context.Background(), "2025-09-01", "2025-09-30"); err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		if !strings.Contains(out, "идёт расчёт отчёта") {
			t.Errorf("нет сообщений heartbeat в логе:\n%s", out)
		}
		if !strings.Contains(out, "тикер остановлен") {
			t.Errorf("тикер не остановлен после расчёта:\n%s", out)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		buf.Reset()
		svc, _ := newSlowService(t, 5*time.Second, "еда")
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
		defer cancel()
		if _, err := svc.GetReportSummary(ctx, "2025-09-01", "2025-09-30"); err == nil {
			t.Fatal("ожидалась ошибка таймаута")
		}
		out := buf.String()
		if !strings.Contains(out, "тикер остановлен") || !strings.Contains(out, "расчёт отчёта прерван") {
			t.Errorf("в логе нет остановки тикера или обрыва расчёта:\n%s", out)
		}
	})

	t.Run("no_ticks_after_stop", func(t *testing.T) {
		buf.Reset()
		stop := startHeartbeat(context.Background(), "test")
		time.Sleep(25 * time.Millisecond)
		stop()
		n := strings.Count(buf.String(), "идёт расчёт")
		time.Sleep(50 * time.Millisecond)
		if m := strings.Count(buf.String(), "идёт расчёт"); m != n {
			t.Errorf("после stop() тикер продолжает писать: было %d, стало %d", n, m)
		}
	})
}
