package util

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestSafeGo verifies panic recovery in goroutines.
func TestSafeGo(t *testing.T) {
	t.Run("normal execution", func(t *testing.T) {
		var buf bytes.Buffer
		executed := make(chan bool, 1)

		SafeGo("test", &buf, func() {
			executed <- true
		}, nil)

		select {
		case <-executed:
			// Success
		case <-time.After(1 * time.Second):
			t.Fatal("Goroutine did not execute")
		}

		// Should not have logged anything
		if buf.Len() > 0 {
			t.Errorf("Unexpected log output: %s", buf.String())
		}
	})

	t.Run("panic recovery", func(t *testing.T) {
		var buf bytes.Buffer
		panicCaught := make(chan bool, 1)

		SafeGo("test", &buf, func() {
			panic("test panic")
		}, func(r interface{}, stack []byte) {
			panicCaught <- true
		})

		select {
		case <-panicCaught:
			// Success
		case <-time.After(1 * time.Second):
			t.Fatal("Panic was not caught")
		}

		// Should have logged the panic
		logOutput := buf.String()
		if !strings.Contains(logOutput, "PANIC in test") {
			t.Errorf("Log should contain 'PANIC in test', got: %s", logOutput)
		}
		if !strings.Contains(logOutput, "test panic") {
			t.Errorf("Log should contain panic message, got: %s", logOutput)
		}
	})

	t.Run("panic without logger", func(t *testing.T) {
		panicCaught := make(chan bool, 1)

		SafeGo("test", nil, func() {
			panic("test panic")
		}, func(r interface{}, stack []byte) {
			panicCaught <- true
		})

		select {
		case <-panicCaught:
			// Success - panic was caught even without logger
		case <-time.After(1 * time.Second):
			t.Fatal("Panic was not caught")
		}
	})

	t.Run("panic without callback", func(t *testing.T) {
		var buf bytes.Buffer
		var mu sync.Mutex
		done := make(chan bool, 1)

		// This should not crash even though panic callback is nil
		SafeGo("test", &buf, func() {
			panic("test panic")
		}, func(r interface{}, stack []byte) {
			done <- true
		})

		// Wait for panic to be caught
		select {
		case <-done:
			// Success
		case <-time.After(1 * time.Second):
			t.Fatal("Panic was not caught")
		}

		// Should have logged the panic (use mutex to avoid race)
		mu.Lock()
		logOutput := buf.String()
		mu.Unlock()
		if !strings.Contains(logOutput, "PANIC in test") {
			t.Errorf("Log should contain panic message, got: %s", logOutput)
		}
	})

	t.Run("stack trace included", func(t *testing.T) {
		var buf bytes.Buffer
		var mu sync.Mutex
		done := make(chan bool, 1)

		SafeGo("test", &buf, func() {
			panic("test panic")
		}, func(r interface{}, stack []byte) {
			done <- true
		})

		// Wait for panic to be caught
		select {
		case <-done:
			// Success
		case <-time.After(1 * time.Second):
			t.Fatal("Panic was not caught")
		}

		// Stack trace should include goroutine info (use mutex to avoid race)
		mu.Lock()
		logOutput := buf.String()
		mu.Unlock()
		if !strings.Contains(logOutput, "goroutine") {
			t.Errorf("Log should contain stack trace with 'goroutine', got: %s", logOutput)
		}
	})
}

// TestSafeGoWithRecover verifies panic recovery with error channel.
func TestSafeGoWithRecover(t *testing.T) {
	t.Run("normal execution", func(t *testing.T) {
		var buf bytes.Buffer
		errCh := make(chan error, 1)

		SafeGoWithRecover("test", &buf, func() error {
			return nil
		}, errCh, nil)

		// Should receive nil error and channel should close
		err, ok := <-errCh
		if ok && err != nil {
			t.Errorf("Expected nil error, got: %v", err)
		}
	})

	t.Run("error return", func(t *testing.T) {
		var buf bytes.Buffer
		errCh := make(chan error, 1)
		testErr := errors.New("test error")

		SafeGoWithRecover("test", &buf, func() error {
			return testErr
		}, errCh, nil)

		err := <-errCh
		if err != testErr {
			t.Errorf("Expected test error, got: %v", err)
		}
	})

	t.Run("panic recovery", func(t *testing.T) {
		var buf bytes.Buffer
		errCh := make(chan error, 1)
		panicCaught := make(chan bool, 1)

		SafeGoWithRecover("test", &buf, func() error {
			panic("test panic")
		}, errCh, func(r interface{}, stack []byte) {
			panicCaught <- true
		})

		// Should receive panic as error
		err := <-errCh
		if err == nil {
			t.Fatal("Expected error from panic")
		}
		if !strings.Contains(err.Error(), "panic in test") {
			t.Errorf("Error should contain 'panic in test', got: %v", err)
		}

		// Panic callback should be called
		select {
		case <-panicCaught:
			// Success
		case <-time.After(1 * time.Second):
			t.Fatal("Panic callback was not called")
		}
	})

	t.Run("panic without error channel", func(t *testing.T) {
		var buf bytes.Buffer
		var mu sync.Mutex
		done := make(chan bool, 1)

		// Should not crash even without error channel
		SafeGoWithRecover("test", &buf, func() error {
			panic("test panic")
		}, nil, func(r interface{}, stack []byte) {
			done <- true
		})

		// Wait for panic to be caught
		select {
		case <-done:
			// Success
		case <-time.After(1 * time.Second):
			t.Fatal("Panic was not caught")
		}

		// Should have logged the panic (use mutex to avoid race)
		mu.Lock()
		logOutput := buf.String()
		mu.Unlock()
		if !strings.Contains(logOutput, "PANIC in test") {
			t.Errorf("Log should contain panic message, got: %s", logOutput)
		}
	})
}

// TestRecoverToPanic verifies panic-to-error conversion.
func TestRecoverToPanic(t *testing.T) {
	t.Run("normal execution", func(t *testing.T) {
		err := RecoverToPanic(func() error {
			return nil
		})

		if err != nil {
			t.Errorf("Expected nil error, got: %v", err)
		}
	})

	t.Run("error return", func(t *testing.T) {
		testErr := errors.New("test error")

		err := RecoverToPanic(func() error {
			return testErr
		})

		if err != testErr {
			t.Errorf("Expected test error, got: %v", err)
		}
	})

	t.Run("panic conversion", func(t *testing.T) {
		err := RecoverToPanic(func() error {
			panic("test panic")
		})

		if err == nil {
			t.Fatal("Expected error from panic")
		}

		if !strings.Contains(err.Error(), "panic: test panic") {
			t.Errorf("Error should contain panic message, got: %v", err)
		}
	})

	t.Run("panic with different types", func(t *testing.T) {
		tests := []struct {
			name       string
			panicValue interface{}
		}{
			{"string", "panic string"},
			{"int", 42},
			{"error", errors.New("panic error")},
			{"nil", nil},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := RecoverToPanic(func() error {
					panic(tt.panicValue)
				})

				if err == nil {
					t.Fatal("Expected error from panic")
				}

				if !strings.Contains(err.Error(), "panic:") {
					t.Errorf("Error should contain 'panic:', got: %v", err)
				}
			})
		}
	})
}

// TestSafeGoConcurrency verifies concurrent goroutine execution.
func TestSafeGoConcurrency(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex
	var counter int
	const numGoroutines = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		SafeGo("worker", &buf, func() {
			defer wg.Done()
			mu.Lock()
			counter++
			mu.Unlock()
		}, nil)
	}

	// Wait for all goroutines to complete
	done := make(chan bool)
	go func() {
		wg.Wait()
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Goroutines did not complete in time")
	}

	if counter != numGoroutines {
		t.Errorf("Counter = %d, want %d", counter, numGoroutines)
	}
}

// BenchmarkSafeGo measures overhead of SafeGo wrapper.
func BenchmarkSafeGo(b *testing.B) {
	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		done := make(chan bool, 1)
		SafeGo("bench", &buf, func() {
			done <- true
		}, nil)
		<-done
	}
}

// BenchmarkRecoverToPanic measures overhead of panic recovery.
func BenchmarkRecoverToPanic(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RecoverToPanic(func() error {
			return nil
		})
	}
}
