package save

import (
	"encoding/json"
	"net/http"

	"github.com/AL1iX/templateAvitoCourse/internal/generated"
)

type Handler struct {
	uc   useCase
	errw errorWriter
}

func New(uc useCase, errw errorWriter) *Handler {
	return &Handler{uc: uc, errw: errw}
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, id generated.TripId) {
	point, recordedAt, err := decode(r.Body)
	if err != nil {
		h.errw.Write(w, r, err)
		return
	}

	p, err := h.uc.Execute(r.Context(), id.String(), point, recordedAt)
	if err != nil {
		h.errw.Write(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toAPI(p))
}
