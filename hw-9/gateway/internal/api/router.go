package api

import (
	"net/http"
	"time"

	"ledger"
)

func NewRouter(svc ledger.Service, timeout time.Duration) http.Handler {
	h := NewHandler(svc)

	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/transactions", h.Transactions)
	apiMux.HandleFunc("/budgets", h.Budgets)
	apiMux.HandleFunc("/reports/summary", h.ReportSummary)
	apiMux.HandleFunc("/", NotFound)

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", Ping)
	mux.Handle("/api/", http.StripPrefix("/api", Timeout(timeout)(apiMux)))
	mux.HandleFunc("/", NotFound)

	return Logging(JSONContentType(mux))
}
