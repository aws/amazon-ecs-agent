package introspection

import (
	"errors"
	"net/http"

	"github.com/aws/amazon-ecs-agent/ecs-agent/metrics"
)

// panicHandler handler will gracefully close the connection if a panic occurs, returning
// an internal server error to the client.
func panicHandler(next http.Handler, metricsFactory metrics.EntryFactory) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if r := recover(); r != nil {
				var err error
				switch x := r.(type) {
				case string:
					err = errors.New(x)
				case error:
					err = x
				default:
					err = errors.New("unknown panic")
				}
				w.Header().Set("Connection", "close")
				w.WriteHeader(http.StatusInternalServerError)
				metricsFactory.New(metrics.IntrospectionCrash).Done(err)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
