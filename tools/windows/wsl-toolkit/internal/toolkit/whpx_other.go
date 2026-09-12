//go:build !windows

package toolkit

// whpxAvailable is unanswerable off Windows, because the accelerator it reports
// on is a Windows optional feature. It answers no with the reason rather than a
// guess, and BsdProbe refuses the whole interface on this host anyway.
func whpxAvailable() (bool, string) {
	return false, "the Windows Hypervisor Platform exists only on a Windows host"
}
