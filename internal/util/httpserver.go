package util

import (
	"net"
	"net/http"
	_ "net/http/pprof" //nolint:gosec // G108, serves /debug/pprof/ on the metrics server
	"net/url"
	"strconv"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/ceph/ceph-csi/internal/util/log"
)

// ValidateURL validates the url.
func ValidateURL(c *Config) error {
	_, err := url.Parse(c.MetricsPath)

	return err
}

// StartMetricsServer starts http server.
func StartMetricsServer(c *Config) {
	addr := net.JoinHostPort(c.MetricsIP, strconv.Itoa(c.MetricsPort))
	http.Handle(c.MetricsPath, promhttp.Handler())

	//nolint:gosec // TODO: add support for passing timeouts
	err := http.ListenAndServe(addr, nil)
	if err != nil {
		log.FatalLogMsg("failed to listen on address %v: %s", addr, err)
	}
}
