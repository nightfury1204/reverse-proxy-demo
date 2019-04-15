package filter

import (
	"fmt"
	"net/http"
)

type ResponseWriterWrapper struct {
	rw http.ResponseWriter
}

func NewResponseWriterWrapper(w http.ResponseWriter) *ResponseWriterWrapper {
	return &ResponseWriterWrapper{
		w,
	}
}

func (w *ResponseWriterWrapper) Header() http.Header {
	return w.rw.Header()
}

func (w *ResponseWriterWrapper) Write(data []byte) (int, error) {
	s := string(data)
	fmt.Println(s)
	return w.rw.Write(data)
}

func (w *ResponseWriterWrapper) WriteHeader(statusCode int) {
	w.rw.WriteHeader(statusCode)
}