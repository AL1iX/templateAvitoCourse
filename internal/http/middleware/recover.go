package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
)

func Recover(errw errorWriter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(rec)
				}
				errw.Write(w, r, fmt.Errorf("panic: %v\n%s", rec, debug.Stack()))
			}()

			next.ServeHTTP(w, r)
		})
	}
}
