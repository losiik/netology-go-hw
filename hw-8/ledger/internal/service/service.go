package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"sync"
	"time"

	"ledger/internal/domain"
)

const (
	BudgetsCacheKey = "budgets:all"
	BudgetsCacheTTL = 30 * time.Second
	ReportCacheTTL  = 30 * time.Second
)

func ReportCacheKey(from, to string) string {
	return "report:summary:" + from + ":" + to
}

type Service interface {
	SetBudget(ctx context.Context, b domain.Budget) error
	ListBudgets(ctx context.Context) ([]domain.Budget, error)
	LoadBudgets(ctx context.Context, r io.Reader) error
	AddTransaction(ctx context.Context, tx domain.Transaction) (domain.Transaction, error)
	ListTransactions(ctx context.Context) ([]domain.Transaction, error)
	GetReportSummary(ctx context.Context, from, to string) (domain.ReportSummary, error)
}

type ledgerService struct {
	budgets  domain.BudgetRepository
	expenses domain.ExpenseRepository
	cache    domain.Cache

	mu sync.Mutex
}

func New(budgets domain.BudgetRepository, expenses domain.ExpenseRepository, cache domain.Cache) Service {
	return &ledgerService{budgets: budgets, expenses: expenses, cache: cache}
}

func cents(v float64) int64 {
	return int64(math.Round(v * 100))
}

func (s *ledgerService) SetBudget(ctx context.Context, b domain.Budget) error {
	if err := b.Validate(); err != nil {
		return domain.NewValidationError(err)
	}
	if err := s.budgets.Upsert(ctx, b); err != nil {
		return err
	}
	s.cacheDelete(ctx, BudgetsCacheKey)
	return nil
}

func (s *ledgerService) ListBudgets(ctx context.Context) ([]domain.Budget, error) {
	var list []domain.Budget
	if s.cacheGet(ctx, BudgetsCacheKey, &list) {
		return list, nil
	}

	list, err := s.budgets.List(ctx)
	if err != nil {
		return nil, err
	}
	s.cacheSet(ctx, BudgetsCacheKey, list, BudgetsCacheTTL)
	return list, nil
}

func (s *ledgerService) LoadBudgets(ctx context.Context, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("не удалось прочитать бюджеты: %w", err)
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))

	var list []domain.Budget
	if err := json.Unmarshal(data, &list); err != nil {
		return domain.NewValidationError(fmt.Errorf("не удалось разобрать JSON с бюджетами: %w", err))
	}

	for i, b := range list {
		if err := b.Validate(); err != nil {
			return domain.NewValidationError(fmt.Errorf("бюджет #%d в файле: %w", i+1, err))
		}
	}

	for _, b := range list {
		if err := s.SetBudget(ctx, b); err != nil {
			return err
		}
	}
	return nil
}

func (s *ledgerService) AddTransaction(ctx context.Context, tx domain.Transaction) (domain.Transaction, error) {
	if err := tx.Validate(); err != nil {
		return domain.Transaction{}, domain.NewValidationError(err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	b, err := s.budgets.GetByCategory(ctx, tx.Category)
	switch {
	case errors.Is(err, domain.ErrNotFound):
	case err != nil:
		return domain.Transaction{}, err
	default:
		spent, err := s.expenses.SumByCategory(ctx, tx.Category)
		if err != nil {
			return domain.Transaction{}, err
		}
		if cents(spent)+cents(tx.Amount) > cents(b.Limit) {
			return domain.Transaction{}, domain.ErrBudgetExceeded
		}
	}

	return s.expenses.Add(ctx, tx)
}

func (s *ledgerService) ListTransactions(ctx context.Context) ([]domain.Transaction, error) {
	return s.expenses.List(ctx)
}

func (s *ledgerService) GetReportSummary(ctx context.Context, from, to string) (domain.ReportSummary, error) {
	if err := domain.ValidatePeriod(from, to); err != nil {
		return domain.ReportSummary{}, domain.NewValidationError(err)
	}

	key := ReportCacheKey(from, to)
	var report domain.ReportSummary
	if s.cacheGet(ctx, key, &report) {
		return report, nil
	}

	categories, err := s.expenses.SummaryByCategory(ctx, from, to)
	if err != nil {
		return domain.ReportSummary{}, err
	}

	report = domain.ReportSummary{From: from, To: to, Categories: categories}
	var total int64
	for i, c := range categories {
		categories[i].Total = float64(cents(c.Total)) / 100
		total += cents(c.Total)
	}
	report.Total = float64(total) / 100

	s.cacheSet(ctx, key, report, ReportCacheTTL)
	return report, nil
}

func (s *ledgerService) cacheGet(ctx context.Context, key string, v any) bool {
	if s.cache == nil {
		return false
	}
	data, ok, err := s.cache.Get(ctx, key)
	if err != nil {
		log.Printf("ошибка чтения кеша %s: %v", key, err)
		return false
	}
	if !ok {
		log.Printf("cache miss: %s", key)
		return false
	}
	if err := json.Unmarshal(data, v); err != nil {
		log.Printf("битое значение в кеше %s: %v", key, err)
		return false
	}
	log.Printf("cache hit: %s", key)
	return true
}

func (s *ledgerService) cacheSet(ctx context.Context, key string, v any, ttl time.Duration) {
	if s.cache == nil {
		return
	}
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("не удалось сериализовать %s: %v", key, err)
		return
	}
	if err := s.cache.Set(ctx, key, data, ttl); err != nil {
		log.Printf("ошибка записи кеша %s: %v", key, err)
		return
	}
	log.Printf("cache set: %s (ttl %s)", key, ttl)
}

func (s *ledgerService) cacheDelete(ctx context.Context, keys ...string) {
	if s.cache == nil {
		return
	}
	if err := s.cache.Delete(ctx, keys...); err != nil {
		log.Printf("ошибка удаления из кеша %v: %v", keys, err)
	}
}
