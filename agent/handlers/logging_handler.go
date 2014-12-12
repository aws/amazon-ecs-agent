package handlers

import "net/http"

type LoggingHandler struct{ h http.Handler }

func (lh LoggingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Info("Handling http request", "method", r.Method, "from", r.RemoteAddr, "uri", r.RequestURI)
	lh.h.ServeHTTP(w, r)
}
