package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const DateLayout = "2006-01-02"

type Validatable interface {
	Validate() error
}

func CheckValid(v Validatable) error {
	return v.Validate()
}

type Transaction struct {
	ID          int
	UserID      int64
	Amount      float64
	Category    string
	Description string
	Date        string
}

func (t Transaction) Validate() error {
	if t.Amount <= 0 {
		return fmt.Errorf("сумма должна быть больше 0, получено %.2f", t.Amount)
	}
	if strings.TrimSpace(t.Category) == "" {
		return errors.New("категория транзакции не может быть пустой")
	}
	if _, err := time.Parse(DateLayout, t.Date); err != nil {
		return fmt.Errorf("неверный формат даты %q, нужен ГГГГ-ММ-ДД", t.Date)
	}
	return nil
}

type Budget struct {
	UserID   int64   `json:"-"`
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

type CategoryReport struct {
	Category      string   `json:"category"`
	Total         float64  `json:"total"`
	Budget        *float64 `json:"budget,omitempty"`
	BudgetPercent *float64 `json:"budget_percent,omitempty"`
}

type ReportSummary struct {
	From       string           `json:"from"`
	To         string           `json:"to"`
	Total      float64          `json:"total"`
	Categories []CategoryReport `json:"categories"`
}

func ValidatePeriod(from, to string) error {
	f, err := time.Parse(DateLayout, from)
	if err != nil {
		return fmt.Errorf("%w: from=%q, нужен формат ГГГГ-ММ-ДД", ErrInvalidPeriod, from)
	}
	t, err := time.Parse(DateLayout, to)
	if err != nil {
		return fmt.Errorf("%w: to=%q, нужен формат ГГГГ-ММ-ДД", ErrInvalidPeriod, to)
	}
	if f.After(t) {
		return fmt.Errorf("%w: from позже to", ErrInvalidPeriod)
	}
	return nil
}

type ImportError struct {
	Index int    `json:"index"`
	Line  int    `json:"line,omitempty"`
	Error string `json:"error"`
}

type ImportSummary struct {
	Total    int           `json:"total"`
	Accepted int           `json:"accepted"`
	Rejected int           `json:"rejected"`
	Errors   []ImportError `json:"errors"`
}
