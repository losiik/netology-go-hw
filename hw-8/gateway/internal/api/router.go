package api

import (
	"net/http"

	"ledger"
)

func NewRouter(svc ledger.Service) http.Handler {
	h := NewHandler(svc)

	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/transactions", h.Transactions)
	apiMux.HandleFunc("/budgets", h.Budgets)
	apiMux.HandleFunc("/reports/summary", h.ReportSummary)
	apiMux.HandleFunc("/", NotFound)

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", Ping)
	mux.Handle("/api/", http.StripPrefix("/api", apiMux))
	mux.HandleFunc("/", NotFound)

	return Logging(JSONContentType(mux))
}
