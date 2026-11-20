package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"ledger"
)

type Handler struct {
	svc ledger.Service
}

func NewHandler(svc ledger.Service) *Handler {
	return &Handler{svc: svc}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Println("ошибка записи ответа:", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}

func writeServiceError(w http.ResponseWriter, r *http.Request, op string, err error) {
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled), r.Context().Err() != nil:
		log.Printf("%s: запрос отменён: %v", op, err)
		writeError(w, http.StatusGatewayTimeout, "request timeout")
	case errors.Is(err, ledger.ErrValidation):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ledger.ErrBudgetExceeded):
		writeError(w, http.StatusConflict, err.Error())
	default:
		log.Printf("%s: %v", op, err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("неверный JSON: %w", err)
	}
	return nil
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

func (h *Handler) Transactions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listTransactions(w, r)
	case http.MethodPost:
		h.createTransaction(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) Budgets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listBudgets(w, r)
	case http.MethodPost:
		h.createBudget(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) createTransaction(w http.ResponseWriter, r *http.Request) {
	var req CreateTransactionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	created, err := h.svc.AddTransaction(r.Context(), req.ToDomain())
	if err != nil {
		writeServiceError(w, r, "ошибка добавления транзакции", err)
		return
	}

	writeJSON(w, http.StatusCreated, NewTransactionResponse(created))
}

func (h *Handler) listTransactions(w http.ResponseWriter, r *http.Request) {
	txs, err := h.svc.ListTransactions(r.Context())
	if err != nil {
		writeServiceError(w, r, "ошибка чтения транзакций", err)
		return
	}
	resp := make([]TransactionResponse, 0, len(txs))
	for _, tx := range txs {
		resp = append(resp, NewTransactionResponse(tx))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) createBudget(w http.ResponseWriter, r *http.Request) {
	var req CreateBudgetRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	b := req.ToDomain()
	if err := h.svc.SetBudget(r.Context(), b); err != nil {
		writeServiceError(w, r, "ошибка установки бюджета", err)
		return
	}

	writeJSON(w, http.StatusCreated, NewBudgetResponse(b))
}

func (h *Handler) listBudgets(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListBudgets(r.Context())
	if err != nil {
		writeServiceError(w, r, "ошибка чтения бюджетов", err)
		return
	}
	resp := make([]BudgetResponse, 0, len(list))
	for _, b := range list {
		resp = append(resp, NewBudgetResponse(b))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) ReportSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if err := ledger.ValidatePeriod(from, to); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	report, err := h.svc.GetReportSummary(r.Context(), from, to)
	if err != nil {
		writeServiceError(w, r, "ошибка построения отчёта", err)
		return
	}

	writeJSON(w, http.StatusOK, NewReportSummaryResponse(report))
}
