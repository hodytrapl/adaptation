package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

type ctxKeyRequestID struct{}


func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if reqID == "" {
			reqID = newRequestID()
		}

		w.Header().Set("X-Request-ID", reqID)
		ctx := context.WithValue(r.Context(), ctxKeyRequestID{}, reqID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}


func GetRequestID(ctx context.Context) string {
	v := ctx.Value(ctxKeyRequestID{})
	s, _ := v.(string)
	return s
}


func newRequestID() string {
	var b [16]byte
	_, err := rand.Read(b[:])
	if err != nil {

		return "0000000000000000"
	}
	return hex.EncodeToString(b[:])
}
