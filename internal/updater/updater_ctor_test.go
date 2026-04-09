package updater

import (
	"net/http"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	u := New()
	if u == nil {
		t.Fatal("New() returned nil")
	}
	if u.owner != DefaultOwner {
		t.Errorf("owner = %q, want %q", u.owner, DefaultOwner)
	}
	if u.repo != DefaultRepo {
		t.Errorf("repo = %q, want %q", u.repo, DefaultRepo)
	}
}

func TestNewWithOptions(t *testing.T) {
	u := New(
		WithOwner("testowner"),
		WithRepo("testrepo"),
		WithCurrentVersion("v1.0.0"),
	)

	if u.owner != "testowner" {
		t.Errorf("owner = %q, want %q", u.owner, "testowner")
	}
	if u.repo != "testrepo" {
		t.Errorf("repo = %q, want %q", u.repo, "testrepo")
	}
	if u.currentVersion != "v1.0.0" {
		t.Errorf("currentVersion = %q, want %q", u.currentVersion, "v1.0.0")
	}
}

func TestWithHTTPClient(t *testing.T) {
	customClient := &http.Client{Timeout: 60 * time.Second}
	u := New(WithHTTPClient(customClient))
	if u.httpClient != customClient {
		t.Error("httpClient was not set correctly")
	}
}
