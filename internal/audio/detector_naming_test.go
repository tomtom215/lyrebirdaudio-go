package audio

import (
	"testing"
)

// TestDeviceSanitizedNames verifies sanitized name generation for config lookup.
func TestDeviceSanitizedNames(t *testing.T) {
	tests := []struct {
		name             string
		deviceName       string
		wantFriendlyName string
	}{
		{
			name:             "Blue Yeti",
			deviceName:       "YetiStereoMicrophone",
			wantFriendlyName: "YetiStereoMicrophone",
		},
		{
			name:             "with spaces",
			deviceName:       "USB Audio Device",
			wantFriendlyName: "USB_Audio_Device",
		},
		{
			name:             "with special chars",
			deviceName:       "C-Media USB Audio",
			wantFriendlyName: "C_Media_USB_Audio",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dev := &Device{
				Name: tt.deviceName,
			}

			friendlyName := dev.FriendlyName()
			if friendlyName != tt.wantFriendlyName {
				t.Errorf("FriendlyName() = %q, want %q", friendlyName, tt.wantFriendlyName)
			}
		})
	}
}

// TestDeviceFullDeviceID verifies FullDeviceID formatting.
func TestDeviceFullDeviceID(t *testing.T) {
	tests := []struct {
		name     string
		deviceID string
		want     string
	}{
		{
			name:     "standard USB device ID",
			deviceID: "usb-Manufacturer_Model_Serial-00",
			want:     "USB_MANUFACTURER_MODEL_SERIAL_00",
		},
		{
			name:     "device ID without usb- prefix",
			deviceID: "Manufacturer_Model_Serial-00",
			want:     "USB_MANUFACTURER_MODEL_SERIAL_00",
		},
		{
			name:     "device ID with special characters",
			deviceID: "usb-Blue_Microphones_Yeti_Stereo_Microphone_797_2020_10_29_53769-00",
			want:     "USB_BLUE_MICROPHONES_YETI_STEREO_MICROPHONE_797_2020_10_29_53769_00",
		},
		{
			name:     "empty device ID",
			deviceID: "",
			want:     "",
		},
		{
			name:     "device ID with consecutive underscores",
			deviceID: "usb-Manufacturer__Model___Serial-00",
			want:     "USB_MANUFACTURER_MODEL_SERIAL_00",
		},
		{
			name:     "device ID with hyphens",
			deviceID: "usb-My-Device-Name-123",
			want:     "USB_MY_DEVICE_NAME_123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dev := &Device{DeviceID: tt.deviceID}
			got := dev.FullDeviceID()
			if got != tt.want {
				t.Errorf("FullDeviceID() = %q, want %q", got, tt.want)
			}
		})
	}
}
