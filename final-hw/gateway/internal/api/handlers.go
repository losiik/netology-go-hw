package api

import (
	"net/http"

	"proto/authv1"
	"proto/ledgerv1"
)

type Handler struct {
	ledger ledgerv1.LedgerServiceClient
	auth   authv1.AuthServiceClient
}

func NewHandler(ledger ledgerv1.LedgerServiceClient, auth authv1.AuthServiceClient) *Handler {
	return &Handler{ledger: ledger, auth: auth}
}

func Ping(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, "pong")
}

func NotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "not found")
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (h *Handler) Transactions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listTransactions(w, r)
	case http.MethodPost:
		h.createTransaction(w, r)
	default:
		methodNotAllowed(w)
	}
}

func (h *Handler) Budgets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listBudgets(w, r)
	case http.MethodPost:
		h.createBudget(w, r)
	default:
		methodNotAllowed(w)
	}
}

func (h *Handler) createTransaction(w http.ResponseWriter, r *http.Request) {
	var req CreateTransactionRequest
	if !decodeJSON(w, r, MaxJSONBody, &req) {
		return
	}

	resp, err := h.ledger.AddTransaction(r.Context(), &ledgerv1.AddTransactionRequest{Transaction: req.ToPB()})
	if err != nil {
		writeGRPCError(w, r, "ошибка добавления транзакции", err)
		return
	}
	writeJSON(w, http.StatusCreated, NewTransactionResponse(resp.GetTransaction()))
}

func (h *Handler) listTransactions(w http.ResponseWriter, r *http.Request) {
	resp, err := h.ledger.ListTransactions(r.Context(), &ledgerv1.ListTransactionsRequest{})
	if err != nil {
		writeGRPCError(w, r, "ошибка чтения транзакций", err)
		return
	}
	list := make([]TransactionResponse, 0, len(resp.GetTransactions()))
	for _, tx := range resp.GetTransactions() {
		list = append(list, NewTransactionResponse(tx))
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) createBudget(w http.ResponseWriter, r *http.Request) {
	var req CreateBudgetRequest
	if !decodeJSON(w, r, MaxJSONBody, &req) {
		return
	}

	resp, err := h.ledger.SetBudget(r.Context(), &ledgerv1.SetBudgetRequest{
		Budget: &ledgerv1.Budget{Category: req.Category, Limit: req.Limit},
	})
	if err != nil {
		writeGRPCError(w, r, "ошибка установки бюджета", err)
		return
	}
	writeJSON(w, http.StatusCreated, NewBudgetResponse(resp.GetBudget()))
}

func (h *Handler) listBudgets(w http.ResponseWriter, r *http.Request) {
	resp, err := h.ledger.ListBudgets(r.Context(), &ledgerv1.ListBudgetsRequest{})
	if err != nil {
		writeGRPCError(w, r, "ошибка чтения бюджетов", err)
		return
	}
	list := make([]BudgetResponse, 0, len(resp.GetBudgets()))
	for _, b := range resp.GetBudgets() {
		list = append(list, NewBudgetResponse(b))
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) ReportSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if err := validatePeriod(from, to); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := h.ledger.GetReportSummary(r.Context(), &ledgerv1.GetReportSummaryRequest{From: from, To: to})
	if err != nil {
		writeGRPCError(w, r, "ошибка построения отчёта", err)
		return
	}
	writeJSON(w, http.StatusOK, NewReportSummaryResponse(resp))
}
