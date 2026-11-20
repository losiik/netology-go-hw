package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"ledger"
)

const (
	DefaultBulkWorkers = ledger.DefaultImportWorkers
	MaxBulkItems       = 10000
)

func parseWorkers(r *http.Request) (int, error) {
	v := r.URL.Query().Get("workers")
	if v == "" {
		return DefaultBulkWorkers, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > ledger.MaxImportWorkers {
		return 0, fmt.Errorf("workers должен быть числом от 1 до %d, получено %q", ledger.MaxImportWorkers, v)
	}
	return n, nil
}

func (h *Handler) BulkTransactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	workers, err := parseWorkers(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req []CreateTransactionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req == nil {
		writeError(w, http.StatusBadRequest, "ожидался JSON-массив транзакций")
		return
	}
	if len(req) > MaxBulkItems {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("слишком много транзакций: %d, максимум %d", len(req), MaxBulkItems))
		return
	}

	txs := make([]ledger.Transaction, 0, len(req))
	for _, item := range req {
		txs = append(txs, item.ToDomain())
	}

	summary, err := h.svc.ImportTransactions(r.Context(), txs, workers)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || r.Context().Err() != nil {
			log.Printf("импорт транзакций: %v", err)
			writeJSON(w, http.StatusGatewayTimeout, ImportTimeoutResponse{
				Error:                 "request timeout",
				ImportSummaryResponse: NewImportSummaryResponse(summary),
			})
			return
		}
		writeServiceError(w, r, "ошибка импорта транзакций", err)
		return
	}

	writeJSON(w, http.StatusOK, NewImportSummaryResponse(summary))
}
