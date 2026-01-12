//go:build windows

package telemetry

import (
	"context"
	"errors"
	"runtime"
	"strconv"
	"strings"
	"unsafe"

	"mosona-manager/agent/types"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func CollectHostInfo(ctx context.Context) types.Info {
	_ = ctx

	var info types.Info

	info.SystemVersion = firstNonEmpty(readWindowsProductName(), "Windows")
	info.Uptime = int64(getTickCount64() / 1000)
	info.CpuName = strings.TrimSpace(firstNonEmpty(readCPUNameFromRegistry(), runtime.GOARCH))

	cores, threads, err := cpuCoreAndThreadCount()
	if err != nil || cores <= 0 || threads <= 0 {
		// fallback
		threads = runtime.NumCPU()
		if threads <= 0 {
			threads = 1
		}
		cores = threads
	}
	info.CpuC = cores
	info.CpuT = threads

	info.KernelVersion = strings.TrimSpace(firstNonEmpty(getWindowsKernelVersionString(), ""))
	info.Architecture = runtime.GOARCH

	return info
}

func getTickCount64() uint64 {
	k32 := windows.NewLazySystemDLL("kernel32.dll")
	proc := k32.NewProc("GetTickCount64")
	if err := proc.Find(); err == nil {
		r, _, _ := proc.Call()
		return uint64(r)
	}
	proc2 := k32.NewProc("GetTickCount")
	if err := proc2.Find(); err == nil {
		r, _, _ := proc2.Call()
		return uint64(uint32(r))
	}
	return 0
}

func readWindowsProductName() string {
	// HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProductName
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`,
		registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()

	s, _, err := k.GetStringValue("ProductName")
	if err != nil {
		return ""
	}
	return s
}

func readCPUNameFromRegistry() string {
	// HKLM\HARDWARE\DESCRIPTION\System\CentralProcessor\0\ProcessorNameString
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`HARDWARE\DESCRIPTION\System\CentralProcessor\0`,
		registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()

	s, _, err := k.GetStringValue("ProcessorNameString")
	if err != nil {
		return ""
	}
	return s
}

func getWindowsKernelVersionString() string {
	// Use RtlGetVersion to avoid compatibility lies.
	type osVersionInfoExW struct {
		DwOSVersionInfoSize uint32
		DwMajorVersion      uint32
		DwMinorVersion      uint32
		DwBuildNumber       uint32
		DwPlatformId        uint32
		SzCSDVersion        [128]uint16
		WServicePackMajor   uint16
		WServicePackMinor   uint16
		WSuiteMask          uint16
		WProductType        byte
		WReserved           byte
	}

	mod := windows.NewLazySystemDLL("ntdll.dll")
	proc := mod.NewProc("RtlGetVersion")

	var v osVersionInfoExW
	v.DwOSVersionInfoSize = uint32(unsafe.Sizeof(v))

	r1, _, _ := proc.Call(uintptr(unsafe.Pointer(&v)))
	// STATUS_SUCCESS == 0
	if r1 != 0 {
		return ""
	}

	// e.g. "10.0.22631"
	return strconv.FormatUint(uint64(v.DwMajorVersion), 10) + "." +
		strconv.FormatUint(uint64(v.DwMinorVersion), 10) + "." +
		strconv.FormatUint(uint64(v.DwBuildNumber), 10)
}

// cpuCoreAndThreadCount returns (physical cores, logical processors).
// Uses GetLogicalProcessorInformationEx for accuracy (multi-socket friendly).
func cpuCoreAndThreadCount() (int, int, error) {
	// Constants
	const relationProcessorCore = 0

	k32 := windows.NewLazySystemDLL("kernel32.dll")
	proc := k32.NewProc("GetLogicalProcessorInformationEx")

	// First call to get buffer size
	var length uint32
	r1, _, e1 := proc.Call(uintptr(relationProcessorCore), 0, uintptr(unsafe.Pointer(&length)))
	if r1 != 0 {
		// should not succeed with null buffer; but if it does, length is set
	}
	if length == 0 {
		if e1 != nil && e1 != windows.ERROR_INSUFFICIENT_BUFFER {
			return 0, 0, errors.New("GetLogicalProcessorInformationEx: size query failed")
		}
	}

	buf := make([]byte, length)
	r1, _, e2 := proc.Call(
		uintptr(relationProcessorCore),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&length)),
	)
	if r1 == 0 {
		_ = e2
		return 0, 0, errors.New("GetLogicalProcessorInformationEx: call failed")
	}

	cores := 0
	threads := 0

	// Parse SYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX list
	// Layout:
	//  ULONG Relationship;
	//  ULONG Size;
	//  union { PROCESSOR_RELATIONSHIP ... }
	off := 0
	for off+8 <= int(length) {
		rel := *(*uint32)(unsafe.Pointer(&buf[off]))
		sz := *(*uint32)(unsafe.Pointer(&buf[off+4]))
		if sz == 0 || off+int(sz) > int(length) {
			break
		}
		if rel == relationProcessorCore {
			cores++

			// PROCESSOR_RELATIONSHIP starts at off+8:
			//  BYTE Flags;
			//  BYTE EfficiencyClass;
			//  BYTE Reserved[20];
			//  WORD GroupCount;
			//  GROUP_AFFINITY GroupMask[ANYSIZE_ARRAY];
			//
			// We only need GroupCount and count bits in each GroupMask.Mask.
			base := off + 8
			if base+24 > int(length) { // up to GroupCount (approx)
				off += int(sz)
				continue
			}
			// GroupCount is uint16 at base+22 (Flags(1)+Eff(1)+Res[20]=22)
			groupCount := *(*uint16)(unsafe.Pointer(&buf[base+22]))
			gmOff := base + 24 // start of GROUP_AFFINITY array

			for i := 0; i < int(groupCount); i++ {
				// GROUP_AFFINITY:
				// KAFFINITY Mask (uintptr)
				// WORD Group
				// WORD Reserved[3]
				// Size is 8+2+6 on 64-bit? Mask is uintptr (8 on amd64, 4 on 386)
				// We compute stride accordingly.
				mask := *(*uintptr)(unsafe.Pointer(&buf[gmOff]))
				threads += popCountUintptr(mask)

				stride := int(unsafe.Sizeof(uintptr(0)) + 2 + 6)
				gmOff += stride
				if gmOff > off+int(sz) {
					break
				}
			}
		}
		off += int(sz)
	}

	if cores <= 0 || threads <= 0 {
		return 0, 0, errors.New("cpu parse failed")
	}
	return cores, threads, nil
}

func popCountUintptr(x uintptr) int {
	n := 0
	for x != 0 {
		x &= x - 1
		n++
	}
	return n
}
