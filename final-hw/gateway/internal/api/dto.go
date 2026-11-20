package api

import (
	"time"

	"proto/ledgerv1"
)

type ErrorResponse struct {
	Error string `json:"error"`
}

type CreateTransactionRequest struct {
	Amount      float64 `json:"amount"`
	Category    string  `json:"category"`
	Description string  `json:"description"`
	Date        string  `json:"date"`
}

type TransactionResponse struct {
	ID          int64   `json:"id"`
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

type CategoryReportResponse struct {
	Category      string   `json:"category"`
	Total         float64  `json:"total"`
	Budget        *float64 `json:"budget,omitempty"`
	BudgetPercent *float64 `json:"budget_percent,omitempty"`
}

type ReportSummaryResponse struct {
	From       string                   `json:"from"`
	To         string                   `json:"to"`
	Total      float64                  `json:"total"`
	Categories []CategoryReportResponse `json:"categories"`
}

type ImportErrorResponse struct {
	Index int    `json:"index"`
	Line  int    `json:"line,omitempty"`
	Error string `json:"error"`
}

type ImportSummaryResponse struct {
	Total    int                   `json:"total"`
	Accepted int                   `json:"accepted"`
	Rejected int                   `json:"rejected"`
	Errors   []ImportErrorResponse `json:"errors"`
}

type CredentialsRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type RegisterResponse struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

type LoginResponse struct {
	Token     string `json:"token"`
	TokenType string `json:"token_type"`
	ExpiresAt string `json:"expires_at"`
}

func (r CreateTransactionRequest) ToPB() *ledgerv1.Transaction {
	return &ledgerv1.Transaction{
		Amount:      r.Amount,
		Category:    r.Category,
		Description: r.Description,
		Date:        r.Date,
	}
}

func NewTransactionResponse(tx *ledgerv1.Transaction) TransactionResponse {
	return TransactionResponse{
		ID:          tx.GetId(),
		Amount:      tx.GetAmount(),
		Category:    tx.GetCategory(),
		Description: tx.GetDescription(),
		Date:        tx.GetDate(),
	}
}

func NewBudgetResponse(b *ledgerv1.Budget) BudgetResponse {
	return BudgetResponse{Category: b.GetCategory(), Limit: b.GetLimit()}
}

func NewReportSummaryResponse(r *ledgerv1.GetReportSummaryResponse) ReportSummaryResponse {
	resp := ReportSummaryResponse{
		From:       r.GetFrom(),
		To:         r.GetTo(),
		Total:      r.GetTotal(),
		Categories: make([]CategoryReportResponse, 0, len(r.GetCategories())),
	}
	for _, c := range r.GetCategories() {
		resp.Categories = append(resp.Categories, CategoryReportResponse{
			Category:      c.GetCategory(),
			Total:         c.GetTotal(),
			Budget:        c.Budget,
			BudgetPercent: c.BudgetPercent,
		})
	}
	return resp
}

func NewImportSummaryResponse(s *ledgerv1.ImportSummary) ImportSummaryResponse {
	resp := ImportSummaryResponse{
		Total:    int(s.GetTotal()),
		Accepted: int(s.GetAccepted()),
		Rejected: int(s.GetRejected()),
		Errors:   make([]ImportErrorResponse, 0, len(s.GetErrors())),
	}
	for _, e := range s.GetErrors() {
		resp.Errors = append(resp.Errors, ImportErrorResponse{Index: int(e.GetIndex()), Line: int(e.GetLine()), Error: e.GetError()})
	}
	return resp
}

func NewLoginResponse(token string, expiresAt int64) LoginResponse {
	return LoginResponse{
		Token:     token,
		TokenType: "Bearer",
		ExpiresAt: time.Unix(expiresAt, 0).UTC().Format(time.RFC3339),
	}
}
