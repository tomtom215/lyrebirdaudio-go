// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"testing"
)

func TestIsPortOpenInvalidAddresses(t *testing.T) {
	tests := []struct {
		name string
		addr string
	}{
		{name: "empty address", addr: ""},
		{name: "no port", addr: "localhost"},
		{name: "invalid host", addr: "this-host-does-not-exist.invalid:80"},
		{name: "port zero", addr: "127.0.0.1:0"},
		{name: "high unused port", addr: "127.0.0.1:59999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPortOpen(tt.addr)
			if result {
				t.Errorf("isPortOpen(%q) = true, expected false for unreachable address", tt.addr)
			}
		})
	}
}
