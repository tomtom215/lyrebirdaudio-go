package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockSysInfoProvider implements SystemInfoProvider for testing.
type mockSysInfoProvider struct {
	info SystemInfo
}

func (m *mockSysInfoProvider) SystemInfo(context.Context) SystemInfo {
	return m.info
}

func TestWithSystemInfoHealthy(t *testing.T) {
	provider := &mockProvider{
		services: []ServiceInfo{
			{Name: "blue_yeti", State: "running", Healthy: true},
		},
	}
	sysProvider := &mockSysInfoProvider{
		info: SystemInfo{
			DiskFreeBytes:  10 * 1024 * 1024 * 1024,
			DiskTotalBytes: 64 * 1024 * 1024 * 1024,
			DiskLowWarning: false,
			NTPSynced:      true,
		},
	}

	h := NewHandler(provider).WithSystemInfo(sysProvider)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != "healthy" {
		t.Errorf("status = %q, want healthy", resp.Status)
	}
	if resp.System == nil {
		t.Fatal("system field should be present when sysProvider is set")
	}
	if resp.System.DiskLowWarning {
		t.Error("DiskLowWarning should be false")
	}
	if !resp.System.NTPSynced {
		t.Error("NTPSynced should be true")
	}
}

func TestWithSystemInfoDiskLowWarning(t *testing.T) {
	provider := &mockProvider{
		services: []ServiceInfo{
			{Name: "blue_yeti", State: "running", Healthy: true},
		},
	}
	sysProvider := &mockSysInfoProvider{
		info: SystemInfo{
			DiskFreeBytes:  100 * 1024 * 1024, // 100 MB — below threshold
			DiskTotalBytes: 64 * 1024 * 1024 * 1024,
			DiskLowWarning: true,
			NTPSynced:      true,
		},
	}

	h := NewHandler(provider).WithSystemInfo(sysProvider)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 when disk is low", rec.Code)
	}

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != "degraded" {
		t.Errorf("status = %q, want degraded", resp.Status)
	}
}

func TestWithSystemInfoNTPDesynced(t *testing.T) {
	provider := &mockProvider{
		services: []ServiceInfo{
			{Name: "blue_yeti", State: "running", Healthy: true},
		},
	}
	sysProvider := &mockSysInfoProvider{
		info: SystemInfo{
			DiskFreeBytes:  10 * 1024 * 1024 * 1024,
			DiskTotalBytes: 64 * 1024 * 1024 * 1024,
			DiskLowWarning: false,
			NTPSynced:      false,
			NTPMessage:     "NTP not synchronized",
		},
	}

	h := NewHandler(provider).WithSystemInfo(sysProvider)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// NTP desync downgrades the body to "degraded" but is a SOFT warning: the
	// HTTP status must stay 200 so a routine clock re-sync doesn't flap a
	// watchdog or load balancer.
	if resp.Status != "degraded" {
		t.Errorf("status = %q, want degraded when NTP not synced", resp.Status)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("HTTP status = %d, want 200 (NTP desync is a soft warning, not a hard failure)", rec.Code)
	}
	if resp.System.NTPSynced {
		t.Error("NTPSynced should be false")
	}
}
