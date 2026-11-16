// Package middleware содержит HTTP‑middleware: функции-обёртки над http.Handler,
// которые добавляют общий функционал (логирование, авторизация, заголовки)
// вокруг основного обработчика без изменения его кода.
package middleware

import (
	"log"
	"net/http"
	"time"
)

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := newStatusRecorder(w)
		next.ServeHTTP(rec, r)
		reqID := GetRequestID(r.Context())

		log.Printf("request_id=%s method=%s path=%s status=%d bytes=%d server_in=%v", reqID, r.Method, r.URL.Path, rec.status, rec.bytes, time.Since(start))
	})
}

func BasicAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, pass, ok := r.BasicAuth()
		if !ok || name != "admin" || pass != "secret" {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Unauthorized", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func JSONHeaderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		next.ServeHTTP(w, r)
	})
}
