// SPDX-License-Identifier: MIT

package util

import (
	"fmt"
	"io"
	"runtime/debug"
)

// SafeGo wraps goroutine execution with panic recovery.
//
// This is CRITICAL for 24/7 unattended operation. Any panic in a goroutine
// would normally crash the entire application. SafeGo ensures panics are:
//  1. Logged with stack traces for debugging
//  2. Recovered to prevent application crash
//  3. Optionally reported to a callback for monitoring
//
// Example:
//
//	SafeGo("stream-monitor", logger, func() {
//	    // Your goroutine code here
//	    // If this panics, it will be caught and logged
//	}, nil)
func SafeGo(name string, logger io.Writer, fn func(), onPanic func(interface{}, []byte)) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()

				// Log the panic
				if logger != nil {
					_, _ = fmt.Fprintf(logger, "[PANIC in %s] %v\n%s\n", name, r, stack)
				}

				// Call panic callback if provided
				if onPanic != nil {
					onPanic(r, stack)
				}
			}
		}()

		// Execute the function
		fn()
	}()
}

// SafeGoWithRecover wraps goroutine execution with panic recovery and error channel.
//
// Similar to SafeGo but sends recovered panics to an error channel for handling.
// The error channel is closed when the goroutine exits normally.
//
// Example:
//
//	errCh := make(chan error, 1)
//	SafeGoWithRecover("worker", logger, func() error {
//	    // Your goroutine code here
//	    return nil
//	}, errCh, nil)
//
//	if err := <-errCh; err != nil {
//	    log.Printf("Goroutine failed: %v", err)
//	}
func SafeGoWithRecover(name string, logger io.Writer, fn func() error, errCh chan<- error, onPanic func(interface{}, []byte)) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()

				// Log the panic
				if logger != nil {
					_, _ = fmt.Fprintf(logger, "[PANIC in %s] %v\n%s\n", name, r, stack)
				}

				// Call panic callback if provided
				if onPanic != nil {
					onPanic(r, stack)
				}

				// Send panic as error to channel and close it so callers
				// using for-range or a second receive do not block forever.
				if errCh != nil {
					errCh <- fmt.Errorf("panic in %s: %v", name, r)
					close(errCh)
				}
			}
		}()

		// Execute the function
		err := fn()

		// Send result to error channel
		if errCh != nil {
			if err != nil {
				errCh <- err
			}
			close(errCh)
		}
	}()
}

// RecoverToPanic wraps a function call and converts panics to errors.
//
// This is useful for testing or when you want to handle panics as errors
// instead of letting them propagate.
//
// Example:
//
//	err := RecoverToPanic(func() error {
//	    // Code that might panic
//	    panic("something went wrong")
//	    return nil
//	})
//	// err will contain the panic message
func RecoverToPanic(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	return fn()
}
