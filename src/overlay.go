package main

// Mini overlay en jeu : petite barre transparente, toujours au premier plan,
// les clics passent au travers. Touche Inser pour l'afficher / la masquer.

import (
	"fmt"
	"math"
	"syscall"
	"unsafe"
)

var (
	pUpdateLayeredWindow = modUser32.NewProc("UpdateLayeredWindow")
	pSetWindowPos        = modUser32.NewProc("SetWindowPos")
	pGetForegroundWindow = modUser32.NewProc("GetForegroundWindow")
	pMonitorFromWindow   = modUser32.NewProc("MonitorFromWindow")
	pGetMonitorInfoW     = modUser32.NewProc("GetMonitorInfoW")
	pRegisterHotKey      = modUser32.NewProc("RegisterHotKey")
	pCreateDIBSection    = modGdi32.NewProc("CreateDIBSection")

	pGdipCreateBitmapFromScan0     = modGdiplus.NewProc("GdipCreateBitmapFromScan0")
	pGdipGetImageGraphicsContext   = modGdiplus.NewProc("GdipGetImageGraphicsContext")
	pGdipDisposeImage              = modGdiplus.NewProc("GdipDisposeImage")
	pGdipGraphicsClear             = modGdiplus.NewProc("GdipGraphicsClear")
	pGdipCreateFontFamilyFromName  = modGdiplus.NewProc("GdipCreateFontFamilyFromName")
	pGdipCreateFont                = modGdiplus.NewProc("GdipCreateFont")
	pGdipStringFormatGetGenericTypographic = modGdiplus.NewProc("GdipStringFormatGetGenericTypographic")
	pGdipDrawString                = modGdiplus.NewProc("GdipDrawString")
	pGdipMeasureString             = modGdiplus.NewProc("GdipMeasureString")
	pGdipSetTextRenderingHint      = modGdiplus.NewProc("GdipSetTextRenderingHint")
)

const (
	WM_HOTKEY   = 0x0312
	hotkeyID    = 1
	VK_INSERT   = 0x2D
	WS_POPUP    = 0x80000000
	WS_EX_TOPMOST     = 0x00000008
	WS_EX_TRANSPARENT = 0x00000020
	WS_EX_TOOLWINDOW  = 0x00000080
	WS_EX_LAYERED     = 0x00080000
	WS_EX_NOACTIVATE  = 0x08000000
)

type rectF struct{ X, Y, W, H float32 }

type monitorInfo struct {
	CbSize    uint32
	RcMonitor RECT
	RcWork    RECT
	DwFlags   uint32
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

var (
	hwndOverlay     uintptr
	overlayVisible  bool
	overlayAutoOpen bool
	ovFontValue     uintptr
	ovFontLabel     uintptr
	ovFormat        uintptr
	overlayCb       = syscall.NewCallback(func(h, m, wp, lp uintptr) uintptr {
		if m == 0x84 { // WM_NCHITTEST : les clics passent au travers
			return ^uintptr(0)
		}
		return call(pDefWindowProcW, h, m, wp, lp)
	})
)

func f32(v float32) uintptr { return uintptr(math.Float32bits(v)) }

func createOverlay() {
	wc := WNDCLASSEXW{LpfnWndProc: overlayCb, HInstance: hInst, LpszClassName: w("FPSBoostOverlay")}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	call(pRegisterClassExW, uintptr(unsafe.Pointer(&wc)))
	hwndOverlay = call(pCreateWindowExW, WS_EX_TOPMOST|WS_EX_LAYERED|WS_EX_TRANSPARENT|WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE,
		uintptr(unsafe.Pointer(w("FPSBoostOverlay"))), uintptr(unsafe.Pointer(w("FPS Boost Overlay"))),
		WS_POPUP, 0, 0, 10, 10, 0, 0, hInst, 0)

	var fam uintptr
	call(pGdipCreateFontFamilyFromName, uintptr(unsafe.Pointer(w("Segoe UI"))), 0, uintptr(unsafe.Pointer(&fam)))
	call(pGdipCreateFont, fam, f32(float32(s(14))), 1, 2, uintptr(unsafe.Pointer(&ovFontValue)))
	call(pGdipCreateFont, fam, f32(float32(s(11))), 1, 2, uintptr(unsafe.Pointer(&ovFontLabel)))
	call(pGdipStringFormatGetGenericTypographic, uintptr(unsafe.Pointer(&ovFormat)))

	if call(pRegisterHotKey, hwndMain, hotkeyID, 0x4000, VK_INSERT) == 0 {
		addLog("Touche Inser déjà utilisée par un autre programme : overlay au clavier indisponible")
	}
}

// ---- Contenu ---------------------------------------------------------------------

type ovSeg struct {
	value, label string
	valueFirst   bool
	c            color
}

func overlaySegments() []ovSeg {
	var segs []ovSeg
	fv, fc := "—", cMuted
	if sess != nil && fpsValue > 0 {
		fv, fc = itoa(fpsValue), cGreen
		if fpsValue < 60 {
			fc = color{0xF5, 0x9E, 0x0B}
		}
	}
	segs = append(segs, ovSeg{fv, "FPS", true, fc})

	sd := sensors()
	cpu := ""
	if sd.CPUUseOk {
		cpu = fmt.Sprintf("%.0f%%", sd.CPUUse)
	}
	cpuC := cText
	if cfg.OverlayTemps && sd.CPUTempOk {
		cpu += fmt.Sprintf("  %.0f°", sd.CPUTemp)
		if sd.CPUTemp >= 85 {
			cpuC = color{0xF5, 0x9E, 0x0B}
		}
	}
	if cpu != "" {
		segs = append(segs, ovSeg{cpu, "CPU", false, cpuC})
	}
	gpu := ""
	if sd.GPUUseOk {
		gpu = fmt.Sprintf("%.0f%%", sd.GPUUse)
	}
	gpuC := cText
	if cfg.OverlayTemps && sd.GPUTempOk {
		gpu += fmt.Sprintf("  %.0f°", sd.GPUTemp)
		if sd.GPUTemp >= 83 {
			gpuC = color{0xF5, 0x9E, 0x0B}
		}
	}
	if gpu != "" {
		segs = append(segs, ovSeg{gpu, "GPU", false, gpuC})
	}
	if cfg.OverlayRAM {
		segs = append(segs, ovSeg{fmt.Sprintf("%d%%", memStatus().MemoryLoad), "RAM", false, cText})
	}
	return segs
}

func measure(g uintptr, text string, font uintptr) float32 {
	u, _ := syscall.UTF16FromString(text)
	layout := rectF{0, 0, 2000, 200}
	var box rectF
	call(pGdipMeasureString, g, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), font, uintptr(unsafe.Pointer(&layout)),
		ovFormat, uintptr(unsafe.Pointer(&box)), 0, 0)
	return box.W
}

func drawStr(g uintptr, text string, font uintptr, c color, x, y, h float32) {
	u, _ := syscall.UTF16FromString(text)
	var hh rectF
	layout := rectF{0, 0, 2000, 200}
	call(pGdipMeasureString, g, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), font, uintptr(unsafe.Pointer(&layout)),
		ovFormat, uintptr(unsafe.Pointer(&hh)), 0, 0)
	r := rectF{x, y + (h-hh.H)/2, 2000, hh.H + 2}
	b := solidBrush(c)
	call(pGdipDrawString, g, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), font, uintptr(unsafe.Pointer(&r)), ovFormat, b)
	freeBrush(b)
}

// Calcule (draw=false) ou dessine (draw=true) la barre ; renvoie sa largeur
func pill(g uintptr, x0, y0 int32, draw bool) (int32, int32) {
	segs := overlaySegments()
	pad, gap, inner := float32(s(12)), float32(s(13)), float32(s(5))
	H := s(30)
	x := pad + float32(s(14))
	type pos struct{ x, wv, wl float32 }
	var ps []pos
	for i, sg := range segs {
		if i > 0 {
			x += gap*2 + 1
		}
		wv, wl := measure(g, sg.value, ovFontValue), measure(g, sg.label, ovFontLabel)
		ps = append(ps, pos{x, wv, wl})
		x += wv + inner + wl
	}
	W := int32(x+pad) + 1
	if !draw {
		return W, H
	}
	gg := gfx{g}
	gb := gradientBrush(x0, y0, x0+W, y0, cAccent, cAccent2)
	gg.round(gb, x0, y0, W, H, H/2)
	freeBrush(gb)
	var b uintptr
	call(pGdipCreateSolidFill, uintptr(0xE60D0F15), uintptr(unsafe.Pointer(&b)))
	gg.round(b, x0+1, y0+1, W-2, H-2, H/2-1)
	freeBrush(b)
	dot := s(8)
	db := gradientBrush(x0+int32(pad), 0, x0+int32(pad)+dot, 0, cAccent, cAccent2)
	gg.ellipse(db, x0+int32(pad), y0+(H-dot)/2, dot, dot)
	freeBrush(db)
	fx, fy, fh := float32(x0), float32(y0), float32(H)
	for i, sg := range segs {
		p := ps[i]
		if i > 0 {
			sep := solidBrush(cBorder)
			gg.rect(sep, x0+int32(p.x-gap-1), y0+s(8), 1, H-s(16))
			freeBrush(sep)
		}
		if sg.valueFirst {
			drawStr(g, sg.value, ovFontValue, sg.c, fx+p.x, fy, fh)
			drawStr(g, sg.label, ovFontLabel, cMuted, fx+p.x+p.wv+inner, fy+1, fh)
		} else {
			drawStr(g, sg.label, ovFontLabel, cMuted, fx+p.x, fy+1, fh)
			drawStr(g, sg.value, ovFontValue, sg.c, fx+p.x+p.wl+inner, fy, fh)
		}
	}
	return W, H
}

// ---- Affichage -------------------------------------------------------------------

func renderOverlay() {
	if hwndOverlay == 0 {
		return
	}
	// Mesure
	sdc := call(pGetDC, 0)
	var mg uintptr
	call(pGdipCreateFromHDC, sdc, uintptr(unsafe.Pointer(&mg)))
	W, H := pill(mg, 0, 0, false)
	call(pGdipDeleteGraphics, mg)

	// Image transparente 32 bits
	bi := bitmapInfoHeader{Size: 40, Width: W, Height: -H, Planes: 1, BitCount: 32}
	var bits uintptr
	mem := call(pCreateCompatibleDC, sdc)
	dib := call(pCreateDIBSection, sdc, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if dib == 0 {
		call(pDeleteDC, mem)
		call(pReleaseDC, 0, sdc)
		return
	}
	old := call(pSelectObject, mem, dib)
	var bmp, g uintptr
	call(pGdipCreateBitmapFromScan0, uintptr(W), uintptr(H), uintptr(W*4), 0xE200B, bits, uintptr(unsafe.Pointer(&bmp)))
	call(pGdipGetImageGraphicsContext, bmp, uintptr(unsafe.Pointer(&g)))
	call(pGdipSetSmoothingMode, g, 4)
	call(pGdipSetPixelOffsetMode, g, 4)
	call(pGdipSetTextRenderingHint, g, 4)
	call(pGdipGraphicsClear, g, 0)
	pill(g, 0, 0, true)
	call(pGdipDeleteGraphics, g)
	call(pGdipDisposeImage, bmp)

	// Position : coin choisi de l'écran du jeu
	mon := call(pMonitorFromWindow, call(pGetForegroundWindow), 1)
	mi := monitorInfo{CbSize: 40}
	call(pGetMonitorInfoW, mon, uintptr(unsafe.Pointer(&mi)))
	r := mi.RcMonitor
	m := s(12)
	x, y := r.Left+m, r.Top+m
	switch cfg.OverlayPos {
	case 1:
		x = r.Right - W - m
	case 2:
		y = r.Bottom - H - m
	case 3:
		x, y = r.Right-W-m, r.Bottom-H-m
	}
	dst := POINT{x, y}
	size := SIZE{W, H}
	src := POINT{0, 0}
	blend := uint32(0x01FF0000) // AC_SRC_OVER, 255, AC_SRC_ALPHA
	call(pUpdateLayeredWindow, hwndOverlay, sdc, uintptr(unsafe.Pointer(&dst)), uintptr(unsafe.Pointer(&size)), mem,
		uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), 2)
	call(pSetWindowPos, hwndOverlay, ^uintptr(0), 0, 0, 0, 0, 0x1|0x2|0x10) // toujours au premier plan

	call(pSelectObject, mem, old)
	call(pDeleteObject, dib)
	call(pDeleteDC, mem)
	call(pReleaseDC, 0, sdc)
}

func showOverlay(on bool) {
	if hwndOverlay == 0 {
		return
	}
	overlayVisible = on
	if on {
		renderOverlay()
		call(pShowWindow, hwndOverlay, 4) // SW_SHOWNOACTIVATE
	} else {
		call(pShowWindow, hwndOverlay, SW_HIDE)
	}
	if hwndMain != 0 {
		refreshUI()
	}
}
