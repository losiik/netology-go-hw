package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"ledger"
)

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

func Transactions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		listTransactions(w, r)
	case http.MethodPost:
		createTransaction(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func Budgets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		listBudgets(w, r)
	case http.MethodPost:
		createBudget(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func NotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "not found")
}

func createTransaction(w http.ResponseWriter, r *http.Request) {
	var req CreateTransactionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	tx := req.ToDomain()
	if err := tx.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	created, err := ledger.AddTransaction(tx)
	if errors.Is(err, ledger.ErrBudgetExceeded) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		log.Println("ошибка добавления транзакции:", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusCreated, NewTransactionResponse(created))
}

func listTransactions(w http.ResponseWriter, r *http.Request) {
	txs := ledger.ListTransactions()
	resp := make([]TransactionResponse, 0, len(txs))
	for _, tx := range txs {
		resp = append(resp, NewTransactionResponse(tx))
	}
	writeJSON(w, http.StatusOK, resp)
}

func createBudget(w http.ResponseWriter, r *http.Request) {
	var req CreateBudgetRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	b := req.ToDomain()
	if err := b.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := ledger.SetBudget(b); err != nil {
		log.Println("ошибка установки бюджета:", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusCreated, NewBudgetResponse(b))
}

func listBudgets(w http.ResponseWriter, r *http.Request) {
	list := ledger.ListBudgets()
	resp := make([]BudgetResponse, 0, len(list))
	for _, b := range list {
		resp = append(resp, NewBudgetResponse(b))
	}
	writeJSON(w, http.StatusOK, resp)
}
