package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type Budget struct {
	Category string  `json:"category"`
	Limit    float64 `json:"limit"`
	Period   string  `json:"period"`
}

var budgets map[string]Budget

func SetBudget(b Budget) {
	budgets[b.Category] = b
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

	for _, b := range list {
		if b.Category == "" {
			return errors.New("у бюджета не указана категория")
		}
		if b.Limit <= 0 {
			return fmt.Errorf("неверный лимит для категории %q: %.2f", b.Category, b.Limit)
		}
	}

	for _, b := range list {
		SetBudget(b)
	}
	return nil
}

func categoryTotal(category string) float64 {
	var total float64
	for _, tx := range transactions {
		if tx.Category == category {
			total += tx.Amount
		}
	}
	return total
}
