//go:build darwin

package caffeinate

import "os/exec"

// Start spawns `caffeinate -dims` in its own process group so we can clean
// up the whole tree. -d prevents display sleep, -i prevents idle sleep,
// -m prevents disk sleep, -s prevents system sleep on AC.
//
// We deliberately omit -u: per caffeinate(8), -u defaults to a 5-second
// timeout when no -t is supplied, which would silently exit our child. The
// other four flags are assertion-based and live as long as the process.
func Start() (*Manager, error) {
	return startCmd(exec.Command("caffeinate", "-dims"))
}
