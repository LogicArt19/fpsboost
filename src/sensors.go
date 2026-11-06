package main

// Capteurs : utilisation CPU/GPU, températures CPU/GPU

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	modPdh = syscall.NewLazyDLL("pdh.dll")

	pPdhOpenQueryW               = modPdh.NewProc("PdhOpenQueryW")
	pPdhAddEnglishCounterW       = modPdh.NewProc("PdhAddEnglishCounterW")
	pPdhCollectQueryData         = modPdh.NewProc("PdhCollectQueryData")
	pPdhGetFormattedCounterArray = modPdh.NewProc("PdhGetFormattedCounterArrayW")

	pGetSystemTimes            = modKernel32.NewProc("GetSystemTimes")
	pD3DKMTEnumAdapters2       = modGdi32.NewProc("D3DKMTEnumAdapters2")
	pD3DKMTQueryAdapterInfo    = modGdi32.NewProc("D3DKMTQueryAdapterInfo")
)

type SensorData struct {
	CPUUse, GPUUse   float64
	CPUTemp, GPUTemp float64
	CPUUseOk         bool
	GPUUseOk         bool
	CPUTempOk        bool
	GPUTempOk        bool
}

var (
	sensorsMu sync.Mutex
	sensorVal SensorData
)

func sensors() SensorData {
	sensorsMu.Lock()
	defer sensorsMu.Unlock()
	return sensorVal
}

// ---- PDH ------------------------------------------------------------------

type pdhCounter struct{ q, c uintptr }

func pdhOpen(path string) *pdhCounter {
	if modPdh.Load() != nil {
		return nil
	}
	var q, c uintptr
	if call(pPdhOpenQueryW, 0, 0, uintptr(unsafe.Pointer(&q))) != 0 {
		return nil
	}
	if call(pPdhAddEnglishCounterW, q, uintptr(unsafe.Pointer(w(path))), 0, uintptr(unsafe.Pointer(&c))) != 0 {
		return nil
	}
	call(pPdhCollectQueryData, q)
	return &pdhCounter{q, c}
}

// Renvoie les valeurs de toutes les instances
func (p *pdhCounter) values() []float64 {
	if p == nil || call(pPdhCollectQueryData, p.q) != 0 {
		return nil
	}
	var size, count uint32
	call(pPdhGetFormattedCounterArray, p.c, 0x200|0x8000, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
	if size == 0 {
		return nil
	}
	buf := make([]byte, size)
	if call(pPdhGetFormattedCounterArray, p.c, 0x200|0x8000, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf[0]))) != 0 {
		return nil
	}
	out := make([]float64, 0, count)
	for i := uint32(0); i < count; i++ {
		item := unsafe.Pointer(&buf[uintptr(i)*24])
		status := *(*uint32)(unsafe.Add(item, 8))
		if status <= 1 {
			out = append(out, *(*float64)(unsafe.Add(item, 16)))
		}
	}
	return out
}

// ---- CPU -------------------------------------------------------------------

func fileTime(ft [2]uint32) uint64 { return uint64(ft[1])<<32 | uint64(ft[0]) }

var lastIdle, lastTotal uint64

func cpuUsage() (float64, bool) {
	var idle, kernel, user [2]uint32
	if call(pGetSystemTimes, uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user))) == 0 {
		return 0, false
	}
	i, t := fileTime(idle), fileTime(kernel)+fileTime(user)
	di, dt := i-lastIdle, t-lastTotal
	first := lastTotal == 0
	lastIdle, lastTotal = i, t
	if first || dt == 0 {
		return 0, false
	}
	return clamp(100*(1-float64(di)/float64(dt)), 0, 100), true
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ---- GPU : température via Windows (comme le Gestionnaire des tâches) --------

type d3dkmtAdapterInfo struct {
	HAdapter     uint32
	LuidLow      uint32
	LuidHigh     int32
	NumSources   uint32
	PrecisePresent int32
}

type d3dkmtEnumAdapters2 struct {
	NumAdapters uint32
	_           uint32
	PAdapters   uintptr
}

type d3dkmtQueryAdapterInfo struct {
	HAdapter uint32
	Type     int32
	PData    uintptr
	DataSize uint32
	_        uint32
}

type d3dkmtAdapterPerfData struct {
	PhysicalAdapterIndex uint32
	MemoryFrequency      uint64
	MaxMemoryFrequency   uint64
	MaxMemoryFrequencyOC uint64
	MemoryBandwidth      uint64
	PCIEBandwidth        uint64
	FanRPM               uint32
	Power                uint32
	Temperature          uint32
	PowerStateOverride   uint8
}

var gpuAdapters []uint32

func initGPUAdapters() {
	if pD3DKMTEnumAdapters2.Find() != nil {
		return
	}
	var e d3dkmtEnumAdapters2
	if call(pD3DKMTEnumAdapters2, uintptr(unsafe.Pointer(&e))) != 0 || e.NumAdapters == 0 {
		return
	}
	list := make([]d3dkmtAdapterInfo, e.NumAdapters)
	e.PAdapters = uintptr(unsafe.Pointer(&list[0]))
	if call(pD3DKMTEnumAdapters2, uintptr(unsafe.Pointer(&e))) != 0 {
		return
	}
	for _, a := range list[:e.NumAdapters] {
		gpuAdapters = append(gpuAdapters, a.HAdapter)
	}
}

func gpuTempWindows() (float64, bool) {
	best := 0.0
	for _, h := range gpuAdapters {
		var pd d3dkmtAdapterPerfData
		q := d3dkmtQueryAdapterInfo{HAdapter: h, Type: 62, PData: uintptr(unsafe.Pointer(&pd)), DataSize: uint32(unsafe.Sizeof(pd))}
		if call(pD3DKMTQueryAdapterInfo, uintptr(unsafe.Pointer(&q))) != 0 {
			continue
		}
		if t := float64(pd.Temperature) / 10; t > best && t < 130 {
			best = t
		}
	}
	return best, best > 5
}

// ---- GPU NVIDIA (NVML) ---------------------------------------------------------

var (
	nvmlDev                         uintptr
	nvmlOk                          bool
	pNvmlTemp, pNvmlUtil            *syscall.LazyProc
)

func initNVML() {
	paths := []string{
		filepath.Join(os.Getenv("WINDIR"), "System32", "nvml.dll"),
		filepath.Join(os.Getenv("ProgramFiles"), "NVIDIA Corporation", "NVSMI", "nvml.dll"),
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		d := syscall.NewLazyDLL(p)
		if d.Load() != nil {
			continue
		}
		initP, getH := d.NewProc("nvmlInit_v2"), d.NewProc("nvmlDeviceGetHandleByIndex_v2")
		if initP.Find() != nil || getH.Find() != nil || call(initP) != 0 {
			continue
		}
		if call(getH, 0, uintptr(unsafe.Pointer(&nvmlDev))) != 0 {
			continue
		}
		pNvmlTemp, pNvmlUtil = d.NewProc("nvmlDeviceGetTemperature"), d.NewProc("nvmlDeviceGetUtilizationRates")
		nvmlOk = pNvmlTemp.Find() == nil && pNvmlUtil.Find() == nil
		return
	}
}

func nvmlRead() (temp, use float64, tok, uok bool) {
	if !nvmlOk {
		return
	}
	var t uint32
	if call(pNvmlTemp, nvmlDev, 0, uintptr(unsafe.Pointer(&t))) == 0 && t > 0 {
		temp, tok = float64(t), true
	}
	var u [2]uint32
	if call(pNvmlUtil, nvmlDev, uintptr(unsafe.Pointer(&u))) == 0 {
		use, uok = float64(u[0]), true
	}
	return
}

// ---- Boucle de mesure ------------------------------------------------------------

func startSensors() {
	go func() {
		initGPUAdapters()
		initNVML()
		gpuEngine := pdhOpen(`\GPU Engine(*engtype_3D)\Utilization Percentage`)
		thermal := pdhOpen(`\Thermal Zone Information(*)\High Precision Temperature`)
		thermalDeci := true
		if thermal == nil {
			thermal = pdhOpen(`\Thermal Zone Information(*)\Temperature`)
			thermalDeci = false
		}
		cpuUsage()
		for {
			time.Sleep(time.Second)
			if atomic.LoadInt32(&sensorsWanted) == 0 {
				continue // rien n'affiche les capteurs : pause, 0 % CPU
			}
			var d SensorData
			d.CPUUse, d.CPUUseOk = cpuUsage()

			nt, nu, ntok, nuok := nvmlRead()
			if t, ok := gpuTempWindows(); ok {
				d.GPUTemp, d.GPUTempOk = t, true
			} else if ntok {
				d.GPUTemp, d.GPUTempOk = nt, true
			}
			if nuok {
				d.GPUUse, d.GPUUseOk = nu, true
			} else if vals := gpuEngine.values(); vals != nil {
				sum := 0.0
				for _, v := range vals {
					sum += v
				}
				d.GPUUse, d.GPUUseOk = clamp(sum, 0, 100), true
			}

			best := 0.0
			for _, v := range thermal.values() {
				if thermalDeci {
					v /= 10
				}
				if c := v - 273.15; c > best {
					best = c
				}
			}
			if best >= 20 && best <= 110 {
				d.CPUTemp, d.CPUTempOk = best, true
			}

			sensorsMu.Lock()
			sensorVal = d
			sensorsMu.Unlock()
		}
	}()
}
