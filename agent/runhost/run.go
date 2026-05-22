//go:build !windows

package runhost

func Run(_ string, fn func()) error {
	fn()
	return nil
}
