package middleware

import "net/http"

type errorWriter interface {
	Write(w http.ResponseWriter, r *http.Request, err error)
}
