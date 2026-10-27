package finish

import (
	"encoding/json"
	"net/http"

	"github.com/AL1iX/templateAvitoCourse/api"
)

type Handler struct {
	uc   useCase
	errw errorWriter
}

func New(uc useCase, errw errorWriter) *Handler {
	return &Handler{uc: uc, errw: errw}
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, id api.TripId) {
	t, err := h.uc.Execute(r.Context(), id.String())
	if err != nil {
		h.errw.Write(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(toAPI(t))
}
