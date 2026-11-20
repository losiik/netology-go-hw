package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"ledger/internal/domain"
)

const (
	DefaultImportWorkers = 4
	MaxImportWorkers     = 32
)

type importJob struct {
	index int
	tx    domain.Transaction
}

type importResult struct {
	index int
	err   error
}

func (s *ledgerService) ImportTransactions(ctx context.Context, txs []domain.Transaction, workers int) (domain.ImportSummary, error) {
	if workers <= 0 {
		workers = DefaultImportWorkers
	}
	if workers > MaxImportWorkers {
		workers = MaxImportWorkers
	}
	if workers > len(txs) {
		workers = max(len(txs), 1)
	}
	start := time.Now()
	log.Printf("импорт: %d транзакций, воркеров %d", len(txs), workers)

	jobs := make(chan importJob)
	results := make(chan importResult)

	go func() {
		defer close(jobs)
		for i, tx := range txs {
			select {
			case <-ctx.Done():
				return
			case jobs <- importJob{index: i, tx: tx}:
			}
		}
	}()

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}
				_, err := s.addTransaction(ctx, job.tx)
				results <- importResult{index: job.index, err: err}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var accepted, rejected int64
	var importErrors []domain.ImportError
	done := make(chan struct{})

	go func() {
		defer close(done)
		for r := range results {
			if r.err == nil {
				atomic.AddInt64(&accepted, 1)
				continue
			}
			if ctx.Err() != nil && (errors.Is(r.err, context.Canceled) || errors.Is(r.err, context.DeadlineExceeded)) {
				continue
			}
			atomic.AddInt64(&rejected, 1)
			importErrors = append(importErrors, domain.ImportError{Index: r.index, Error: importErrorText(r.index, r.err)})
		}
	}()
	<-done

	sort.Slice(importErrors, func(i, j int) bool { return importErrors[i].Index < importErrors[j].Index })
	if importErrors == nil {
		importErrors = []domain.ImportError{}
	}
	summary := domain.ImportSummary{
		Total:    len(txs),
		Accepted: int(atomic.LoadInt64(&accepted)),
		Rejected: int(atomic.LoadInt64(&rejected)),
		Errors:   importErrors,
	}

	if summary.Accepted > 0 {
		s.invalidateReports(context.WithoutCancel(ctx))
	}

	if err := ctx.Err(); err != nil {
		log.Printf("импорт прерван: %v, принято %d, отклонено %d из %d", err, summary.Accepted, summary.Rejected, summary.Total)
		return summary, fmt.Errorf("импорт прерван, обработано %d из %d: %w",
			summary.Accepted+summary.Rejected, summary.Total, err)
	}
	log.Printf("импорт завершён за %s: принято %d, отклонено %d", time.Since(start), summary.Accepted, summary.Rejected)
	return summary, nil
}

func importErrorText(index int, err error) string {
	if errors.Is(err, domain.ErrValidation) || errors.Is(err, domain.ErrBudgetExceeded) {
		return err.Error()
	}
	log.Printf("импорт, элемент #%d: %v", index, err)
	return "internal error"
}
