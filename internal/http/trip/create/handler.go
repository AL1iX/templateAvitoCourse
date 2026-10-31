package create

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/AL1iX/templateAvitoCourse/internal/domain/trip"
	"github.com/AL1iX/templateAvitoCourse/internal/generated"
)

type Handler struct {
	uc   useCase
	errw errorWriter
}

func New(uc useCase, errw errorWriter) *Handler {
	return &Handler{uc: uc, errw: errw}
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, params generated.CreateTripParams) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		h.errw.Write(w, r, &trip.ValidationError{Field: bodyField, Reason: err.Error()})
		return
	}

	in, err := toInput(raw, params)
	if err != nil {
		h.errw.Write(w, r, err)
		return
	}

	res, err := h.uc.Execute(r.Context(), in)
	if err != nil {
		h.errw.Write(w, r, err)
		return
	}

	status := http.StatusOK
	if !res.Replayed {
		status = http.StatusCreated
		w.Header().Set("Location", "/api/v1/trips/"+res.Trip.ID())
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(toAPI(res.Trip))
}
