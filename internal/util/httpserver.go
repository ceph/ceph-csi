package util

import (
	"net"
	"net/http"
	"net/http/pprof"
	"net/url"
	runtime_pprof "runtime/pprof"
	"strconv"

	"github.com/ceph/ceph-csi/internal/util/log"

	"github.com/prometheus/client_golang/prometheus/promhttp"
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

func addPath(name string, handler http.Handler) {
	http.Handle(name, handler)
	log.DebugLogMsg("DEBUG: registered profiling handler on /debug/pprof/%s\n", name)
}

// EnableProfiling enables golang profiling.
func EnableProfiling() {
	for _, profile := range runtime_pprof.Profiles() {
		name := profile.Name()
		handler := pprof.Handler(name)
		addPath(name, handler)
	}

	// static profiles as listed in net/http/pprof/pprof.go:init()
	addPath("cmdline", http.HandlerFunc(pprof.Cmdline))
	addPath("profile", http.HandlerFunc(pprof.Profile))
	addPath("symbol", http.HandlerFunc(pprof.Symbol))
	addPath("trace", http.HandlerFunc(pprof.Trace))
}
