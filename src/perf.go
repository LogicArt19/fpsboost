package main

// Légèreté de FPS Boost lui-même : mode efficacité Windows + mesure de sa propre consommation

import (
	"runtime"
	"sync/atomic"
	"time"
	"unsafe"
)

var (
	pSetProcessInformation    = modKernel32.NewProc("SetProcessInformation")
	pGetProcessTimes          = modKernel32.NewProc("GetProcessTimes")
	pK32GetProcessMemoryInfo  = modKernel32.NewProc("K32GetProcessMemoryInfo")
)

type powerThrottlingState struct {
	Version     uint32
	ControlMask uint32
	StateMask   uint32
}

type processMemoryCounters struct {
	Cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

// FPS Boost passe derrière le jeu : priorité basse + "mode efficacité" de Windows
func makeSelfLight() {
	runtime.GOMAXPROCS(2)
	self := ^uintptr(0)
	call(pSetPriorityClass, self, prioBelowNormal)
	if pSetProcessInformation.Find() == nil {
		st := powerThrottlingState{Version: 1, ControlMask: 1, StateMask: 1}
		call(pSetProcessInformation, self, 4, uintptr(unsafe.Pointer(&st)), unsafe.Sizeof(st))
	}
}

var (
	selfCPU      float64
	selfMemMB    float64
	lastSelfCPU  uint64
	lastSelfWall time.Time
)

func updateSelfUsage() {
	self := ^uintptr(0)
	var c, e, k, u [2]uint32
	if call(pGetProcessTimes, self, uintptr(unsafe.Pointer(&c)), uintptr(unsafe.Pointer(&e)),
		uintptr(unsafe.Pointer(&k)), uintptr(unsafe.Pointer(&u))) != 0 {
		total := fileTime(k) + fileTime(u)
		now := time.Now()
		if !lastSelfWall.IsZero() {
			wall := now.Sub(lastSelfWall).Seconds() * 1e7 * float64(runtime.NumCPU())
			if wall > 0 {
				selfCPU = clamp(100*float64(total-lastSelfCPU)/wall, 0, 100)
			}
		}
		lastSelfCPU, lastSelfWall = total, now
	}
	var m processMemoryCounters
	m.Cb = uint32(unsafe.Sizeof(m))
	if call(pK32GetProcessMemoryInfo, self, uintptr(unsafe.Pointer(&m)), uintptr(m.Cb)) != 0 {
		selfMemMB = float64(m.PagefileUsage) / (1024 * 1024)
	}
}

// Les capteurs ne travaillent que si quelque chose les affiche
var sensorsWanted int32

func setSensorsWanted(v bool) {
	n := int32(0)
	if v {
		n = 1
	}
	atomic.StoreInt32(&sensorsWanted, n)
}
