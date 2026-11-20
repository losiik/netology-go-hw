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
	BudgetsCacheTTL = 30 * time.Second
	ReportCacheTTL  = 30 * time.Second
)

func BudgetsCacheKey(userID int64) string {
	return fmt.Sprintf("budgets:%d", userID)
}

func ReportCachePrefix(userID int64) string {
	return fmt.Sprintf("report:summary:%d:", userID)
}

func ReportCacheKey(userID int64, from, to string) string {
	return ReportCachePrefix(userID) + from + ":" + to
}

type Service interface {
	SetBudget(ctx context.Context, b domain.Budget) error
	ListBudgets(ctx context.Context) ([]domain.Budget, error)
	LoadBudgets(ctx context.Context, r io.Reader) error
	AddTransaction(ctx context.Context, tx domain.Transaction) (domain.Transaction, error)
	ListTransactions(ctx context.Context) ([]domain.Transaction, error)
	GetReportSummary(ctx context.Context, from, to string) (domain.ReportSummary, error)
	ImportTransactions(ctx context.Context, txs []domain.Transaction, workers int) (domain.ImportSummary, error)
	ImportCSV(ctx context.Context, r io.Reader, workers int) (domain.ImportSummary, error)
	ExportCSV(ctx context.Context, w io.Writer, from, to string) error
}

type ledgerService struct {
	budgets  domain.BudgetRepository
	expenses domain.ExpenseRepository
	cache    domain.Cache

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func New(budgets domain.BudgetRepository, expenses domain.ExpenseRepository, cache domain.Cache) Service {
	return &ledgerService{budgets: budgets, expenses: expenses, cache: cache, locks: map[string]*sync.Mutex{}}
}

func (s *ledgerService) lockCategory(userID int64, category string) func() {
	key := fmt.Sprintf("%d:%s", userID, category)
	s.mu.Lock()
	l, ok := s.locks[key]
	if !ok {
		l = &sync.Mutex{}
		s.locks[key] = l
	}
	s.mu.Unlock()

	l.Lock()
	return l.Unlock
}

func cents(v float64) int64 {
	return int64(math.Round(v * 100))
}

func (s *ledgerService) SetBudget(ctx context.Context, b domain.Budget) error {
	if err := b.Validate(); err != nil {
		return domain.NewValidationError(err)
	}
	b.UserID = domain.UserIDFromContext(ctx)
	if err := s.budgets.Upsert(ctx, b); err != nil {
		return err
	}
	s.cacheDelete(ctx, BudgetsCacheKey(b.UserID))
	s.invalidateReports(ctx)
	return nil
}

func (s *ledgerService) ListBudgets(ctx context.Context) ([]domain.Budget, error) {
	userID := domain.UserIDFromContext(ctx)
	key := BudgetsCacheKey(userID)
	var list []domain.Budget
	if s.cacheGet(ctx, key, &list) {
		return list, nil
	}

	list, err := s.budgets.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	s.cacheSet(ctx, key, list, BudgetsCacheTTL)
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
	created, err := s.addTransaction(ctx, tx)
	if err != nil {
		return domain.Transaction{}, err
	}
	s.invalidateReports(ctx)
	return created, nil
}

func (s *ledgerService) addTransaction(ctx context.Context, tx domain.Transaction) (domain.Transaction, error) {
	if err := tx.Validate(); err != nil {
		return domain.Transaction{}, domain.NewValidationError(err)
	}

	tx.UserID = domain.UserIDFromContext(ctx)
	unlock := s.lockCategory(tx.UserID, tx.Category)
	defer unlock()

	b, err := s.budgets.GetByCategory(ctx, tx.UserID, tx.Category)
	switch {
	case errors.Is(err, domain.ErrNotFound):
	case err != nil:
		return domain.Transaction{}, err
	default:
		spent, err := s.expenses.SumByCategory(ctx, tx.UserID, tx.Category)
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
	return s.expenses.List(ctx, domain.UserIDFromContext(ctx))
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

func (s *ledgerService) invalidateReports(ctx context.Context) {
	if s.cache == nil {
		return
	}
	prefix := ReportCachePrefix(domain.UserIDFromContext(ctx))
	if err := s.cache.DeleteByPrefix(ctx, prefix); err != nil {
		log.Printf("ошибка сброса кеша отчётов: %v", err)
		return
	}
	log.Printf("cache invalidate: %s*", prefix)
}
