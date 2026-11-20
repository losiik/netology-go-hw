package api

import (
	"net/http"
	"strings"

	"proto/authv1"
)

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req CredentialsRequest
	if !decodeJSON(w, r, MaxJSONBody, &req) {
		return
	}
	if strings.TrimSpace(req.Login) == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "логин и пароль обязательны")
		return
	}

	resp, err := h.auth.Register(r.Context(), &authv1.RegisterRequest{Login: req.Login, Password: req.Password})
	if err != nil {
		writeGRPCError(w, r, "ошибка регистрации", err)
		return
	}
	writeJSON(w, http.StatusCreated, RegisterResponse{ID: resp.GetId(), Login: resp.GetLogin()})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req CredentialsRequest
	if !decodeJSON(w, r, MaxJSONBody, &req) {
		return
	}
	if strings.TrimSpace(req.Login) == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "логин и пароль обязательны")
		return
	}

	resp, err := h.auth.Login(r.Context(), &authv1.LoginRequest{Login: req.Login, Password: req.Password})
	if err != nil {
		writeGRPCError(w, r, "ошибка входа", err)
		return
	}
	writeJSON(w, http.StatusOK, NewLoginResponse(resp.GetToken(), resp.GetExpiresAt()))
}
