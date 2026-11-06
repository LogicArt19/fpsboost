package main

// Compteur de FPS en direct.
// Écoute (en lecture seule) les événements "Present" de DirectX via ETW,
// la même méthode que PresentMon / CapFrameX. Rien n'est injecté dans le jeu.

import (
	"encoding/binary"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	pStartTraceW    = modAdvapi32.NewProc("StartTraceW")
	pControlTraceW  = modAdvapi32.NewProc("ControlTraceW")
	pEnableTraceEx2 = modAdvapi32.NewProc("EnableTraceEx2")
	pOpenTraceW     = modAdvapi32.NewProc("OpenTraceW")
	pProcessTrace   = modAdvapi32.NewProc("ProcessTrace")
	pCloseTrace     = modAdvapi32.NewProc("CloseTrace")
)

const traceName = "FPSBoost-FPS"

var (
	dxgiGUID = GUID{0xCA11C036, 0x0102, 0x4A2D, [8]byte{0xA6, 0xAD, 0xF0, 0x3C, 0xFE, 0xD5, 0xD3, 0xC9}}
	d3d9GUID = GUID{0x783ACA0A, 0x790E, 0x4D7F, [8]byte{0x84, 0x51, 0xAA, 0x85, 0x05, 0x11, 0xC6, 0xB9}}
)

type eventTraceLogfile struct {
	LogFileName         *uint16
	LoggerName          *uint16
	CurrentTime         int64
	BuffersRead         uint32
	ProcessTraceMode    uint32
	CurrentEvent        [88]byte
	LogfileHeader       [280]byte
	BufferCallback      uintptr
	BufferSize          uint32
	Filled              uint32
	EventsLost          uint32
	EventRecordCallback uintptr
	IsKernelTrace       uint32
	Context             uintptr
}

var (
	fpsPid      uint32 // atomique
	fpsCount    uint64 // atomique
	fpsRunning  int32  // atomique
	fpsSession  uint64
	fpsTrace    uint64
	fpsLogfile  eventTraceLogfile
	fpsValue    int
	fpsAvg      int
	fpsLast     time.Time
	fpsLastCnt  uint64
	fpsSum      int
	fpsSamples  int
	fpsStarted  time.Time
	fpsLow1     int

	frameMu     sync.Mutex
	tsRing      [16384]int64 // horodatage exact de chaque image (horloge QPC)
	tsPos       int
	tsN         int
	frameHist   [2000]uint32 // durées des images, tranches de 0,1 ms
	histTotal   uint64
	histEnd     int64
	qpcFreq     float64
)

var pQueryPerformanceFrequency = modKernel32.NewProc("QueryPerformanceFrequency")

func initQPC() {
	var f int64
	call(pQueryPerformanceFrequency, uintptr(unsafe.Pointer(&f)))
	qpcFreq = float64(f)
}

var pQueryPerformanceCounter = modKernel32.NewProc("QueryPerformanceCounter")

func qpcNow() int64 {
	var v int64
	call(pQueryPerformanceCounter, uintptr(unsafe.Pointer(&v)))
	return v
}

// 1 % low : moyenne des 1 % d'images les plus lentes, convertie en FPS
func low1Of(ft []float32) int {
	if len(ft) < 100 {
		return 0
	}
	sort.Slice(ft, func(i, j int) bool { return ft[i] > ft[j] })
	k := len(ft) / 100
	var sum float32
	for _, v := range ft[:k] {
		sum += v
	}
	if sum <= 0 {
		return 0
	}
	return int(1000/(sum/float32(k)) + 0.5)
}

func sessionLow1() int {
	frameMu.Lock()
	defer frameMu.Unlock()
	if histTotal < 200 {
		return 0
	}
	need := histTotal / 100
	var cnt uint64
	var sum float64
	for b := len(frameHist) - 1; b >= 0 && cnt < need; b-- {
		c := uint64(frameHist[b])
		if cnt+c > need {
			c = need - cnt
		}
		cnt += c
		sum += float64(c) * (float64(b) + 0.5) / 10
	}
	if sum <= 0 {
		return 0
	}
	return int(1000/(sum/float64(cnt)) + 0.5)
}

var etwCb = syscall.NewCallback(func(rec uintptr) uintptr {
	pid := *(*uint32)(unsafe.Pointer(rec + 12))
	if pid == 0 || pid != atomic.LoadUint32(&fpsPid) {
		return 0
	}
	d1 := *(*uint32)(unsafe.Pointer(rec + 24))
	id := *(*uint16)(unsafe.Pointer(rec + 40))
	if (d1 == dxgiGUID.Data1 && (id == 42 || id == 55)) || (d1 == d3d9GUID.Data1 && id == 1) {
		atomic.AddUint64(&fpsCount, 1)
		ts := *(*int64)(unsafe.Pointer(rec + 16))
		frameMu.Lock()
		tsRing[tsPos] = ts
		tsPos = (tsPos + 1) % len(tsRing)
		if tsN < len(tsRing) {
			tsN++
		}
		frameMu.Unlock()
	}
	return 0
})

func traceProps() []uint64 {
	const nameOff = 120
	words := make([]uint64, (nameOff+1024)/8)
	b := unsafe.Slice((*byte)(unsafe.Pointer(&words[0])), len(words)*8)
	le := binary.LittleEndian
	le.PutUint32(b[0:], uint32(len(b))) // Wnode.BufferSize
	le.PutUint32(b[40:], 1)             // ClientContext = QPC
	le.PutUint32(b[44:], 0x00020000)    // WNODE_FLAG_TRACED_GUID
	le.PutUint32(b[48:], 64)            // BufferSize (Ko)
	le.PutUint32(b[64:], 0x100)         // EVENT_TRACE_REAL_TIME_MODE
	le.PutUint32(b[68:], 1)             // FlushTimer = 1 s
	le.PutUint32(b[116:], nameOff)      // LoggerNameOffset
	return words
}

func stopTraceSession() {
	p := traceProps()
	call(pControlTraceW, 0, uintptr(unsafe.Pointer(w(traceName))), uintptr(unsafe.Pointer(&p[0])), 1)
}

func fpsReset(pid uint32) {
	atomic.StoreUint32(&fpsPid, pid)
	atomic.StoreUint64(&fpsCount, 0)
	fpsValue, fpsAvg, fpsSum, fpsSamples = 0, 0, 0, 0
	fpsLast, fpsLastCnt = time.Time{}, 0
	fpsStarted = time.Now()
	frameMu.Lock()
	tsN, tsPos, histTotal, fpsLow1, fpsFluid, fpsStutterMin = 0, 0, 0, 0, 0, 0
	histEnd = qpcNow()
	frameHist = [2000]uint32{}
	frameMu.Unlock()
}

func fpsStart(pid uint32) {
	fpsReset(pid)
	if atomic.LoadInt32(&fpsRunning) == 1 {
		return
	}
	var h uint64
	p := traceProps()
	r := call(pStartTraceW, uintptr(unsafe.Pointer(&h)), uintptr(unsafe.Pointer(w(traceName))), uintptr(unsafe.Pointer(&p[0])))
	if r == 183 { // session restée ouverte : on la relance
		stopTraceSession()
		p = traceProps()
		r = call(pStartTraceW, uintptr(unsafe.Pointer(&h)), uintptr(unsafe.Pointer(w(traceName))), uintptr(unsafe.Pointer(&p[0])))
	}
	if r != 0 {
		logFile("Compteur FPS indisponible (StartTrace " + itoa(int(r)) + ")")
		return
	}
	fpsSession = h
	for _, g := range []*GUID{&dxgiGUID, &d3d9GUID} {
		call(pEnableTraceEx2, uintptr(h), uintptr(unsafe.Pointer(g)), 1, 4, ^uintptr(0), 0, 0, 0)
	}
	fpsLogfile = eventTraceLogfile{
		LoggerName:          w(traceName),
		ProcessTraceMode:    0x100 | 0x10000000, // REAL_TIME | EVENT_RECORD
		EventRecordCallback: etwCb,
	}
	th := call(pOpenTraceW, uintptr(unsafe.Pointer(&fpsLogfile)))
	if th == ^uintptr(0) {
		stopTraceSession()
		fpsSession = 0
		return
	}
	fpsTrace = uint64(th)
	atomic.StoreInt32(&fpsRunning, 1)
	go func(t uint64) {
		runtime.LockOSThread()
		call(pProcessTrace, uintptr(unsafe.Pointer(&t)), 1, 0, 0)
		atomic.StoreInt32(&fpsRunning, 0)
	}(fpsTrace)
}

func fpsStop() {
	atomic.StoreUint32(&fpsPid, 0)
	if fpsSession != 0 {
		stopTraceSession()
		fpsSession = 0
	}
	if fpsTrace != 0 {
		call(pCloseTrace, uintptr(fpsTrace))
		fpsTrace = 0
	}
	fpsValue, fpsAvg = 0, 0
}

// Appelé chaque seconde depuis l'interface.
// Windows livre les images par paquets (environ 1 fois par seconde) : on ne compte donc pas
// les images reçues, mais celles réellement affichées dans une fenêtre d'1 seconde,
// d'après leur horodatage exact, avec 2 s de recul pour être sûr que tout est arrivé.
func fpsUpdate() {
	if atomic.LoadUint32(&fpsPid) == 0 || qpcFreq <= 0 {
		fpsValue = 0
		return
	}
	end := qpcNow() - int64(2*qpcFreq)
	start := end - int64(qpcFreq)
	lowFrom := end - int64(10*qpcFreq)

	frameMu.Lock()
	ts := make([]int64, tsN)
	copy(ts, tsRing[:tsN])
	frameMu.Unlock()
	sort.Slice(ts, func(i, j int) bool { return ts[i] < ts[j] })

	count := 0
	var recent []float32
	var newFT []float32
	frameMu.Lock()
	for i, t := range ts {
		if t > end {
			break
		}
		if t > start {
			count++
		}
		if i == 0 {
			continue
		}
		ms := float32(float64(t-ts[i-1]) * 1000 / qpcFreq)
		if ms <= 0 || ms >= 1000 {
			continue
		}
		if t > lowFrom {
			recent = append(recent, ms)
		}
		if t > histEnd {
			newFT = append(newFT, ms)
			b := int(ms * 10)
			if b >= len(frameHist) {
				b = len(frameHist) - 1
			}
			frameHist[b]++
			histTotal++
		}
	}
	if end > histEnd {
		histEnd = end
	}
	frameMu.Unlock()

	fpsValue = count
	// Fluidité : saccades = images 2,5 fois plus longues que la normale
	if len(recent) >= 100 {
		sorted := append([]float32(nil), recent...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		med := sorted[len(sorted)/2]
		var sum float32
		st := 0
		for _, v := range sorted {
			sum += v
			if v > med*2.5 && v > med+8 {
				st++
			}
		}
		ns := 0
		for _, v := range newFT {
			if v > med*2.5 && v > med+8 {
				sessStutters++
				ns++
			}
		}
		lastSecStut = ns
		span := float64(sum) / 60000
		if span > 0 {
			fpsStutterMin = int(float64(st)/span + 0.5)
		}
		avg := 1000 / (sum / float32(len(sorted)))
		if l := low1Of(recent); l > 0 && avg > 0 {
			fpsFluid = clampInt(int(100*float32(l)/avg+0.5), 0, 100)
		}
	} else {
		fpsFluid, fpsStutterMin, lastSecStut = 0, 0, 0
	}
	fpsLow1 = low1Of(recent)
	if fpsValue > 0 {
		fpsSum += fpsValue
		fpsSamples++
		fpsAvg = fpsSum / fpsSamples
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
