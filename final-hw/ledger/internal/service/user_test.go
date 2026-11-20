package service

import (
	"context"
	"testing"

	"ledger/internal/domain"
	"ledger/internal/repository/memory"
)

func TestUserIsolation(t *testing.T) {
	cache := newFakeCache()
	svc := New(memory.NewBudgetRepo(), memory.NewExpenseRepo(), cache)
	ivan := domain.WithUserID(context.Background(), 1)
	petr := domain.WithUserID(context.Background(), 2)

	if err := svc.SetBudget(ivan, domain.Budget{Category: "еда", Limit: 100}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddTransaction(ivan, domain.Transaction{Amount: 100, Category: "еда", Date: "2025-09-10"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddTransaction(ivan, domain.Transaction{Amount: 1, Category: "еда", Date: "2025-09-10"}); err != domain.ErrBudgetExceeded {
		t.Errorf("ivan: ожидали ErrBudgetExceeded, получили %v", err)
	}
	if _, err := svc.AddTransaction(petr, domain.Transaction{Amount: 1000, Category: "еда", Date: "2025-09-10"}); err != nil {
		t.Errorf("petr: лимит ivan не должен действовать, получили %v", err)
	}

	ivanReport, _ := svc.GetReportSummary(ivan, "2025-09-01", "2025-09-30")
	petrReport, _ := svc.GetReportSummary(petr, "2025-09-01", "2025-09-30")
	if ivanReport.Total != 100 || petrReport.Total != 1000 {
		t.Errorf("отчёты: ivan=%v petr=%v", ivanReport.Total, petrReport.Total)
	}
	if !cache.has(ReportCacheKey(1, "2025-09-01", "2025-09-30")) || !cache.has(ReportCacheKey(2, "2025-09-01", "2025-09-30")) {
		t.Error("у каждого пользователя должен быть свой ключ кеша")
	}

	if _, err := svc.AddTransaction(petr, domain.Transaction{Amount: 5, Category: "кино", Date: "2025-09-11"}); err != nil {
		t.Fatal(err)
	}
	if !cache.has(ReportCacheKey(1, "2025-09-01", "2025-09-30")) {
		t.Error("запись petr не должна сбрасывать кеш ivan")
	}

	petrBudgets, _ := svc.ListBudgets(petr)
	ivanTx, _ := svc.ListTransactions(ivan)
	if len(petrBudgets) != 0 || len(ivanTx) != 1 {
		t.Errorf("petr видит бюджеты %v, у ivan транзакций %d", petrBudgets, len(ivanTx))
	}
}
