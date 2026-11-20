package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type Budget struct {
	Category string  `json:"category"`
	Limit    float64 `json:"limit"`
	Period   string  `json:"period"`
}

var budgets map[string]Budget

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
	budgets[b.Category] = b
	return nil
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

func categoryTotal(category string) float64 {
	var total float64
	for _, tx := range transactions {
		if tx.Category == category {
			total += tx.Amount
		}
	}
	return total
}
