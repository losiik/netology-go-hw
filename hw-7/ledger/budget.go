package ledger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	budgetsCacheKey = "budgets:all"
	budgetsCacheTTL = 30 * time.Second
)

type Budget struct {
	Category string  `json:"category"`
	Limit    float64 `json:"limit"`
}

func (b Budget) Validate() error {
	if strings.TrimSpace(b.Category) == "" {
		return errors.New("категория бюджета не может быть пустой")
	}
	if b.Limit <= 0 {
		return fmt.Errorf("лимит должен быть больше 0, получено %.2f", b.Limit)
	}
	return nil
}

func SetBudget(b Budget) error {
	if err := b.Validate(); err != nil {
		return fmt.Errorf("невалидный бюджет: %w", err)
	}
	if err := ready(); err != nil {
		return err
	}

	ctx := context.Background()
	_, err := pg.ExecContext(ctx,
		`INSERT INTO budgets (category, limit_amount) VALUES ($1, $2)
		 ON CONFLICT (category) DO UPDATE SET limit_amount = EXCLUDED.limit_amount`,
		b.Category, b.Limit)
	if err != nil {
		return fmt.Errorf("сохранение бюджета: %w", err)
	}

	cacheDel(ctx, budgetsCacheKey)
	return nil
}

func ListBudgets() ([]Budget, error) {
	if err := ready(); err != nil {
		return nil, err
	}

	ctx := context.Background()
	var result []Budget
	if cacheGet(ctx, budgetsCacheKey, &result) {
		return result, nil
	}

	rows, err := pg.QueryContext(ctx, "SELECT category, limit_amount FROM budgets ORDER BY category")
	if err != nil {
		return nil, fmt.Errorf("чтение бюджетов: %w", err)
	}
	defer rows.Close()

	result = []Budget{}
	for rows.Next() {
		var b Budget
		if err := rows.Scan(&b.Category, &b.Limit); err != nil {
			return nil, fmt.Errorf("чтение бюджета: %w", err)
		}
		result = append(result, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение бюджетов: %w", err)
	}

	cacheSet(ctx, budgetsCacheKey, result, budgetsCacheTTL)
	return result, nil
}

func LoadBudgets(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("не удалось прочитать бюджеты: %w", err)
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))

	var list []Budget
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("не удалось разобрать JSON с бюджетами: %w", err)
	}

	for i, b := range list {
		if err := b.Validate(); err != nil {
			return fmt.Errorf("бюджет #%d в файле: %w", i+1, err)
		}
	}

	for _, b := range list {
		if err := SetBudget(b); err != nil {
			return err
		}
	}
	return nil
}
