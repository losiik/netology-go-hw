package api

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"proto/ledgerv1"
)

const (
	DefaultWorkers = 4
	MaxWorkers     = 32
)

func parseWorkers(r *http.Request) (int32, error) {
	v := r.URL.Query().Get("workers")
	if v == "" {
		return DefaultWorkers, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > MaxWorkers {
		return 0, fmt.Errorf("workers должен быть числом от 1 до %d, получено %q", MaxWorkers, v)
	}
	return int32(n), nil
}

func (h *Handler) BulkTransactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	workers, err := parseWorkers(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req []CreateTransactionRequest
	if !decodeJSON(w, r, MaxBulkBody, &req) {
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

	txs := make([]*ledgerv1.Transaction, 0, len(req))
	for _, item := range req {
		txs = append(txs, item.ToPB())
	}

	resp, err := h.ledger.ImportTransactions(r.Context(), &ledgerv1.ImportTransactionsRequest{Transactions: txs, Workers: workers})
	if err != nil {
		writeGRPCError(w, r, "ошибка импорта транзакций", err)
		return
	}
	writeJSON(w, http.StatusOK, NewImportSummaryResponse(resp.GetSummary()))
}

func readCSVBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxCSVBody)

	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaType == "multipart/form-data" {
		file, _, err := r.FormFile("file")
		if err != nil {
			return nil, fmt.Errorf("в форме нет файла в поле \"file\": %w", err)
		}
		defer file.Close()
		return io.ReadAll(file)
	}
	return io.ReadAll(r.Body)
}

func (h *Handler) ImportCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	workers, err := parseWorkers(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	data, err := readCSVBody(w, r)
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("CSV больше %d байт", tooBig.Limit))
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		writeError(w, http.StatusBadRequest, "пустой CSV")
		return
	}

	resp, err := h.ledger.ImportCSV(r.Context(), &ledgerv1.ImportCSVRequest{Data: data, Workers: workers})
	if err != nil {
		writeGRPCError(w, r, "ошибка импорта CSV", err)
		return
	}
	writeJSON(w, http.StatusOK, NewImportSummaryResponse(resp.GetSummary()))
}

func (h *Handler) ExportCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	filename := "transactions.csv"
	if from != "" || to != "" {
		if err := validatePeriod(from, to); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		filename = fmt.Sprintf("transactions_%s_%s.csv", from, to)
	}

	resp, err := h.ledger.ExportCSV(r.Context(), &ledgerv1.ExportCSVRequest{From: from, To: to})
	if err != nil {
		writeGRPCError(w, r, "ошибка экспорта CSV", err)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("\xef\xbb\xbf"))
	w.Write(resp.GetData())
}
