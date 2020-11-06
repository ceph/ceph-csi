package util

import (
	"net"
	"net/http"
	"net/url"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ValidateURL validates the url.
func ValidateURL(c *Config) error {
	_, err := url.Parse(c.MetricsPath)
	return err
}

// StartMetricsServer starts http server.
func StartMetricsServer(c *Config) {
	addr := net.JoinHostPort(c.MetricsIP, c.MetricsPort)
	http.Handle(c.MetricsPath, promhttp.Handler())
	ExtendedLogMsg("Serving Metrics requests on: http://%s%s", addr, c.MetricsPath)

	// Spawn a new go routine to listen on specified endpoint
	go func() {
		err := http.ListenAndServe(addr, nil)
		if err != nil {
			FatalLogMsg("failed to listen on address %v: %s", addr, err)
		}
	}()
}
