package node

import (
	"net/http"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type ControllerMetrics struct {
	Nodes       prometheus.Gauge
	SyncErrors  prometheus.Counter
	SyncSeconds *prometheus.CounterVec
}

func NewControllerMetrics() *ControllerMetrics {
	nodes := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "kubemachine",
		Name:      "nodes",
		Help:      "Number of nodes managed by kubmachine",
	})
	syncErrors := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "kubemachine",
		Subsystem: "controller",
		Name:      "sync_errors_total",
		Help:      "Total number of errors during during sync",
	})
	syncSeconds := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kubemachine",
		Subsystem: "controller",
		Name:      "sync_seconds",
		Help:      "Total time spend syncing in a phase in seconds",
	}, []string{"phase"})

	prometheus.MustRegister(nodes, syncErrors, syncSeconds)

	return &ControllerMetrics{
		Nodes:       nodes,
		SyncErrors:  syncErrors,
		SyncSeconds: syncSeconds,
	}
}

func (cm *ControllerMetrics) Serve(addr string) {
	if addr != "" {
		http.Handle("/metrics", promhttp.Handler())

		glog.Infof("Starting prometheus, listening on %s", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			glog.V(1).Info(err)
		}
	}
}
