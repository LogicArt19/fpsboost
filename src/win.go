package main

import (
	"syscall"
	"unsafe"
)

var (
	modUser32   = syscall.NewLazyDLL("user32.dll")
	modGdi32    = syscall.NewLazyDLL("gdi32.dll")
	modKernel32 = syscall.NewLazyDLL("kernel32.dll")
	modShell32  = syscall.NewLazyDLL("shell32.dll")
	modAdvapi32 = syscall.NewLazyDLL("advapi32.dll")
	modNtdll    = syscall.NewLazyDLL("ntdll.dll")
	modDwmapi   = syscall.NewLazyDLL("dwmapi.dll")
	modUxtheme  = syscall.NewLazyDLL("uxtheme.dll")
	modGdiplus  = syscall.NewLazyDLL("gdiplus.dll")
	modPowrprof = syscall.NewLazyDLL("powrprof.dll")

	// user32
	pRegisterClassExW         = modUser32.NewProc("RegisterClassExW")
	pCreateWindowExW          = modUser32.NewProc("CreateWindowExW")
	pDefWindowProcW           = modUser32.NewProc("DefWindowProcW")
	pGetMessageW              = modUser32.NewProc("GetMessageW")
	pTranslateMessage         = modUser32.NewProc("TranslateMessage")
	pDispatchMessageW         = modUser32.NewProc("DispatchMessageW")
	pIsDialogMessageW         = modUser32.NewProc("IsDialogMessageW")
	pPostQuitMessage          = modUser32.NewProc("PostQuitMessage")
	pPostMessageW             = modUser32.NewProc("PostMessageW")
	pSendMessageW             = modUser32.NewProc("SendMessageW")
	pShowWindow               = modUser32.NewProc("ShowWindow")
	pDestroyWindow            = modUser32.NewProc("DestroyWindow")
	pSetForegroundWindow      = modUser32.NewProc("SetForegroundWindow")
	pIsIconic                 = modUser32.NewProc("IsIconic")
	pIsWindowVisible          = modUser32.NewProc("IsWindowVisible")
	pBeginPaint               = modUser32.NewProc("BeginPaint")
	pEndPaint                 = modUser32.NewProc("EndPaint")
	pInvalidateRect           = modUser32.NewProc("InvalidateRect")
	pGetClientRect            = modUser32.NewProc("GetClientRect")
	pLoadCursorW              = modUser32.NewProc("LoadCursorW")
	pLoadImageW               = modUser32.NewProc("LoadImageW")
	pSetTimer                 = modUser32.NewProc("SetTimer")
	pAdjustWindowRectEx       = modUser32.NewProc("AdjustWindowRectEx")
	pGetSystemMetrics         = modUser32.NewProc("GetSystemMetrics")
	pMessageBoxW              = modUser32.NewProc("MessageBoxW")
	pCreatePopupMenu          = modUser32.NewProc("CreatePopupMenu")
	pAppendMenuW              = modUser32.NewProc("AppendMenuW")
	pTrackPopupMenu           = modUser32.NewProc("TrackPopupMenu")
	pDestroyMenu              = modUser32.NewProc("DestroyMenu")
	pGetCursorPos             = modUser32.NewProc("GetCursorPos")
	pEnumWindows              = modUser32.NewProc("EnumWindows")
	pGetWindowThreadProcessId = modUser32.NewProc("GetWindowThreadProcessId")
	pGetWindow                = modUser32.NewProc("GetWindow")
	pGetWindowTextLengthW     = modUser32.NewProc("GetWindowTextLengthW")
	pGetWindowTextW           = modUser32.NewProc("GetWindowTextW")
	pSetWindowTextW           = modUser32.NewProc("SetWindowTextW")
	pFindWindowW              = modUser32.NewProc("FindWindowW")
	pRegisterWindowMessageW   = modUser32.NewProc("RegisterWindowMessageW")
	pGetDC                    = modUser32.NewProc("GetDC")
	pReleaseDC                = modUser32.NewProc("ReleaseDC")
	pDrawTextW                = modUser32.NewProc("DrawTextW")
	pGetWindowRect            = modUser32.NewProc("GetWindowRect")
	pScreenToClient           = modUser32.NewProc("ScreenToClient")

	// gdi32
	pCreateFontW            = modGdi32.NewProc("CreateFontW")
	pSelectObject           = modGdi32.NewProc("SelectObject")
	pDeleteObject           = modGdi32.NewProc("DeleteObject")
	pSetTextColor           = modGdi32.NewProc("SetTextColor")
	pSetBkColor             = modGdi32.NewProc("SetBkColor")
	pSetBkMode              = modGdi32.NewProc("SetBkMode")
	pCreateSolidBrush       = modGdi32.NewProc("CreateSolidBrush")
	pCreateCompatibleDC     = modGdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmap = modGdi32.NewProc("CreateCompatibleBitmap")
	pBitBlt                 = modGdi32.NewProc("BitBlt")
	pDeleteDC               = modGdi32.NewProc("DeleteDC")
	pGetDeviceCaps          = modGdi32.NewProc("GetDeviceCaps")
	pGetTextExtentPoint32W  = modGdi32.NewProc("GetTextExtentPoint32W")

	// kernel32
	pGetModuleHandleW            = modKernel32.NewProc("GetModuleHandleW")
	pCreateMutexW                = modKernel32.NewProc("CreateMutexW")
	pQueryFullProcessImageNameW  = modKernel32.NewProc("QueryFullProcessImageNameW")
	pGlobalMemoryStatusEx        = modKernel32.NewProc("GlobalMemoryStatusEx")
	pGetPriorityClass            = modKernel32.NewProc("GetPriorityClass")
	pSetPriorityClass            = modKernel32.NewProc("SetPriorityClass")

	// shell32
	pShell_NotifyIconW = modShell32.NewProc("Shell_NotifyIconW")
	pShellExecuteW     = modShell32.NewProc("ShellExecuteW")

	// advapi32
	pRegOpenKeyExW         = modAdvapi32.NewProc("RegOpenKeyExW")
	pRegCreateKeyExW       = modAdvapi32.NewProc("RegCreateKeyExW")
	pRegQueryValueExW      = modAdvapi32.NewProc("RegQueryValueExW")
	pRegSetValueExW        = modAdvapi32.NewProc("RegSetValueExW")
	pRegDeleteValueW       = modAdvapi32.NewProc("RegDeleteValueW")
	pRegCloseKey           = modAdvapi32.NewProc("RegCloseKey")
	pOpenProcessToken      = modAdvapi32.NewProc("OpenProcessToken")
	pLookupPrivilegeValueW = modAdvapi32.NewProc("LookupPrivilegeValueW")
	pAdjustTokenPrivileges = modAdvapi32.NewProc("AdjustTokenPrivileges")

	// ntdll
	pNtSetTimerResolution     = modNtdll.NewProc("NtSetTimerResolution")
	pNtQueryTimerResolution   = modNtdll.NewProc("NtQueryTimerResolution")
	pNtSetSystemInformation   = modNtdll.NewProc("NtSetSystemInformation")
	pNtQuerySystemInformation = modNtdll.NewProc("NtQuerySystemInformation")

	// divers
	pDwmSetWindowAttribute = modDwmapi.NewProc("DwmSetWindowAttribute")
	pSetWindowTheme        = modUxtheme.NewProc("SetWindowTheme")

	// gdiplus
	pGdiplusStartup         = modGdiplus.NewProc("GdiplusStartup")
	pGdipCreateFromHDC      = modGdiplus.NewProc("GdipCreateFromHDC")
	pGdipDeleteGraphics     = modGdiplus.NewProc("GdipDeleteGraphics")
	pGdipSetSmoothingMode   = modGdiplus.NewProc("GdipSetSmoothingMode")
	pGdipSetPixelOffsetMode = modGdiplus.NewProc("GdipSetPixelOffsetMode")
	pGdipCreateSolidFill    = modGdiplus.NewProc("GdipCreateSolidFill")
	pGdipCreateLineBrushI   = modGdiplus.NewProc("GdipCreateLineBrushI")
	pGdipDeleteBrush        = modGdiplus.NewProc("GdipDeleteBrush")
	pGdipFillRectangleI     = modGdiplus.NewProc("GdipFillRectangleI")
	pGdipFillEllipseI       = modGdiplus.NewProc("GdipFillEllipseI")

	// powrprof
	pPowerGetActualOverlayScheme = modPowrprof.NewProc("PowerGetActualOverlayScheme")
	pPowerSetActiveOverlayScheme = modPowrprof.NewProc("PowerSetActiveOverlayScheme")
)

func call(p *syscall.LazyProc, a ...uintptr) uintptr {
	r, _, _ := p.Call(a...)
	return r
}

func w(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

// ---- Structures ----------------------------------------------------------

type POINT struct{ X, Y int32 }
type RECT struct{ Left, Top, Right, Bottom int32 }
type SIZE struct{ CX, CY int32 }

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type MSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
	Private uint32
}

type PAINTSTRUCT struct {
	Hdc         uintptr
	FErase      int32
	RcPaint     RECT
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}

type DRAWITEMSTRUCT struct {
	CtlType    uint32
	CtlID      uint32
	ItemID     uint32
	ItemAction uint32
	ItemState  uint32
	HwndItem   uintptr
	HDC        uintptr
	RcItem     RECT
	ItemData   uintptr
}

type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type NOTIFYICONDATAW struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         GUID
	HBalloonIcon     uintptr
}

type MEMORYSTATUSEX struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

type gdipStartupInput struct {
	GdiplusVersion           uint32
	DebugEventCallback       uintptr
	SuppressBackgroundThread int32
	SuppressExternalCodecs   int32
}

// ---- Constantes ----------------------------------------------------------

const (
	WS_OVERLAPPED   = 0x00000000
	WS_CAPTION      = 0x00C00000
	WS_SYSMENU      = 0x00080000
	WS_MINIMIZEBOX  = 0x00020000
	WS_CLIPCHILDREN = 0x02000000
	WS_CHILD        = 0x40000000
	WS_VISIBLE      = 0x10000000
	WS_VSCROLL      = 0x00200000
	WS_TABSTOP      = 0x00010000

	BS_OWNERDRAW         = 0x0B
	ES_AUTOHSCROLL       = 0x80
	LBS_NOTIFY           = 0x1
	LBS_NOINTEGRALHEIGHT = 0x100
	LBS_NOSEL            = 0x4000

	WM_DESTROY         = 0x0002
	WM_PAINT           = 0x000F
	WM_CLOSE           = 0x0010
	WM_QUERYENDSESSION = 0x0011
	WM_ERASEBKGND      = 0x0014
	WM_ENDSESSION      = 0x0016
	WM_SETFONT         = 0x0030
	WM_SETICON         = 0x0080
	WM_DRAWITEM        = 0x002B
	WM_COMMAND         = 0x0111
	WM_TIMER           = 0x0113
	WM_CTLCOLOREDIT    = 0x0133
	WM_CTLCOLORLISTBOX = 0x0134
	WM_CTLCOLORSTATIC  = 0x0138
	WM_LBUTTONUP       = 0x0202
	WM_LBUTTONDBLCLK   = 0x0203
	WM_RBUTTONUP       = 0x0205
	WM_APP             = 0x8000

	LB_ADDSTRING     = 0x0180
	LB_DELETESTRING  = 0x0182
	LB_RESETCONTENT  = 0x0184
	LB_GETCURSEL     = 0x0188
	LB_GETCOUNT      = 0x018B
	LB_SETTOPINDEX   = 0x0197
	LB_SETITEMHEIGHT = 0x01A0
	EM_SETMARGINS    = 0x00D3
	EM_SETCUEBANNER  = 0x1501

	LBN_DBLCLK   = 2
	BN_CLICKED   = 0
	ODS_SELECTED = 0x1

	SW_HIDE    = 0
	SW_SHOW    = 5
	SW_RESTORE = 9

	MF_STRING    = 0x0
	MF_SEPARATOR = 0x800
	MF_GRAYED    = 0x1
	TPM_RETURNCMD   = 0x100
	TPM_RIGHTBUTTON = 0x2

	MB_YESNO        = 0x4
	MB_OK           = 0x0
	MB_ICONQUESTION = 0x20
	MB_ICONINFO     = 0x40
	IDYES           = 6

	NIM_ADD     = 0
	NIM_MODIFY  = 1
	NIM_DELETE  = 2
	NIF_MESSAGE = 0x1
	NIF_ICON    = 0x2
	NIF_TIP     = 0x4
	NIF_INFO    = 0x10
	NIIF_INFO   = 0x1

	DT_LEFT         = 0x0
	DT_CENTER       = 0x1
	DT_RIGHT        = 0x2
	DT_VCENTER      = 0x4
	DT_WORDBREAK    = 0x10
	DT_SINGLELINE   = 0x20
	DT_NOPREFIX     = 0x800
	DT_END_ELLIPSIS = 0x8000

	SRCCOPY = 0x00CC0020
)

// Couleurs ---------------------------------------------------------------

func rgb(r, g, b uint32) uint32 { return r | g<<8 | b<<16 }
func argb(r, g, b uint32) uint32 { return 0xFF000000 | r<<16 | g<<8 | b }

type color struct{ r, g, b uint32 }

func (c color) ref() uint32  { return rgb(c.r, c.g, c.b) }
func (c color) argb() uint32 { return argb(c.r, c.g, c.b) }

var (
	cBg       = color{0x0D, 0x0F, 0x15}
	cCard     = color{0x16, 0x19, 0x22}
	cBorder   = color{0x25, 0x29, 0x35}
	cInput    = color{0x0F, 0x11, 0x18}
	cText     = color{0xEA, 0xEC, 0xF2}
	cMuted    = color{0x86, 0x8C, 0x9E}
	cAccent   = color{0x8B, 0x5C, 0xF6}
	cAccent2  = color{0x22, 0xD3, 0xEE}
	cGreen    = color{0x22, 0xC5, 0x5E}
	cBtn      = color{0x24, 0x28, 0x34}
	cBtnPress = color{0x1A, 0x1D, 0x26}
	cOff      = color{0x3A, 0x3F, 0x4E}
	cWhite    = color{0xFF, 0xFF, 0xFF}
)

// ---- GDI+ ----------------------------------------------------------------

var gdipToken uintptr

func gdipInit() {
	in := gdipStartupInput{GdiplusVersion: 1}
	call(pGdiplusStartup, uintptr(unsafe.Pointer(&gdipToken)), uintptr(unsafe.Pointer(&in)), 0)
}

type gfx struct{ g uintptr }

func newGfx(hdc uintptr) gfx {
	var g uintptr
	call(pGdipCreateFromHDC, hdc, uintptr(unsafe.Pointer(&g)))
	call(pGdipSetSmoothingMode, g, 4)   // AntiAlias
	call(pGdipSetPixelOffsetMode, g, 4) // Half
	return gfx{g}
}
func (x gfx) close() { call(pGdipDeleteGraphics, x.g) }

func solidBrush(c color) uintptr {
	var b uintptr
	call(pGdipCreateSolidFill, uintptr(c.argb()), uintptr(unsafe.Pointer(&b)))
	return b
}
func gradientBrush(x1, y1, x2, y2 int32, c1, c2 color) uintptr {
	p1 := POINT{x1, y1}
	p2 := POINT{x2, y2}
	var b uintptr
	call(pGdipCreateLineBrushI, uintptr(unsafe.Pointer(&p1)), uintptr(unsafe.Pointer(&p2)),
		uintptr(c1.argb()), uintptr(c2.argb()), 0, uintptr(unsafe.Pointer(&b)))
	return b
}
func freeBrush(b uintptr) { call(pGdipDeleteBrush, b) }

func (x gfx) rect(b uintptr, l, t, wd, ht int32) {
	if wd <= 0 || ht <= 0 {
		return
	}
	call(pGdipFillRectangleI, x.g, b, uintptr(l), uintptr(t), uintptr(wd), uintptr(ht))
}
func (x gfx) ellipse(b uintptr, l, t, wd, ht int32) {
	call(pGdipFillEllipseI, x.g, b, uintptr(l), uintptr(t), uintptr(wd), uintptr(ht))
}
func (x gfx) round(b uintptr, l, t, wd, ht, r int32) {
	if r*2 > ht {
		r = ht / 2
	}
	if r*2 > wd {
		r = wd / 2
	}
	d := r * 2
	x.ellipse(b, l, t, d, d)
	x.ellipse(b, l+wd-d, t, d, d)
	x.ellipse(b, l, t+ht-d, d, d)
	x.ellipse(b, l+wd-d, t+ht-d, d, d)
	x.rect(b, l+r, t, wd-d, ht)
	x.rect(b, l, t+r, wd, ht-d)
}
func (x gfx) roundColor(c color, l, t, wd, ht, r int32) {
	b := solidBrush(c)
	x.round(b, l, t, wd, ht, r)
	freeBrush(b)
}
func (x gfx) rectColor(c color, l, t, wd, ht int32) {
	b := solidBrush(c)
	x.rect(b, l, t, wd, ht)
	freeBrush(b)
}
func (x gfx) ellipseColor(c color, l, t, wd, ht int32) {
	b := solidBrush(c)
	x.ellipse(b, l, t, wd, ht)
	freeBrush(b)
}

// ---- Texte GDI -----------------------------------------------------------

func drawText(hdc uintptr, s string, font uintptr, c color, r RECT, flags uint32) {
	call(pSelectObject, hdc, font)
	call(pSetTextColor, hdc, uintptr(c.ref()))
	call(pSetBkMode, hdc, 1)
	u, _ := syscall.UTF16FromString(s)
	call(pDrawTextW, hdc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&r)), uintptr(flags|DT_NOPREFIX))
}

func textWidth(hdc uintptr, s string, font uintptr) int32 {
	call(pSelectObject, hdc, font)
	u, _ := syscall.UTF16FromString(s)
	var sz SIZE
	call(pGetTextExtentPoint32W, hdc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&sz)))
	return sz.CX
}
