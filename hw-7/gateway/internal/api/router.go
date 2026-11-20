package api

import "net/http"

func NewRouter() http.Handler {
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/transactions", Transactions)
	apiMux.HandleFunc("/budgets", Budgets)
	apiMux.HandleFunc("/reports/summary", ReportSummary)
	apiMux.HandleFunc("/", NotFound)

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", Ping)
	mux.Handle("/api/", http.StripPrefix("/api", apiMux))
	mux.HandleFunc("/", NotFound)

	return Logging(JSONContentType(mux))
}
