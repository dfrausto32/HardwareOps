package metrics

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry             *prometheus.Registry
	httpRequests         *prometheus.CounterVec
	httpDuration         *prometheus.HistogramVec
	deviceStatus         *prometheus.GaugeVec
	deviceTotal          prometheus.Gauge
	dbOpenConns          prometheus.Gauge
	dbInUse              prometheus.Gauge
	dbWaitCount          prometheus.Gauge
	s3ObjectsTotal       prometheus.Gauge
	s3BytesTotal         prometheus.Gauge
	checkinTotal         *prometheus.CounterVec
	enrollTokenTotal     *prometheus.CounterVec
	enrollTotal          *prometheus.CounterVec
	applyTotal           *prometheus.CounterVec
	preApplyTotal        *prometheus.CounterVec
	artifactUploadTotal  *prometheus.CounterVec
	artifactPresignTotal *prometheus.CounterVec
	rateLimitTotal       *prometheus.CounterVec
	upgradeTotal         *prometheus.CounterVec
	backupTotal          *prometheus.CounterVec
	pendingActionsTotal  *prometheus.CounterVec
}

func New() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector())
	reg.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))

	httpRequests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hwops_http_requests_total",
		Help: "Total HTTP requests by method, route, and status.",
	}, []string{"method", "route", "status"})
	httpDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "hwops_http_request_duration_seconds",
		Help:    "HTTP request duration in seconds by method and route.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})
	deviceTotal := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "hwops_devices_total",
		Help: "Total devices registered.",
	})
	deviceStatus := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hwops_devices_status_total",
		Help: "Devices by status.",
	}, []string{"status"})
	dbOpenConns := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "hwops_db_open_conns",
		Help: "Open database connections.",
	})
	dbInUse := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "hwops_db_in_use",
		Help: "Database connections currently in use.",
	})
	dbWaitCount := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "hwops_db_wait_count",
		Help: "Database connection wait count (cumulative).",
	})
	s3ObjectsTotal := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "hwops_s3_objects_total",
		Help: "Total objects tracked in artifact storage.",
	})
	s3BytesTotal := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "hwops_s3_bytes_total",
		Help: "Total bytes tracked in artifact storage.",
	})
	checkinTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hwops_checkin_total",
		Help: "Device check-ins by status and reason.",
	}, []string{"status", "reason"})
	enrollTokenTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hwops_enrollment_token_total",
		Help: "Enrollment token requests by status and reason.",
	}, []string{"status", "reason"})
	enrollTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hwops_enroll_total",
		Help: "Device enrollments by status and reason.",
	}, []string{"status", "reason"})
	applyTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hwops_apply_total",
		Help: "Apply results by status and component.",
	}, []string{"status", "component"})
	preApplyTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hwops_preapply_total",
		Help: "Pre-apply results by status and component.",
	}, []string{"status", "component"})
	artifactUploadTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hwops_artifact_upload_total",
		Help: "Artifact uploads by status.",
	}, []string{"status"})
	artifactPresignTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hwops_artifact_presign_total",
		Help: "Artifact presign requests by status.",
	}, []string{"status"})
	rateLimitTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hwops_rate_limit_total",
		Help: "Rate limit hits by endpoint.",
	}, []string{"endpoint"})
	upgradeTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hwops_upgrade_total",
		Help: "Upgrade attempts by status.",
	}, []string{"status"})
	backupTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hwops_backup_total",
		Help: "Backup/restore attempts by operation and status.",
	}, []string{"operation", "status"})
	pendingActionsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hwops_pending_actions_total",
		Help: "Pending actions issued to devices by type.",
	}, []string{"type"})

	reg.MustRegister(httpRequests, httpDuration, deviceTotal, deviceStatus, dbOpenConns, dbInUse, dbWaitCount, s3ObjectsTotal, s3BytesTotal, checkinTotal, enrollTokenTotal, enrollTotal, applyTotal, preApplyTotal, artifactUploadTotal, artifactPresignTotal, rateLimitTotal, upgradeTotal, backupTotal, pendingActionsTotal)

	return &Metrics{
		registry:             reg,
		httpRequests:         httpRequests,
		httpDuration:         httpDuration,
		deviceTotal:          deviceTotal,
		deviceStatus:         deviceStatus,
		dbOpenConns:          dbOpenConns,
		dbInUse:              dbInUse,
		dbWaitCount:          dbWaitCount,
		s3ObjectsTotal:       s3ObjectsTotal,
		s3BytesTotal:         s3BytesTotal,
		checkinTotal:         checkinTotal,
		enrollTokenTotal:     enrollTokenTotal,
		enrollTotal:          enrollTotal,
		applyTotal:           applyTotal,
		preApplyTotal:        preApplyTotal,
		artifactUploadTotal:  artifactUploadTotal,
		artifactPresignTotal: artifactPresignTotal,
		rateLimitTotal:       rateLimitTotal,
		upgradeTotal:         upgradeTotal,
		backupTotal:          backupTotal,
		pendingActionsTotal:  pendingActionsTotal,
	}
}

func (m *Metrics) Handler() http.Handler {
	if m == nil || m.registry == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "metrics disabled", http.StatusNotFound)
		})
	}
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) Middleware(next http.Handler) http.Handler {
	if m == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)
		route := chi.RouteContext(r.Context()).RoutePattern()
		if route == "" {
			route = r.URL.Path
		}
		m.httpRequests.WithLabelValues(r.Method, route, strconv.Itoa(rec.status)).Inc()
		m.httpDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	})
}

func (m *Metrics) SetDeviceStatusCounts(total int, counts map[string]int) {
	if m == nil {
		return
	}
	m.deviceTotal.Set(float64(total))
	for status, count := range counts {
		m.deviceStatus.WithLabelValues(status).Set(float64(count))
	}
}

func (m *Metrics) SetDBStats(openConns, inUse int, waitCount int64) {
	if m == nil {
		return
	}
	m.dbOpenConns.Set(float64(openConns))
	m.dbInUse.Set(float64(inUse))
	m.dbWaitCount.Set(float64(waitCount))
}

func (m *Metrics) SetStorageUsage(objects int, bytes int64) {
	if m == nil {
		return
	}
	m.s3ObjectsTotal.Set(float64(objects))
	m.s3BytesTotal.Set(float64(bytes))
}

func (m *Metrics) IncCheckin(status, reason string) {
	if m == nil {
		return
	}
	if status == "" {
		status = "error"
	}
	if reason == "" {
		reason = "unknown"
	}
	m.checkinTotal.WithLabelValues(status, reason).Inc()
}

func (m *Metrics) IncEnrollmentToken(status, reason string) {
	if m == nil {
		return
	}
	if status == "" {
		status = "error"
	}
	if reason == "" {
		reason = "unknown"
	}
	m.enrollTokenTotal.WithLabelValues(status, reason).Inc()
}

func (m *Metrics) IncEnroll(status, reason string) {
	if m == nil {
		return
	}
	if status == "" {
		status = "error"
	}
	if reason == "" {
		reason = "unknown"
	}
	m.enrollTotal.WithLabelValues(status, reason).Inc()
}

func (m *Metrics) IncApply(status, component string) {
	if m == nil {
		return
	}
	if status == "" {
		status = "unknown"
	}
	if component == "" {
		component = "unknown"
	}
	m.applyTotal.WithLabelValues(status, component).Inc()
}

func (m *Metrics) IncPreApply(status, component string) {
	if m == nil {
		return
	}
	if status == "" {
		status = "unknown"
	}
	if component == "" {
		component = "unknown"
	}
	m.preApplyTotal.WithLabelValues(status, component).Inc()
}

func (m *Metrics) IncArtifactUpload(status string) {
	if m == nil {
		return
	}
	if status == "" {
		status = "error"
	}
	m.artifactUploadTotal.WithLabelValues(status).Inc()
}

func (m *Metrics) IncArtifactPresign(status string) {
	if m == nil {
		return
	}
	if status == "" {
		status = "error"
	}
	m.artifactPresignTotal.WithLabelValues(status).Inc()
}

func (m *Metrics) IncRateLimit(endpoint string) {
	if m == nil {
		return
	}
	if endpoint == "" {
		endpoint = "unknown"
	}
	m.rateLimitTotal.WithLabelValues(endpoint).Inc()
}

func (m *Metrics) IncUpgrade(status string) {
	if m == nil {
		return
	}
	if status == "" {
		status = "unknown"
	}
	m.upgradeTotal.WithLabelValues(status).Inc()
}

func (m *Metrics) IncBackup(operation, status string) {
	if m == nil {
		return
	}
	if operation == "" {
		operation = "backup"
	}
	if status == "" {
		status = "unknown"
	}
	m.backupTotal.WithLabelValues(operation, status).Inc()
}

func (m *Metrics) IncPendingAction(actionType string) {
	if m == nil {
		return
	}
	if actionType == "" {
		actionType = "unknown"
	}
	m.pendingActionsTotal.WithLabelValues(actionType).Inc()
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijacker not supported")
	}
	return hijacker.Hijack()
}

func (s *statusRecorder) Flush() {
	if flusher, ok := s.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (s *statusRecorder) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := s.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}
