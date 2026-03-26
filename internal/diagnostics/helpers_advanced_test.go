// SPDX-License-Identifier: MIT

//go:build linux

package diagnostics

import (
	"testing"
)

func TestEvaluateKernelModules(t *testing.T) {
	tests := []struct {
		name       string
		data       string
		required   []string
		optional   []string
		wantStatus CheckStatus
		wantSubstr string
	}{
		{
			name:       "all modules loaded",
			data:       "snd_usb_audio 200704 0\nsnd_pcm 135168 2\nsnd_hwdep 16384 1\nsnd_usbmidi_lib 32768 1\n",
			required:   []string{"snd_usb_audio"},
			optional:   []string{"snd_pcm", "snd_hwdep", "snd_usbmidi_lib"},
			wantStatus: StatusOK,
			wantSubstr: "All audio kernel modules loaded",
		},
		{
			name:       "missing required module",
			data:       "snd_pcm 135168 2\nsnd_hwdep 16384 1\n",
			required:   []string{"snd_usb_audio"},
			optional:   []string{"snd_pcm"},
			wantStatus: StatusCritical,
			wantSubstr: "Missing required",
		},
		{
			name:       "missing optional module",
			data:       "snd_usb_audio 200704 0\nsnd_pcm 135168 2\n",
			required:   []string{"snd_usb_audio"},
			optional:   []string{"snd_pcm", "snd_hwdep"},
			wantStatus: StatusWarning,
			wantSubstr: "missing optional",
		},
		{
			name:       "empty modules data",
			data:       "",
			required:   []string{"snd_usb_audio"},
			optional:   []string{},
			wantStatus: StatusCritical,
			wantSubstr: "Missing required",
		},
		{
			name:       "no required or optional",
			data:       "snd_usb_audio 200704 0\n",
			required:   []string{},
			optional:   []string{},
			wantStatus: StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, msg := evaluateKernelModules(tt.data, tt.required, tt.optional)
			if status != tt.wantStatus {
				t.Errorf("status = %v, want %v (msg: %s)", status, tt.wantStatus, msg)
			}
			if tt.wantSubstr != "" && !contains(msg, tt.wantSubstr) {
				t.Errorf("message %q does not contain %q", msg, tt.wantSubstr)
			}
		})
	}
}

func TestEvaluateUSBStability(t *testing.T) {
	tests := []struct {
		name       string
		dmesg      string
		wantStatus CheckStatus
		wantSubstr string
	}{
		{
			name:       "no errors",
			dmesg:      "some normal kernel message\nanother message\n",
			wantStatus: StatusOK,
			wantSubstr: "No USB errors",
		},
		{
			name:       "few errors within range",
			dmesg:      "usb 1-1: device descriptor read error\nusb 1-2: some other message\n",
			wantStatus: StatusOK,
			wantSubstr: "within normal range",
		},
		{
			name: "many USB errors triggers warning",
			dmesg: "usb 1-1: error 1\nusb 1-1: error 2\nusb 1-1: error 3\n" +
				"usb 1-1: error 4\nusb 1-1: error 5\nusb 1-1: error 6\n" +
				"usb 1-1: error 7\nusb 1-1: error 8\nusb 1-1: error 9\n" +
				"usb 1-1: error 10\nusb 1-1: error 11\n",
			wantStatus: StatusWarning,
			wantSubstr: "USB errors",
		},
		{
			name: "many disconnects triggers warning",
			dmesg: "usb 1-1: USB disconnect\nusb 1-1: USB disconnect\n" +
				"usb 1-1: USB disconnect\nusb 1-1: USB disconnect\n" +
				"usb 1-1: USB disconnect\nusb 1-1: USB disconnect\n",
			wantStatus: StatusWarning,
			wantSubstr: "disconnects",
		},
		{
			name:       "usb timeout counted as error",
			dmesg:      "usb 1-1: device not responding to setup address timeout\n",
			wantStatus: StatusOK,
			wantSubstr: "within normal range",
		},
		{
			name:       "usb cannot counted as error",
			dmesg:      "usb 1-1: cannot reset port\n",
			wantStatus: StatusOK,
			wantSubstr: "within normal range",
		},
		{
			name:       "empty dmesg",
			dmesg:      "",
			wantStatus: StatusOK,
			wantSubstr: "No USB errors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, msg, _ := evaluateUSBStability(tt.dmesg)
			if status != tt.wantStatus {
				t.Errorf("status = %v, want %v (msg: %s)", status, tt.wantStatus, msg)
			}
			if tt.wantSubstr != "" && !contains(msg, tt.wantSubstr) {
				t.Errorf("message %q does not contain %q", msg, tt.wantSubstr)
			}
		})
	}
}

func TestEvaluateResourceLimits(t *testing.T) {
	tests := []struct {
		name       string
		data       string
		wantStatus CheckStatus
		wantSubstr string
	}{
		{
			name:       "adequate limits",
			data:       "Max open files            65536                65536                files\nMax processes             32768                32768                processes\n",
			wantStatus: StatusOK,
			wantSubstr: "adequate",
		},
		{
			name:       "low open files",
			data:       "Max open files            256                  1024                 files\nMax processes             32768                32768                processes\n",
			wantStatus: StatusWarning,
			wantSubstr: "low open files",
		},
		{
			name:       "low processes",
			data:       "Max open files            65536                65536                files     \nMax processes             128                  512                  processes \n",
			wantStatus: StatusWarning,
			wantSubstr: "low process limit",
		},
		{
			name:       "both low",
			data:       "Max open files            256                  1024                 files     \nMax processes             128                  512                  processes \n",
			wantStatus: StatusWarning,
			wantSubstr: "low open files",
		},
		{
			name:       "empty data",
			data:       "",
			wantStatus: StatusOK,
			wantSubstr: "adequate",
		},
		{
			name:       "unlimited fields",
			data:       "Max open files            unlimited            unlimited            files\nMax processes             unlimited            unlimited            processes\n",
			wantStatus: StatusOK,
			wantSubstr: "adequate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, msg := evaluateResourceLimits(tt.data)
			if status != tt.wantStatus {
				t.Errorf("status = %v, want %v (msg: %s)", status, tt.wantStatus, msg)
			}
			if tt.wantSubstr != "" && !contains(msg, tt.wantSubstr) {
				t.Errorf("message %q does not contain %q", msg, tt.wantSubstr)
			}
		})
	}
}

// contains is a simple substring check helper for tests.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
