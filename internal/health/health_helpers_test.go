package health

// mockProvider implements StatusProvider for testing.
type mockProvider struct {
	services []ServiceInfo
}

func (m *mockProvider) Services() []ServiceInfo {
	return m.services
}

// containsStr is a helper that checks if s contains substr.
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
