package metrics

import (
	"context"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Serve starts a standalone HTTP server at addr serving only /metrics.
// It returns immediately; the server shuts down when ctx is cancelled.
// No-op when addr is empty.
func Serve(ctx context.Context, addr string) {
	if addr == "" {
		return
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background()) //nolint:errcheck
	}()
	go srv.ListenAndServe() //nolint:errcheck
}
