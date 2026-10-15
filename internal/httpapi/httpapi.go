// Package httpapi exposes the three routes and maps service errors to HTTP
// status codes. Error texts are defined in design "Error Messages".
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/laodao-ai/demo-temp-monitor/internal/service"
	"github.com/laodao-ai/demo-temp-monitor/internal/store"
)

const (
	maxBodyBytes = 1 << 20
	// storeTimeout bounds every store call made for one request.
	storeTimeout = 5 * time.Second

	msgInvalidJSON  = "invalid JSON body"
	msgBodyTooLarge = "request body too large"
	msgIDRequired   = "id is required"
	msgInvalidID    = "invalid device id: must match " + service.IDPattern
	msgTempRequired = "temperature_c is required"
	msgTempNotNum   = "temperature_c must be a number"
	msgNotFound     = "device not found"
	msgExists       = "device already registered"
	msgInternal     = "internal error"
)

type handler struct {
	svc    *service.Service
	logger *slog.Logger
}

// NewHandler returns the router with the three routes registered.
// Other paths and wrong methods get ServeMux's plain-text 404 / 405.
func NewHandler(svc *service.Service, logger *slog.Logger) http.Handler {
	h := &handler{svc: svc, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /devices", h.register)
	mux.HandleFunc("DELETE /devices/{id}", h.deregister)
	mux.HandleFunc("POST /devices/{id}/readings", h.report)
	return mux
}

func (h *handler) register(w http.ResponseWriter, r *http.Request) {
	// A pointer tells a missing id apart from an empty one.
	var req struct {
		ID *string `json:"id"`
	}
	if msg := readJSON(w, r, &req, "id", msgInvalidID); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if req.ID == nil {
		writeError(w, http.StatusBadRequest, msgIDRequired)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), storeTimeout)
	defer cancel()
	d, err := h.svc.Register(ctx, *req.ID)
	if err != nil {
		h.writeServiceError(w, r, *req.ID, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":            d.ID,
		"registered_at": d.RegisteredAt.Format(time.RFC3339Nano),
	})
}

func (h *handler) deregister(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx, cancel := context.WithTimeout(r.Context(), storeTimeout)
	defer cancel()
	if err := h.svc.Deregister(ctx, id); err != nil {
		h.writeServiceError(w, r, id, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) report(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// A pointer tells a missing temperature_c apart from 0.
	var req struct {
		TemperatureC *float64 `json:"temperature_c"`
	}
	if msg := readJSON(w, r, &req, "temperature_c", msgTempNotNum); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if req.TemperatureC == nil {
		writeError(w, http.StatusBadRequest, msgTempRequired)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), storeTimeout)
	defer cancel()
	rd, err := h.svc.Report(ctx, id, *req.TemperatureC)
	if err != nil {
		h.writeServiceError(w, r, id, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":            rd.ID,
		"device_id":     rd.DeviceID,
		"temperature_c": rd.TemperatureC,
		"recorded_at":   rd.RecordedAt.Format(time.RFC3339Nano),
	})
}

// readJSON reads the whole body (at most 1 MiB), then unmarshals it into dst.
// json.Unmarshal rejects empty input and non-whitespace content after the
// first value. It returns the 400 message, or "" on success. A type error on
// field typeField gets typeMsg; any other decode failure is "invalid JSON body".
func readJSON(w http.ResponseWriter, r *http.Request, dst any, typeField, typeMsg string) string {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return msgBodyTooLarge
		}
		return msgInvalidJSON
	}
	if err := json.Unmarshal(body, dst); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field == typeField {
			return typeMsg
		}
		return msgInvalidJSON
	}
	return ""
}

func (h *handler) writeServiceError(w http.ResponseWriter, r *http.Request, deviceID string, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidID):
		writeError(w, http.StatusBadRequest, msgInvalidID)
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, msgNotFound)
	case errors.Is(err, store.ErrAlreadyExists):
		writeError(w, http.StatusConflict, msgExists)
	default:
		// The underlying error goes to the log only, never into the response.
		// Redaction is the injected logger's job: main wraps its handler with
		// internal/redact (Task 3). Pass the error value, not err.Error():
		// redact only withholds the text for error values, not for strings.
		h.logger.Error("request failed",
			"route", r.Pattern,
			"device_id", deviceID,
			"error", err)
		writeError(w, http.StatusInternalServerError, msgInternal)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
