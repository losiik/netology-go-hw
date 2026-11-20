package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"sync"
	"time"

	"ledger/internal/domain"
)

var HeartbeatInterval = 300 * time.Millisecond

func (s *ledgerService) GetReportSummary(ctx context.Context, from, to string) (domain.ReportSummary, error) {
	if err := domain.ValidatePeriod(from, to); err != nil {
		return domain.ReportSummary{}, domain.NewValidationError(err)
	}

	key := ReportCacheKey(domain.UserIDFromContext(ctx), from, to)
	var report domain.ReportSummary
	if s.cacheGet(ctx, key, &report) {
		return report, nil
	}

	sums, err := s.calcSummary(ctx, from, to)
	if err != nil {
		return domain.ReportSummary{}, err
	}

	budgets, err := s.ListBudgets(ctx)
	if err != nil {
		return domain.ReportSummary{}, err
	}

	report = buildReport(from, to, sums, budgets)
	s.cacheSet(ctx, key, report, ReportCacheTTL)
	return report, nil
}

func buildReport(from, to string, sums map[string]float64, budgets []domain.Budget) domain.ReportSummary {
	limits := make(map[string]float64, len(budgets))
	for _, b := range budgets {
		limits[b.Category] = b.Limit
		if _, ok := sums[b.Category]; !ok {
			sums[b.Category] = 0
		}
	}

	report := domain.ReportSummary{From: from, To: to, Categories: make([]domain.CategoryReport, 0, len(sums))}
	var total int64
	for c, sum := range sums {
		item := domain.CategoryReport{Category: c, Total: float64(cents(sum)) / 100}
		if limit, ok := limits[c]; ok {
			percent := math.Round(sum/limit*10000) / 100
			item.Budget = &limit
			item.BudgetPercent = &percent
		}
		report.Categories = append(report.Categories, item)
		total += cents(sum)
	}
	sort.Slice(report.Categories, func(i, j int) bool {
		return report.Categories[i].Category < report.Categories[j].Category
	})
	report.Total = float64(total) / 100
	return report
}

func (s *ledgerService) calcSummary(parent context.Context, from, to string) (map[string]float64, error) {
	start := time.Now()
	stop := startHeartbeat(parent, from+".."+to)
	defer stop()

	userID := domain.UserIDFromContext(parent)
	categories, err := s.expenses.Categories(parent, userID, from, to)
	if err != nil {
		if parent.Err() != nil {
			log.Printf("расчёт отчёта прерван: %v", parent.Err())
			return nil, parent.Err()
		}
		return nil, err
	}
	log.Printf("расчёт отчёта %s..%s: категорий %d", from, to, len(categories))

	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	sums := make([]float64, len(categories))
	errs := make([]error, len(categories))

	var wg sync.WaitGroup
	for i, c := range categories {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				errs[i] = ctx.Err()
				return
			default:
			}
			sum, err := s.expenses.SumByCategoryPeriod(ctx, userID, c, from, to)
			if err != nil {
				errs[i] = fmt.Errorf("сумма по категории %q: %w", c, err)
				cancel()
				return
			}
			sums[i] = sum
		}()
	}
	wg.Wait()

	if err := parent.Err(); err != nil {
		log.Printf("расчёт отчёта прерван: %v", err)
		return nil, err
	}
	for _, err := range errs {
		if err != nil && !errors.Is(err, context.Canceled) {
			return nil, err
		}
	}

	result := make(map[string]float64, len(categories))
	for i, c := range categories {
		result[c] = sums[i]
	}
	log.Printf("расчёт отчёта %s..%s завершён за %s", from, to, time.Since(start))
	return result, nil
}

func startHeartbeat(ctx context.Context, name string) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	go func() {
		defer close(done)
		ticker := time.NewTicker(HeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Printf("heartbeat %s: тикер остановлен", name)
				return
			case <-ticker.C:
				log.Printf("heartbeat %s: идёт расчёт отчёта", name)
			}
		}
	}()

	return func() {
		cancel()
		<-done
	}
}
