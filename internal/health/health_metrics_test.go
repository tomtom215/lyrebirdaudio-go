package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMetricsEndpoint(t *testing.T) {
	provider := &mockProvider{
		services: []ServiceInfo{
			{Name: "blue_yeti", State: "running", Healthy: true, Uptime: 3600 * time.Second, Restarts: 2, Failures: 5},
			{Name: "usb_mic", State: "failed", Healthy: false, Uptime: 0, Restarts: 10, Failures: 20},
		},
	}
	sysProvider := &mockSysInfoProvider{
		info: SystemInfo{
			DiskFreeBytes:  42 * 1024 * 1024 * 1024,
			DiskTotalBytes: 64 * 1024 * 1024 * 1024,
			DiskLowWarning: false,
			NTPSynced:      true,
		},
	}

	h := NewHandler(provider).WithSystemInfo(sysProvider)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("metrics status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()

	wantContains := []string{
		"lyrebird_stream_healthy",
		`lyrebird_stream_healthy{stream="blue_yeti"} 1`,
		`lyrebird_stream_healthy{stream="usb_mic"} 0`,
		"lyrebird_stream_uptime_seconds",
		"lyrebird_stream_restarts_total",
		`lyrebird_stream_restarts_total{stream="blue_yeti"} 2`,
		`lyrebird_stream_restarts_total{stream="usb_mic"} 10`,
		"lyrebird_stream_failures_total",
		`lyrebird_stream_failures_total{stream="blue_yeti"} 5`,
		"lyrebird_disk_free_bytes",
		"lyrebird_disk_total_bytes",
		"lyrebird_disk_low_warning",
		"lyrebird_ntp_synced",
	}

	for _, want := range wantContains {
		if !containsStr(body, want) {
			t.Errorf("metrics body missing %q", want)
		}
	}
}

func TestMetricsEndpointNoProvider(t *testing.T) {
	h := NewHandler(nil)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestMetricsEndpointMethodNotAllowed(t *testing.T) {
	h := NewHandler(nil)
	req := httptest.NewRequest(http.MethodPost, "/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}
