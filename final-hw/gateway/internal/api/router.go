package api

import (
	"net/http"
	"time"

	"gateway/internal/auth"
	"proto/authv1"
	"proto/ledgerv1"
)

type Deps struct {
	Ledger   ledgerv1.LedgerServiceClient
	Auth     authv1.AuthServiceClient
	Verifier *auth.Verifier
	Timeout  time.Duration
}

func NewRouter(d Deps) http.Handler {
	h := NewHandler(d.Ledger, d.Auth)
	timeout := Timeout(d.Timeout)

	public := http.NewServeMux()
	public.HandleFunc("/auth/register", h.Register)
	public.HandleFunc("/auth/login", h.Login)
	public.HandleFunc("/", NotFound)

	protected := http.NewServeMux()
	protected.HandleFunc("/transactions", h.Transactions)
	protected.HandleFunc("/transactions/bulk", h.BulkTransactions)
	protected.HandleFunc("/transactions/import", h.ImportCSV)
	protected.HandleFunc("/transactions/export.csv", h.ExportCSV)
	protected.HandleFunc("/budgets", h.Budgets)
	protected.HandleFunc("/reports/summary", h.ReportSummary)
	protected.HandleFunc("/", NotFound)

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", Ping)
	mux.Handle("/api/auth/", http.StripPrefix("/api", timeout(public)))
	mux.Handle("/api/", http.StripPrefix("/api", timeout(RequireAuth(d.Verifier)(protected))))
	mux.HandleFunc("/", NotFound)

	return Logging(JSONContentType(mux))
}
