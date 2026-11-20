package api

import "ledger"

type CreateTransactionRequest struct {
	Amount      float64 `json:"amount"`
	Category    string  `json:"category"`
	Description string  `json:"description"`
	Date        string  `json:"date"`
}

type TransactionResponse struct {
	ID          int     `json:"id"`
	Amount      float64 `json:"amount"`
	Category    string  `json:"category"`
	Description string  `json:"description"`
	Date        string  `json:"date"`
}

type CreateBudgetRequest struct {
	Category string  `json:"category"`
	Limit    float64 `json:"limit"`
}

type BudgetResponse struct {
	Category string  `json:"category"`
	Limit    float64 `json:"limit"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func (r CreateTransactionRequest) ToDomain() ledger.Transaction {
	return ledger.Transaction{
		Amount:      r.Amount,
		Category:    r.Category,
		Description: r.Description,
		Date:        r.Date,
	}
}

func NewTransactionResponse(tx ledger.Transaction) TransactionResponse {
	return TransactionResponse{
		ID:          tx.ID,
		Amount:      tx.Amount,
		Category:    tx.Category,
		Description: tx.Description,
		Date:        tx.Date,
	}
}

func (r CreateBudgetRequest) ToDomain() ledger.Budget {
	return ledger.Budget{
		Category: r.Category,
		Limit:    r.Limit,
	}
}

func NewBudgetResponse(b ledger.Budget) BudgetResponse {
	return BudgetResponse{
		Category: b.Category,
		Limit:    b.Limit,
	}
}

type ReportSummaryResponse struct {
	From       string             `json:"from"`
	To         string             `json:"to"`
	Total      float64            `json:"total"`
	Categories map[string]float64 `json:"categories"`
}

func NewReportSummaryResponse(r ledger.ReportSummary) ReportSummaryResponse {
	resp := ReportSummaryResponse{
		From:       r.From,
		To:         r.To,
		Total:      r.Total,
		Categories: make(map[string]float64, len(r.Categories)),
	}
	for c, sum := range r.Categories {
		resp.Categories[c] = sum
	}
	return resp
}
