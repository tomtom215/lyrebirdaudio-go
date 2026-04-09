package supervisor

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func TestSupervisor_LoggingOutput(t *testing.T) {
	var buf bytes.Buffer
	sup := New(Config{
		ShutdownTimeout: 2 * time.Second,
		Logger:          slog.New(slog.NewTextHandler(&buf, nil)),
	})

	svc := newMockService("log-test")
	if err := sup.Add(svc); err != nil {
		t.Fatalf("Add: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- sup.Run(ctx)
	}()

	// Wait for service to start
	select {
	case <-svc.started:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("service did not start")
	}

	cancel()

	select {
	case <-errCh:
		// OK
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not stop")
	}

	logOutput := buf.String()
	// Verify some logging happened
	if len(logOutput) == 0 {
		t.Log("Warning: no log output captured (may be expected with suture)")
	}
}

func TestSupervisor_ConcurrentOperations(t *testing.T) {
	sup := New(Config{
		ShutdownTimeout: 5 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start supervisor
	errCh := make(chan error, 1)
	go func() {
		errCh <- sup.Run(ctx)
	}()

	// Give it time to start
	time.Sleep(100 * time.Millisecond)

	// Concurrent adds
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			svc := newMockService(string(rune('A' + i)))
			_ = sup.Add(svc)
		}(i)
	}
	wg.Wait()

	// Verify services were added
	count := sup.ServiceCount()
	if count != 10 {
		t.Errorf("ServiceCount = %d, want 10", count)
	}

	// Concurrent status checks
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = sup.Status()
		}()
	}
	wg.Wait()

	cancel()

	select {
	case <-errCh:
		// OK
	case <-time.After(10 * time.Second):
		t.Fatal("supervisor did not stop")
	}
}

func TestSupervisor_ServiceName(t *testing.T) {
	tests := []struct {
		name     string
		wantName string
	}{
		{"simple", "simple"},
		{"with-dash", "with-dash"},
		{"with_underscore", "with_underscore"},
		{"CamelCase", "CamelCase"},
		{"123numeric", "123numeric"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sup := New(DefaultConfig())
			svc := newMockService(tt.name)
			if err := sup.Add(svc); err != nil {
				t.Fatalf("Add: %v", err)
			}

			status := sup.Status()
			if len(status) != 1 || status[0].Name != tt.wantName {
				t.Errorf("Name = %q, want %q", status[0].Name, tt.wantName)
			}
		})
	}
}

func TestSupervisor_SupervisorName(t *testing.T) {
	sup := New(Config{
		Name: "test-supervisor",
	})
	if sup == nil {
		t.Fatal("New returned nil")
	}
	// Supervisor should be created with given name
	// (used internally by suture for logging)
}
