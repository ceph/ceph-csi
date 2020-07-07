package util

import (
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog"
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
	err := http.ListenAndServe(addr, nil)
	if err != nil {
		klog.Fatalln(err)
	}
}
