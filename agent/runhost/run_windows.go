//go:build windows

package runhost

import (
	"golang.org/x/sys/windows/svc"
)

type handler struct {
	run func()
}

func Run(name string, fn func()) error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if !isService {
		fn()
		return nil
	}
	return svc.Run(name, &handler{run: fn})
}

func (h *handler) Execute(_ []string, changes <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.run()
	}()

	status <- svc.Status{State: svc.StartPending}
	status <- svc.Status{
		State:   svc.Running,
		Accepts: svc.AcceptStop | svc.AcceptShutdown,
	}

	for {
		select {
		case <-done:
			status <- svc.Status{State: svc.StopPending}
			return false, 0
		case change, ok := <-changes:
			if !ok {
				status <- svc.Status{State: svc.StopPending}
				return false, 0
			}
			switch change.Cmd {
			case svc.Interrogate:
				status <- change.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				return false, 0
			}
		}
	}
}
