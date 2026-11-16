package middleware

import "net/http"

// statusRecorder — обёртка над http.ResponseWriter, которая запоминает статус и количество байт в ответе.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

// newStatusRecorder — конструктор statusRecorder.
func newStatusRecorder(w http.ResponseWriter) *statusRecorder {
	return &statusRecorder{ResponseWriter: w, status: http.StatusOK}
}

// WriteHeader — перехватывает вызов WriteHeader, чтобы запомнить статус-код.
func (r *statusRecorder) WriteHeader(statusCode int) {
	if r.wroteHeader {
		return
	}
	r.status = statusCode
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(statusCode)
}

// Write — перехватывает вызов Write, чтобы запомнить количество байт в ответе.
func (r *statusRecorder) Write(p []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.ResponseWriter.Write(p)
	r.bytes += n
	return n, err
}

// Unwrap — возвращает оригинальный http.ResponseWriter, чтобы можно было использовать его в других middleware.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
