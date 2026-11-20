package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	DateLayout   = "2006-01-02"
	MaxJSONBody  = 1 << 20
	MaxBulkBody  = 8 << 20
	MaxCSVBody   = 8 << 20
	MaxBulkItems = 10000
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

func decodeJSON(w http.ResponseWriter, r *http.Request, limit int64, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("тело запроса больше %d байт", tooBig.Limit))
			return false
		}
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return false
	}
	return true
}

func HTTPStatus(code codes.Code) int {
	switch code {
	case codes.OK:
		return http.StatusOK
	case codes.InvalidArgument, codes.OutOfRange:
		return http.StatusBadRequest
	case codes.FailedPrecondition, codes.AlreadyExists, codes.Aborted:
		return http.StatusConflict
	case codes.NotFound:
		return http.StatusNotFound
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.ResourceExhausted:
		return http.StatusRequestEntityTooLarge
	case codes.DeadlineExceeded, codes.Canceled:
		return http.StatusGatewayTimeout
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func writeGRPCError(w http.ResponseWriter, r *http.Request, op string, err error) {
	if r.Context().Err() != nil {
		log.Printf("%s: запрос отменён: %v", op, err)
		writeError(w, http.StatusGatewayTimeout, "request timeout")
		return
	}

	st := status.Convert(err)
	code := HTTPStatus(st.Code())
	switch code {
	case http.StatusGatewayTimeout:
		writeError(w, code, "request timeout")
	case http.StatusServiceUnavailable:
		log.Printf("%s: %v", op, err)
		writeError(w, code, "service unavailable")
	case http.StatusInternalServerError:
		log.Printf("%s: %v", op, err)
		writeError(w, code, "internal server error")
	default:
		writeError(w, code, st.Message())
	}
}

func validatePeriod(from, to string) error {
	f, err := time.Parse(DateLayout, from)
	if err != nil {
		return fmt.Errorf("неверный период отчёта: from=%q, нужен формат ГГГГ-ММ-ДД", from)
	}
	t, err := time.Parse(DateLayout, to)
	if err != nil {
		return fmt.Errorf("неверный период отчёта: to=%q, нужен формат ГГГГ-ММ-ДД", to)
	}
	if f.After(t) {
		return fmt.Errorf("неверный период отчёта: from позже to")
	}
	return nil
}
