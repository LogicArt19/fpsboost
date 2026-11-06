package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	className = "FPSBoostMainWindow"

	idPower    = 100
	idToggle0  = 200
	idOptimize = 300
	idRam      = 301
	idRestore  = 302
	idHide     = 303
	idAdd      = 304
	idEdit     = 400
	idGames    = 401
	idLog      = 402

	idTrayOpen   = 500
	idTrayToggle = 501
	idTrayQuit   = 502
	idTrayUpdate = 503
	idPickBase   = 1000

	WM_TRAY    = WM_APP + 1
	WM_SHOWME  = WM_APP + 2
	WM_HANDOFF = WM_APP + 3
	WM_QUITAPP = WM_APP + 4

	clientW = 820
	clientH = 620
)

var (
	hInst, hwndMain                                                 uintptr
	dpi                                                             int32 = 96
	fTitle, fSub, fNormal, fBold, fSmall, fStat, fSection, fBtn, fPower, fBig uintptr
	brInput                                                         uintptr
	hEdit, hGames, hLog                                             uintptr
	buttons                                                         = map[int]uintptr{}
	iconBig, iconSmall                                              uintptr
	nid                                                             NOTIFYICONDATAW
	msgTaskbarCreated                                               uintptr
	startupOn                                                       bool
)

func s(v int32) int32 { return v * dpi / 96 }

type toggleDef struct {
	label string
	get   func() bool
	set   func(bool)
}

var toggles = []toggleDef{
	{"Alimentation performances maximales", func() bool { return cfg.PowerPlan }, func(v bool) { cfg.PowerPlan = v }},
	{"Timer système 0,5 ms (fluidité)", func() bool { return cfg.TimerRes }, func(v bool) { cfg.TimerRes = v }},
	{"Nettoyage automatique de la RAM", func() bool { return cfg.RamClean }, func(v bool) { cfg.RamClean = v }},
	{"Priorité Haute pour le jeu", func() bool { return cfg.GamePriority }, func(v bool) { cfg.GamePriority = v }},
	{"Applis d'arrière-plan en priorité basse", func() bool { return cfg.BgPriority }, func(v bool) { cfg.BgPriority = v }},
	{"Lancer au démarrage de Windows", func() bool { return startupOn }, func(v bool) {
		if v {
			if installStartup() {
				startupOn = true
				addLog("FPS Boost se lancera au démarrage de Windows")
			} else {
				addLog("Impossible d'ajouter FPS Boost au démarrage")
			}
		} else {
			removeStartup()
			startupOn = false
			addLog("Démarrage automatique retiré")
		}
	}},
	{"Activer le booster à l'ouverture", func() bool { return cfg.AutoActivate }, func(v bool) { cfg.AutoActivate = v }},
	{"Notifications", func() bool { return cfg.Notifications }, func(v bool) { cfg.Notifications = v }},
}

// ---- Création de la fenêtre ----------------------------------------------

func mkFont(px, weight int32) uintptr {
	h := int32(-s(px))
	return call(pCreateFontW, uintptr(h), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0,
		uintptr(unsafe.Pointer(w("Segoe UI"))))
}

func createMainWindow() {
	hInst = call(pGetModuleHandleW, 0)
	hdc := call(pGetDC, 0)
	if d := int32(call(pGetDeviceCaps, hdc, 88)); d > 0 {
		dpi = d
	}
	call(pReleaseDC, 0, hdc)

	iconBig = call(pLoadImageW, hInst, 1, 1, call(pGetSystemMetrics, 11), call(pGetSystemMetrics, 12), 0)
	iconSmall = call(pLoadImageW, hInst, 1, 1, call(pGetSystemMetrics, 49), call(pGetSystemMetrics, 50), 0)

	fTitle = mkFont(30, 800)
	fSub = mkFont(13, 400)
	fNormal = mkFont(14, 400)
	fBold = mkFont(15, 600)
	fSmall = mkFont(12, 400)
	fStat = mkFont(21, 700)
	fSection = mkFont(11, 700)
	fBtn = mkFont(13, 600)
	fPower = mkFont(15, 700)
	fBig = mkFont(26, 800)
	brInput = call(pCreateSolidBrush, uintptr(cInput.ref()))

	wc := WNDCLASSEXW{
		LpfnWndProc:   syscall.NewCallback(wndProc),
		HInstance:     hInst,
		HIcon:         iconBig,
		HCursor:       call(pLoadCursorW, 0, 32512),
		LpszClassName: w(className),
		HIconSm:       iconSmall,
	}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	call(pRegisterClassExW, uintptr(unsafe.Pointer(&wc)))

	style := uintptr(WS_OVERLAPPED | WS_CAPTION | WS_SYSMENU | WS_MINIMIZEBOX | WS_CLIPCHILDREN)
	r := RECT{0, 0, s(clientW), s(clientH)}
	call(pAdjustWindowRectEx, uintptr(unsafe.Pointer(&r)), style, 0, 0)
	ww, wh := r.Right-r.Left, r.Bottom-r.Top
	x := (int32(call(pGetSystemMetrics, 0)) - ww) / 2
	y := (int32(call(pGetSystemMetrics, 1)) - wh) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	hwndMain = call(pCreateWindowExW, 0, uintptr(unsafe.Pointer(w(className))), uintptr(unsafe.Pointer(w("FPS Boost"))),
		style, uintptr(x), uintptr(y), uintptr(ww), uintptr(wh), 0, 0, hInst, 0)

	one := int32(1)
	call(pDwmSetWindowAttribute, hwndMain, 20, uintptr(unsafe.Pointer(&one)), 4)
	call(pDwmSetWindowAttribute, hwndMain, 19, uintptr(unsafe.Pointer(&one)), 4)

	createControls()
	msgTaskbarCreated = call(pRegisterWindowMessageW, uintptr(unsafe.Pointer(w("TaskbarCreated"))))
	trayAdd()
}

func child(class string, style uintptr, id int, x, y, wd, ht int32) uintptr {
	return call(pCreateWindowExW, 0, uintptr(unsafe.Pointer(w(class))), uintptr(unsafe.Pointer(w(""))),
		WS_CHILD|WS_VISIBLE|style, uintptr(s(x)), uintptr(s(y)), uintptr(s(wd)), uintptr(s(ht)), hwndMain, uintptr(id), hInst, 0)
}

func button(id int, x, y, wd, ht int32) {
	buttons[id] = child("BUTTON", WS_TABSTOP|BS_OWNERDRAW, id, x, y, wd, ht)
}

func createControls() {
	button(idPower, 580, 22, 216, 46)
	for i := range toggles {
		button(idToggle0+i, 36, 262+int32(i)*34, 348, 30)
	}
	button(idOptimize, 436, 262, 168, 32)
	button(idRam, 612, 262, 168, 32)
	button(idRestore, 436, 300, 168, 32)
	button(idHide, 612, 300, 168, 32)
	button(idAdd, 696, 386, 84, 28)

	hEdit = child("EDIT", WS_TABSTOP|ES_AUTOHSCROLL, idEdit, 446, 390, 240, 20)
	call(pSendMessageW, hEdit, WM_SETFONT, fNormal, 1)
	call(pSendMessageW, hEdit, EM_SETCUEBANNER, 1, uintptr(unsafe.Pointer(w("Nom du jeu, ou vide = liste"))))

	hGames = child("LISTBOX", WS_VSCROLL|LBS_NOTIFY|LBS_NOINTEGRALHEIGHT, idGames, 442, 423, 332, 34)
	hLog = child("LISTBOX", WS_VSCROLL|LBS_NOSEL|LBS_NOINTEGRALHEIGHT, idLog, 442, 518, 332, 78)
	for _, h := range []uintptr{hGames, hLog} {
		call(pSendMessageW, h, WM_SETFONT, fSmall, 1)
		call(pSendMessageW, h, LB_SETITEMHEIGHT, 0, uintptr(s(17)))
		call(pSetWindowTheme, h, uintptr(unsafe.Pointer(w("DarkMode_Explorer"))), 0)
	}
	refreshGames()
}

// ---- Dessin ---------------------------------------------------------------

func card(g gfx, x, y, wd, ht int32) {
	g.roundColor(cBorder, s(x), s(y), s(wd), s(ht), s(12))
	g.roundColor(cCard, s(x)+1, s(y)+1, s(wd)-2, s(ht)-2, s(12)-1)
}

func box(g gfx, x, y, wd, ht int32) {
	g.roundColor(cBorder, s(x), s(y), s(wd), s(ht), s(7))
	g.roundColor(cInput, s(x)+1, s(y)+1, s(wd)-2, s(ht)-2, s(7)-1)
}

func rc(l, t, r, b int32) RECT { return RECT{s(l), s(t), s(r), s(b)} }

func paint(hwnd uintptr) {
	var ps PAINTSTRUCT
	hdc := call(pBeginPaint, hwnd, uintptr(unsafe.Pointer(&ps)))
	var cr RECT
	call(pGetClientRect, hwnd, uintptr(unsafe.Pointer(&cr)))
	W, H := cr.Right, cr.Bottom
	mem := call(pCreateCompatibleDC, hdc)
	bmp := call(pCreateCompatibleBitmap, hdc, uintptr(W), uintptr(H))
	old := call(pSelectObject, mem, bmp)

	g := newGfx(mem)
	g.rectColor(cBg, 0, 0, W, H)
	// Bande d'accent en haut
	gb := gradientBrush(0, 0, W, 0, cAccent, cAccent2)
	g.rect(gb, 0, 0, W, s(3))
	freeBrush(gb)

	// Carte de statut
	card(g, 24, 84, 772, 52)
	dot := cOff
	if boosterOn && sess != nil {
		dot = cGreen
	} else if boosterOn {
		dot = cAccent
	}
	if boosterOn {
		g.ellipseColor(color{dot.r / 3, dot.g / 3, dot.b / 3}, s(38), s(100), s(20), s(20))
	}
	g.ellipseColor(dot, s(42), s(104), s(12), s(12))

	// Statistiques
	for i := int32(0); i < 4; i++ {
		card(g, 24+i*196, 148, 184, 62)
	}
	// Sections
	card(g, 24, 226, 372, 322)
	card(g, 420, 226, 376, 112)
	card(g, 420, 350, 376, 118)
	card(g, 420, 480, 376, 128)
	box(g, 436, 386, 252, 28)
	box(g, 436, 420, 344, 40)
	box(g, 436, 514, 344, 86)
	g.close()

	// Titre
	tx := s(24)
	drawText(mem, "FPS", fTitle, cText, RECT{tx, s(14), s(560), s(56)}, DT_SINGLELINE)
	tx += textWidth(mem, "FPS ", fTitle)
	drawText(mem, "BOOST", fTitle, cAccent, RECT{tx, s(14), s(560), s(56)}, DT_SINGLELINE)
	drawText(mem, "Booster automatique pour tous vos jeux", fSub, cMuted, rc(26, 54, 560, 74), DT_SINGLELINE)

	// Statut
	var st, sub string
	switch {
	case boosterOn && sess != nil:
		st = "BOOST ACTIF  —  " + sess.Game
		sub = "depuis " + sess.Start.Format("15:04") + "  ·  " + fmtDur(time.Since(sess.Start))
	case boosterOn:
		st = "En attente d'un jeu…"
		sub = fmt.Sprintf("Lancez n'importe quel jeu  ·  %d jeux boostés", cfg.GamesBoosted)
	default:
		st = "Booster désactivé"
		sub = "Cliquez sur « Activer le booster »"
	}
	drawText(mem, st, fBold, cText, rc(66, 84, 420, 136), DT_SINGLELINE|DT_VCENTER|DT_END_ELLIPSIS)
	drawText(mem, sub, fSmall, cMuted, rc(400, 84, 780, 136), DT_SINGLELINE|DT_VCENTER|DT_RIGHT|DT_END_ELLIPSIS)

	// Valeurs des stats
	m := memStatus()
	cur, _ := timerInfo()
	power := "Normale"
	powerC := cText
	if sess != nil && (sess.PowerChanged || sess.OverlayChanged) {
		power, powerC = "Maximale", cGreen
	}
	timerC := cText
	if cur <= 10000 {
		timerC = cGreen
	}
	ramC := cText
	if m.MemoryLoad >= 85 {
		ramC = color{0xF5, 0x9E, 0x0B}
	}
	fpsTxt, fpsC, fpsSub := "—", cMuted, ""
	if sess != nil {
		switch {
		case fpsValue > 0:
			fpsTxt, fpsC = itoa(fpsValue), cGreen
			if fpsValue < 60 {
				fpsC = color{0xF5, 0x9E, 0x0B}
			}
			if fpsAvg > 0 {
				fpsSub = "moy. " + itoa(fpsAvg)
			}
		case time.Since(fpsStarted) < 15*time.Second:
			fpsTxt = "…"
		default:
			fpsSub = "non mesurable"
		}
	}
	stats := []struct {
		label, value, sub string
		c                 color
	}{
		{"FPS EN DIRECT", fpsTxt, fpsSub, fpsC},
		{fmt.Sprintf("RAM UTILISÉE · %d GO", (m.TotalPhys+(1<<29))>>30), fmt.Sprintf("%d %%", m.MemoryLoad), "", ramC},
		{"TIMER SYSTÈME", fmtMs(float64(cur)/10000) + " ms", "", timerC},
		{"ALIMENTATION", power, "", powerC},
	}
	for i, st := range stats {
		x := 24 + int32(i)*196
		drawText(mem, st.label, fSection, cMuted, rc(x+16, 160, x+178, 176), DT_SINGLELINE)
		f := fStat
		if i == 0 {
			f = fBig
		}
		drawText(mem, st.value, f, st.c, rc(x+16, 172, x+178, 206), DT_SINGLELINE|DT_VCENTER)
		if st.sub != "" {
			drawText(mem, st.sub, fSmall, cMuted, rc(x+70, 172, x+172, 206), DT_SINGLELINE|DT_VCENTER|DT_RIGHT)
		}
	}

	// Titres de sections
	drawText(mem, "PARAMÈTRES", fSection, cMuted, rc(40, 238, 380, 254), DT_SINGLELINE)
	drawText(mem, "ACTIONS", fSection, cMuted, rc(436, 238, 780, 254), DT_SINGLELINE)
	drawText(mem, "JEUX AJOUTÉS", fSection, cMuted, rc(436, 362, 780, 378), DT_SINGLELINE)
	drawText(mem, "double-clic pour retirer", fSmall, cMuted, rc(560, 361, 780, 378), DT_SINGLELINE|DT_RIGHT)
	drawText(mem, "JOURNAL", fSection, cMuted, rc(436, 492, 780, 508), DT_SINGLELINE)
	drawText(mem, "Détection automatique : Steam, Epic, Riot, Xbox, Ubisoft, EA, GOG, Rockstar, Battle.net et plus de 60 jeux populaires.\nFPS Boost " + appVersionStr + "  ·  vos réglages sont gardés à chaque mise à jour",
		fSmall, cMuted, rc(26, 562, 396, 612), DT_WORDBREAK)

	call(pBitBlt, hdc, 0, 0, uintptr(W), uintptr(H), mem, 0, 0, SRCCOPY)
	call(pSelectObject, mem, old)
	call(pDeleteObject, bmp)
	call(pDeleteDC, mem)
	call(pEndPaint, hwnd, uintptr(unsafe.Pointer(&ps)))
}

func buttonText(id int) string {
	switch id {
	case idOptimize:
		if backup.Applied {
			return "Windows optimisé ✓"
		}
		return "Optimiser Windows"
	case idRam:
		return "Libérer la RAM"
	case idRestore:
		return "Tout restaurer"
	case idHide:
		if pendingUpdate != nil {
			return "Mettre à jour ★"
		}
		return "Mises à jour"
	case idAdd:
		return "Ajouter"
	}
	return ""
}

func drawItem(d *DRAWITEMSTRUCT) {
	id := int(d.CtlID)
	W, H := d.RcItem.Right-d.RcItem.Left, d.RcItem.Bottom-d.RcItem.Top
	mem := call(pCreateCompatibleDC, d.HDC)
	bmp := call(pCreateCompatibleBitmap, d.HDC, uintptr(W), uintptr(H))
	old := call(pSelectObject, mem, bmp)
	pressed := d.ItemState&ODS_SELECTED != 0

	g := newGfx(mem)
	bg := cCard
	if id == idPower {
		bg = cBg
	}
	g.rectColor(bg, 0, 0, W, H)

	label := buttonText(id)
	font := fBtn
	var align uint32 = DT_CENTER
	switch {
	case id == idPower:
		font = fPower
		if boosterOn {
			b := gradientBrush(0, 0, W, H, cAccent, cAccent2)
			g.round(b, 0, 0, W, H, s(12))
			freeBrush(b)
			label = "BOOSTER ACTIVÉ"
		} else {
			b := gradientBrush(0, 0, W, H, cAccent, cAccent2)
			g.round(b, 0, 0, W, H, s(12))
			freeBrush(b)
			g.roundColor(cBtn, 2, 2, W-4, H-4, s(11))
			label = "ACTIVER LE BOOSTER"
		}
		if pressed {
			g.roundColor(color{0x10, 0x10, 0x18}, 2, 2, W-4, H-4, s(11))
		}
	case id >= idToggle0 && id < idToggle0+len(toggles):
		t := toggles[id-idToggle0]
		on := t.get()
		pw, ph := s(40), s(22)
		px, py := W-pw-s(4), (H-ph)/2
		if on {
			b := gradientBrush(px, py, px+pw, py, cAccent, cAccent2)
			g.round(b, px, py, pw, ph, ph/2)
			freeBrush(b)
		} else {
			g.roundColor(cOff, px, py, pw, ph, ph/2)
		}
		k := ph - s(6)
		kx := px + s(3)
		if on {
			kx = px + pw - s(3) - k
		}
		g.ellipseColor(cWhite, kx, py+s(3), k, k)
		label = t.label
		font = fNormal
		align = DT_LEFT
	default:
		c := cBtn
		if id == idAdd {
			c = cAccent
		}
		if pressed {
			c = color{c.r * 3 / 4, c.g * 3 / 4, c.b * 3 / 4}
		}
		g.roundColor(c, 0, 0, W, H, s(8))
	}
	g.close()

	r := RECT{0, 0, W, H}
	if align == DT_LEFT {
		r.Left, r.Right = s(4), W-s(52)
	}
	if pressed {
		r.Top++
		r.Bottom++
	}
	drawText(mem, label, font, cText, r, align|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)

	call(pBitBlt, d.HDC, uintptr(d.RcItem.Left), uintptr(d.RcItem.Top), uintptr(W), uintptr(H), mem, 0, 0, SRCCOPY)
	call(pSelectObject, mem, old)
	call(pDeleteObject, bmp)
	call(pDeleteDC, mem)
}

// ---- Rafraîchissement / journal -----------------------------------------

func refreshUI() {
	if hwndMain == 0 {
		return
	}
	call(pInvalidateRect, hwndMain, 0, 0)
	for _, h := range buttons {
		call(pInvalidateRect, h, 0, 0)
	}
	trayTip()
}

func refreshStats() {
	r := rc(0, 0, clientW, 214)
	call(pInvalidateRect, hwndMain, uintptr(unsafe.Pointer(&r)), 0)
}

func addLog(msg string) {
	line := time.Now().Format("15:04:05") + "   " + msg
	logFile(line)
	if hLog == 0 {
		return
	}
	call(pSendMessageW, hLog, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(w(line))))
	n := call(pSendMessageW, hLog, LB_GETCOUNT, 0, 0)
	if n > 300 {
		call(pSendMessageW, hLog, LB_DELETESTRING, 0, 0)
		n--
	}
	call(pSendMessageW, hLog, LB_SETTOPINDEX, n-1, 0)
}

func logFile(line string) {
	p := dataFile("journal.txt")
	if st, err := os.Stat(p); err == nil && st.Size() > 1<<20 {
		os.Remove(p)
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		f.WriteString(time.Now().Format("2006-01-02 ") + line + "\r\n")
		f.Close()
	}
}

func refreshGames() {
	call(pSendMessageW, hGames, LB_RESETCONTENT, 0, 0)
	for _, g := range cfg.CustomGames {
		call(pSendMessageW, hGames, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(w(g+".exe"))))
	}
}

func addCustom(name string) {
	b := baseName(strings.TrimSpace(strings.Trim(name, `"`)))
	if i := strings.LastIndexAny(b, `\/`); i >= 0 {
		b = b[i+1:]
	}
	if b == "" {
		return
	}
	for _, g := range cfg.CustomGames {
		if g == b {
			return
		}
	}
	cfg.CustomGames = append(cfg.CustomGames, b)
	saveConfig()
	refreshGames()
	addLog("Jeu ajouté : " + b + ".exe")
}

func windowText(h uintptr) string {
	n := call(pGetWindowTextLengthW, h)
	buf := make([]uint16, n+1)
	call(pGetWindowTextW, h, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}

var pickIgnore = toSet("explorer", "applicationframehost", "textinputhost", "systemsettings", "shellexperiencehost",
	"searchhost", "startmenuexperiencehost", "fpsboost", "lockapp", "taskmgr")

func pickGame() {
	wins := windowPids()
	self := uint32(os.Getpid())
	seen := map[string]bool{}
	var names []string
	for _, p := range snapshot() {
		b := baseName(p.Name)
		if !wins[p.Pid] || p.Pid == self || pickIgnore[b] || seen[b] {
			continue
		}
		seen[b] = true
		names = append(names, p.Name)
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	if len(names) == 0 {
		msgBox("Lancez votre jeu, puis cliquez à nouveau sur « Ajouter » pour le choisir dans la liste.", MB_OK|MB_ICONINFO)
		return
	}
	menu := call(pCreatePopupMenu)
	call(pAppendMenuW, menu, MF_STRING|MF_GRAYED, 0, uintptr(unsafe.Pointer(w("Choisissez le jeu en cours :"))))
	call(pAppendMenuW, menu, MF_SEPARATOR, 0, 0)
	for i, n := range names {
		call(pAppendMenuW, menu, MF_STRING, uintptr(idPickBase+i), uintptr(unsafe.Pointer(w(n))))
	}
	var r RECT
	call(pGetWindowRect, buttons[idAdd], uintptr(unsafe.Pointer(&r)))
	cmd := int(call(pTrackPopupMenu, menu, TPM_RETURNCMD|TPM_RIGHTBUTTON, uintptr(r.Left), uintptr(r.Bottom), 0, hwndMain, 0))
	call(pDestroyMenu, menu)
	if cmd >= idPickBase && cmd < idPickBase+len(names) {
		addCustom(names[cmd-idPickBase])
	}
}

func msgBox(text string, flags uintptr) uintptr {
	return call(pMessageBoxW, hwndMain, uintptr(unsafe.Pointer(w(text))), uintptr(unsafe.Pointer(w("FPS Boost"))), flags)
}

// ---- Zone de notification -------------------------------------------------

func copyU16(dst []uint16, s string) {
	u, _ := syscall.UTF16FromString(s)
	if len(u) > len(dst) {
		u = u[:len(dst)]
		u[len(u)-1] = 0
	}
	copy(dst, u)
}

func trayAdd() {
	nid = NOTIFYICONDATAW{}
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = hwndMain
	nid.UID = 1
	nid.UFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP
	nid.UCallbackMessage = WM_TRAY
	nid.HIcon = iconSmall
	copyU16(nid.SzTip[:], "FPS Boost")
	call(pShell_NotifyIconW, NIM_ADD, uintptr(unsafe.Pointer(&nid)))
}

func trayTip() {
	t := "FPS Boost — désactivé"
	if boosterOn && sess != nil {
		t = "FPS Boost — " + sess.Game
		if fpsValue > 0 {
			t += " — " + itoa(fpsValue) + " FPS"
		}
	} else if boosterOn {
		t = "FPS Boost — en attente d'un jeu"
	}
	nid.UFlags = NIF_TIP
	nid.SzTip = [128]uint16{}
	copyU16(nid.SzTip[:], t)
	call(pShell_NotifyIconW, NIM_MODIFY, uintptr(unsafe.Pointer(&nid)))
}

func balloon(title, text string) {
	balloonIsMaj = false
	n := nid
	n.UFlags = NIF_INFO
	n.SzInfo = [256]uint16{}
	n.SzInfoTitle = [64]uint16{}
	copyU16(n.SzInfo[:], text)
	copyU16(n.SzInfoTitle[:], title)
	n.DwInfoFlags = NIIF_INFO
	call(pShell_NotifyIconW, NIM_MODIFY, uintptr(unsafe.Pointer(&n)))
}

func trayMenu() {
	menu := call(pCreatePopupMenu)
	call(pAppendMenuW, menu, MF_STRING, idTrayOpen, uintptr(unsafe.Pointer(w("Ouvrir FPS Boost"))))
	t := "Activer le booster"
	if boosterOn {
		t = "Désactiver le booster"
	}
	call(pAppendMenuW, menu, MF_STRING, idTrayToggle, uintptr(unsafe.Pointer(w(t))))
	call(pAppendMenuW, menu, MF_STRING, idTrayUpdate, uintptr(unsafe.Pointer(w("Rechercher des mises à jour"))))
	call(pAppendMenuW, menu, MF_SEPARATOR, 0, 0)
	call(pAppendMenuW, menu, MF_STRING, idTrayQuit, uintptr(unsafe.Pointer(w("Quitter"))))
	var pt POINT
	call(pGetCursorPos, uintptr(unsafe.Pointer(&pt)))
	call(pSetForegroundWindow, hwndMain)
	cmd := call(pTrackPopupMenu, menu, TPM_RETURNCMD|TPM_RIGHTBUTTON, uintptr(pt.X), uintptr(pt.Y), 0, hwndMain, 0)
	call(pDestroyMenu, menu)
	call(pPostMessageW, hwndMain, 0, 0, 0)
	switch cmd {
	case idTrayOpen:
		showMain()
	case idTrayToggle:
		setBooster(!boosterOn)
	case idTrayQuit:
		quit()
	case idTrayUpdate:
		checkForUpdate(true)
	}
}

func showMain() {
	call(pShowWindow, hwndMain, SW_SHOW)
	if call(pIsIconic, hwndMain) != 0 {
		call(pShowWindow, hwndMain, SW_RESTORE)
	}
	call(pSetForegroundWindow, hwndMain)
	refreshUI()
}

func hideToTray() {
	call(pShowWindow, hwndMain, SW_HIDE)
	if !cfg.TrayHintShown {
		balloon("FPS Boost tourne en arrière-plan", "Il continue de booster vos jeux. Cliquez sur l'icône pour l'ouvrir.")
		cfg.TrayHintShown = true
		saveConfig()
	}
}

func quit() {
	endBoost(false)
	fpsStop()
	call(pDestroyWindow, hwndMain)
}

// ---- Actions ---------------------------------------------------------------

func restoreAll() {
	setBooster(false)
	restoreTweaks()
	if backup.CreatedScheme != "" {
		if activeScheme() == backup.CreatedScheme {
			prev := backup.PowerScheme
			if prev == "" || prev == backup.CreatedScheme {
				prev = "381b4222-f694-41f0-9685-ff5bb260df2e"
			}
			setScheme(prev)
		}
		run("powercfg", "/delete", backup.CreatedScheme)
	}
	*backup = Backup{}
	os.Remove(dataFile("backup.json"))
	removeStartup()
	startupOn = false
	addLog("Tout a été restauré comme avant")
	refreshUI()
	msgBox("Tous les réglages ont été remis comme avant.\nRedémarrez le PC pour terminer.", MB_OK|MB_ICONINFO)
}

func onCommand(id, code int) {
	switch {
	case id == idPower:
		setBooster(!boosterOn)
	case id >= idToggle0 && id < idToggle0+len(toggles):
		t := toggles[id-idToggle0]
		t.set(!t.get())
		saveConfig()
		call(pInvalidateRect, buttons[id], 0, 0)
	case id == idOptimize:
		n := applyTweaks()
		addLog(fmt.Sprintf("Windows optimisé : %d réglages appliqués", n))
		addLog("Redémarrez le PC une fois pour activer l'accélération GPU")
		call(pInvalidateRect, buttons[id], 0, 0)
	case id == idRam:
		if mb, ok := purgeStandby(); ok {
			addLog(fmt.Sprintf("RAM optimisée : %d Mo de cache libérés", mb))
		} else {
			addLog("Impossible de vider la RAM pour le moment")
		}
		refreshStats()
	case id == idRestore:
		if msgBox("Remettre Windows exactement comme avant FPS Boost ?\n(désactive aussi le booster et le démarrage automatique)", MB_YESNO|MB_ICONQUESTION) == IDYES {
			restoreAll()
		}
	case id == idHide:
		if pendingUpdate != nil {
			offerUpdate()
		} else {
			checkForUpdate(true)
		}
	case id == idAdd:
		if t := strings.TrimSpace(windowText(hEdit)); t != "" {
			addCustom(t)
			call(pSetWindowTextW, hEdit, uintptr(unsafe.Pointer(w(""))))
		} else {
			pickGame()
		}
	case id == idGames && code == LBN_DBLCLK:
		i := int(call(pSendMessageW, hGames, LB_GETCURSEL, 0, 0))
		if i >= 0 && i < len(cfg.CustomGames) {
			addLog("Jeu retiré : " + cfg.CustomGames[i] + ".exe")
			cfg.CustomGames = append(cfg.CustomGames[:i], cfg.CustomGames[i+1:]...)
			saveConfig()
			refreshGames()
		}
	}
}

// ---- Procédure de fenêtre --------------------------------------------------

func wndProc(hwnd, msg, wp, lp uintptr) (ret uintptr) {
	defer func() {
		if r := recover(); r != nil {
			logFile(fmt.Sprint("ERREUR: ", r))
			ret = 0
		}
	}()
	if msgTaskbarCreated != 0 && msg == msgTaskbarCreated {
		trayAdd()
		return 0
	}
	if handleUpdateMessages(msg, wp) {
		return 0
	}
	switch msg {
	case WM_PAINT:
		paint(hwnd)
		return 0
	case WM_ERASEBKGND:
		return 1
	case WM_DRAWITEM:
		drawItem((*DRAWITEMSTRUCT)(unsafe.Pointer(lp)))
		return 1
	case WM_CTLCOLOREDIT, WM_CTLCOLORLISTBOX, WM_CTLCOLORSTATIC:
		c := cText
		if lp == hLog {
			c = color{0xB8, 0xBD, 0xCC}
		}
		call(pSetTextColor, wp, uintptr(c.ref()))
		call(pSetBkColor, wp, uintptr(cInput.ref()))
		return brInput
	case WM_COMMAND:
		onCommand(int(wp&0xFFFF), int((wp>>16)&0xFFFF))
		return 0
	case WM_TIMER:
		if wp == 1 {
			tick()
		}
		if wp == 3 {
			checkForUpdate(false)
		}
		if wp == 2 {
			fpsUpdate()
			if sess != nil {
				trayTip()
			}
			if call(pIsWindowVisible, hwnd) != 0 {
				refreshStats()
			}
		}
		return 0
	case WM_HANDOFF: // une nouvelle version vient d'être lancée
		if wp > appVersion {
			call(pPostMessageW, hwnd, WM_QUITAPP, 0, 0)
			return 1
		}
		return 2
	case WM_QUITAPP:
		quit()
		return 0
	case WM_TRAY:
		switch lp & 0xFFFF {
		case WM_LBUTTONUP, WM_LBUTTONDBLCLK:
			showMain()
		case WM_RBUTTONUP:
			trayMenu()
		case NIN_BALLOONCLICK:
			if balloonIsMaj {
				balloonIsMaj = false
				showMain()
				offerUpdate()
			} else {
				showMain()
			}
		}
		return 0
	case WM_SHOWME:
		showMain()
		return 0
	case WM_CLOSE:
		hideToTray()
		return 0
	case WM_QUERYENDSESSION:
		return 1
	case WM_ENDSESSION:
		if wp != 0 {
			endBoost(true)
		}
		return 0
	case WM_DESTROY:
		call(pShell_NotifyIconW, NIM_DELETE, uintptr(unsafe.Pointer(&nid)))
		call(pPostQuitMessage, 0)
		return 0
	}
	return call(pDefWindowProcW, hwnd, msg, wp, lp)
}
