//go:build !darwin && !linux

package caffeinate

// Start is a no-op on platforms without a known sleep-inhibitor wrapper
// (Windows, BSDs, etc.). The caller falls back to animated-only mode.
func Start() (*Manager, error) {
	return nil, ErrUnsupported
}

// Stop is a no-op stub so callers can use the same defer-based cleanup
// shape on every GOOS. Start always returns (nil, ErrUnsupported) here, so
// the receiver is always nil and there's nothing to release.
func (m *Manager) Stop() error { return nil }
