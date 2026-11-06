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

	// Navigation
	idNav0 = 100 // 100..104

	// Accueil
	idPower    = 150
	idRam      = 151
	idOptimize = 152
	idRestore  = 153

	// Paramètres
	idToggle0 = 200 // 200..2xx

	// Overlay
	idOvToggle0 = 250 // 250..252
	idOvShow    = 260
	idOvPos0    = 270 // 270..273

	// Jeux
	idEdit   = 300
	idAdd    = 301
	idPick   = 302
	idGames  = 303
	idRemove = 304

	// Journal
	idLog     = 400
	idOpenLog = 401
	idClear   = 402

	// Mises à jour
	idCheckUpd = 500
	idGithub   = 501

	// Menu de l'icône
	idTrayOpen   = 600
	idTrayToggle = 601
	idTrayQuit   = 602
	idTrayUpdate = 603
	idTrayRam    = 604
	idPickBase   = 1000

	WM_TRAY      = WM_APP + 1
	WM_SHOWME    = WM_APP + 2
	WM_HANDOFF   = WM_APP + 3
	WM_QUITAPP   = WM_APP + 4
	WM_LAUNCHRAM = WM_APP + 8

	clientW = 900
	clientH = 600
	sideW   = 210
)

var (
	hInst, hwndMain                                       uintptr
	dpi                                                   int32 = 96
	fH1, fSub, fNormal, fBold, fSmall, fStat, fSection    uintptr
	fBtn, fPower, fHuge, fIcon, fIconBig, fLogo           uintptr
	brInput                                               uintptr
	hEdit, hGames, hLog                                   uintptr
	buttons                                               = map[int]uintptr{}
	pageCtrls                                             = map[int][]uintptr{}
	curPage                                               int
	iconBig, iconSmall                                    uintptr
	nid                                                   NOTIFYICONDATAW
	msgTaskbarCreated                                     uintptr
	startupOn                                             bool
	logLines                                              []string
	trayTick                                              int
	pDrawIconEx                                           = modUser32.NewProc("DrawIconEx")
	cSide                                                 = color{0x10, 0x12, 0x19}
	cNavActive                                            = color{0x1D, 0x21, 0x2E}
)

func s(v int32) int32 { return v * dpi / 96 }

// ---- Pages et menu ---------------------------------------------------------

type navItem struct {
	icon, label string
}

var navItems = []navItem{
	{"", "Accueil"},
	{"", "Paramètres"},
	{"", "Overlay"},
	{"", "Jeux"},
	{"", "Journal"},
	{"", "Mises à jour"},
}

var pageTitles = [][2]string{
	{"Tableau de bord", "Booster automatique pour tous vos jeux"},
	{"Paramètres", "Réglages appliqués automatiquement pendant vos parties"},
	{"Overlay en jeu", "Une petite barre par-dessus le jeu : FPS, processeur, carte graphique, RAM"},
	{"Jeux", "FPS Boost détecte vos jeux tout seul. Ajoutez ici ceux qui ne sont pas reconnus."},
	{"Journal", "Tout ce que fait FPS Boost, en direct"},
	{"Mises à jour", "FPS Boost se met à jour tout seul depuis GitHub"},
}

type toggleDef struct {
	label, desc string
	get         func() bool
	set         func(bool)
}

var toggles = []toggleDef{
	{"Alimentation performances maximales", "Le processeur reste à pleine vitesse pendant le jeu",
		func() bool { return cfg.PowerPlan }, func(v bool) { cfg.PowerPlan = v }},
	{"Timer système 0,5 ms", "Windows plus réactif, partie plus fluide",
		func() bool { return cfg.TimerRes }, func(v bool) { cfg.TimerRes = v }},
	{"Nettoyage de la RAM en jeu", "Vide le cache au lancement du jeu, puis toutes les 10 min si besoin",
		func() bool { return cfg.RamClean }, func(v bool) { cfg.RamClean = v }},
	{"Nettoyage de la RAM à l'ouverture", "Libère la mémoire dès que vous ouvrez FPS Boost",
		func() bool { return cfg.RamOnLaunch }, func(v bool) { cfg.RamOnLaunch = v }},
	{"Priorité Haute pour le jeu", "Le jeu passe devant tous les autres programmes",
		func() bool { return cfg.GamePriority }, func(v bool) { cfg.GamePriority = v }},
	{"Applis d'arrière-plan ralenties", "Navigateurs, Spotify, OneDrive… en priorité basse pendant la partie",
		func() bool { return cfg.BgPriority }, func(v bool) { cfg.BgPriority = v }},
	{"Lancer au démarrage de Windows", "FPS Boost démarre tout seul, discrètement, près de l'horloge",
		func() bool { return startupOn }, func(v bool) {
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
	{"Activer le booster à l'ouverture", "Plus besoin de cliquer sur « Activer le booster »",
		func() bool { return cfg.AutoActivate }, func(v bool) { cfg.AutoActivate = v }},
	{"Notifications", "Prévient quand un jeu est boosté ou qu'une mise à jour arrive",
		func() bool { return cfg.Notifications }, func(v bool) { cfg.Notifications = v }},
}

var ovToggles = []toggleDef{
	{"Ouvrir automatiquement en jeu", "L'overlay apparaît dès qu'un jeu est détecté (sinon : touche Inser)",
		func() bool { return cfg.OverlayAuto }, func(v bool) { cfg.OverlayAuto = v }},
	{"Afficher les températures", "Température du processeur et de la carte graphique",
		func() bool { return cfg.OverlayTemps }, func(v bool) { cfg.OverlayTemps = v }},
	{"Afficher la RAM", "Pourcentage de mémoire utilisée",
		func() bool { return cfg.OverlayRAM }, func(v bool) { cfg.OverlayRAM = v }},
}

var posLabels = []string{"Haut gauche", "Haut droite", "Bas gauche", "Bas droite"}

func toggleByID(id int) (*toggleDef, bool) {
	if id >= idToggle0 && id < idToggle0+len(toggles) {
		return &toggles[id-idToggle0], id-idToggle0 == len(toggles)-1
	}
	if id >= idOvToggle0 && id < idOvToggle0+len(ovToggles) {
		return &ovToggles[id-idOvToggle0], id-idOvToggle0 == len(ovToggles)-1
	}
	return nil, false
}

// ---- Création de la fenêtre --------------------------------------------------

func mkFont(px, weight int32, face string) uintptr {
	h := int32(-s(px))
	return call(pCreateFontW, uintptr(h), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0,
		uintptr(unsafe.Pointer(w(face))))
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

	ui := "Segoe UI"
	fH1 = mkFont(24, 700, ui)
	fSub = mkFont(13, 400, ui)
	fNormal = mkFont(14, 400, ui)
	fBold = mkFont(14, 600, ui)
	fSmall = mkFont(12, 400, ui)
	fStat = mkFont(20, 700, ui)
	fSection = mkFont(11, 700, ui)
	fBtn = mkFont(13, 600, ui)
	fPower = mkFont(14, 700, ui)
	fHuge = mkFont(64, 800, ui)
	fLogo = mkFont(17, 800, ui)
	fIcon = mkFont(16, 400, "Segoe MDL2 Assets")
	fIconBig = mkFont(18, 400, "Segoe MDL2 Assets")
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
	cap := cSide.ref()
	call(pDwmSetWindowAttribute, hwndMain, 35, uintptr(unsafe.Pointer(&cap)), 4) // couleur de la barre de titre (Win11)

	createControls()
	showPage(0)
	msgTaskbarCreated = call(pRegisterWindowMessageW, uintptr(unsafe.Pointer(w("TaskbarCreated"))))
	trayAdd()
}

func child(class string, style uintptr, id int, x, y, wd, ht int32) uintptr {
	return call(pCreateWindowExW, 0, uintptr(unsafe.Pointer(w(class))), uintptr(unsafe.Pointer(w(""))),
		WS_CHILD|style, uintptr(s(x)), uintptr(s(y)), uintptr(s(wd)), uintptr(s(ht)), hwndMain, uintptr(id), hInst, 0)
}

func button(page, id int, x, y, wd, ht int32) {
	h := child("BUTTON", WS_TABSTOP|BS_OWNERDRAW, id, x, y, wd, ht)
	buttons[id] = h
	pageCtrls[page] = append(pageCtrls[page], h)
}

func listbox(page, id int, style uintptr, x, y, wd, ht int32) uintptr {
	h := child("LISTBOX", WS_VSCROLL|LBS_NOINTEGRALHEIGHT|style, id, x, y, wd, ht)
	call(pSendMessageW, h, WM_SETFONT, fSmall, 1)
	call(pSendMessageW, h, LB_SETITEMHEIGHT, 0, uintptr(s(20)))
	call(pSetWindowTheme, h, uintptr(unsafe.Pointer(w("DarkMode_Explorer"))), 0)
	pageCtrls[page] = append(pageCtrls[page], h)
	return h
}

func createControls() {
	// Menu latéral (toujours visible)
	for i := range navItems {
		h := child("BUTTON", WS_VISIBLE|WS_TABSTOP|BS_OWNERDRAW, idNav0+i, 12, 92+int32(i)*46, 186, 40)
		buttons[idNav0+i] = h
	}
	// Accueil
	button(0, idPower, 676, 26, 204, 44)
	button(0, idRam, 246, 350, 196, 38)
	button(0, idOptimize, 454, 350, 196, 38)
	button(0, idRestore, 662, 350, 202, 38)
	// Paramètres
	for i := range toggles {
		button(1, idToggle0+i, 246, 100+int32(i)*53, 618, 50)
	}
	// Overlay
	button(2, idOvShow, 246, 196, 230, 38)
	for i := range ovToggles {
		button(2, idOvToggle0+i, 246, 284+int32(i)*53, 618, 50)
	}
	for i := range posLabels {
		button(2, idOvPos0+i, 246+int32(i)*156, 476, 148, 34)
	}
	// Jeux
	hEdit = child("EDIT", WS_TABSTOP|ES_AUTOHSCROLL, idEdit, 258, 133, 346, 20)
	call(pSendMessageW, hEdit, WM_SETFONT, fNormal, 1)
	call(pSendMessageW, hEdit, EM_SETCUEBANNER, 1, uintptr(unsafe.Pointer(w("Nom du jeu, ex : MonJeu.exe"))))
	pageCtrls[3] = append(pageCtrls[3], hEdit)
	button(3, idAdd, 626, 126, 110, 34)
	button(3, idPick, 744, 126, 120, 34)
	hGames = listbox(3, idGames, LBS_NOTIFY, 252, 276, 606, 242)
	button(3, idRemove, 246, 534, 200, 34)
	// Journal
	hLog = listbox(4, idLog, LBS_NOSEL, 252, 108, 606, 422)
	button(4, idOpenLog, 246, 546, 220, 32)
	button(4, idClear, 476, 546, 120, 32)
	// Mises à jour
	button(5, idCheckUpd, 250, 236, 250, 40)
	button(5, idGithub, 512, 236, 180, 40)

	refreshGames()
}

func showPage(p int) {
	curPage = p
	for page, list := range pageCtrls {
		cmd := uintptr(SW_HIDE)
		if page == p {
			cmd = SW_SHOW
		}
		for _, h := range list {
			call(pShowWindow, h, cmd)
		}
	}
	if p == 4 && hLog != 0 {
		n := call(pSendMessageW, hLog, LB_GETCOUNT, 0, 0)
		if n > 0 {
			call(pSendMessageW, hLog, LB_SETTOPINDEX, n-1, 0)
		}
	}
	refreshUI()
}

// ---- Dessin -------------------------------------------------------------------

func card(g gfx, x, y, wd, ht int32) {
	g.roundColor(cBorder, s(x), s(y), s(wd), s(ht), s(12))
	g.roundColor(cCard, s(x)+1, s(y)+1, s(wd)-2, s(ht)-2, s(12)-1)
}

func box(g gfx, x, y, wd, ht int32) {
	g.roundColor(cBorder, s(x), s(y), s(wd), s(ht), s(8))
	g.roundColor(cInput, s(x)+1, s(y)+1, s(wd)-2, s(ht)-2, s(8)-1)
}

func rc(l, t, r, b int32) RECT { return RECT{s(l), s(t), s(r), s(b)} }

func statusInfo() (dot color, title, sub string) {
	switch {
	case boosterOn && sess != nil:
		return cGreen, "BOOST ACTIF  —  " + sess.Game, "depuis " + sess.Start.Format("15:04") + "  ·  " + fmtDur(time.Since(sess.Start))
	case boosterOn:
		return cAccent, "En attente d'un jeu…", "Lancez n'importe quel jeu, il sera détecté automatiquement"
	}
	return cOff, "Booster désactivé", "Cliquez sur « Activer le booster »"
}

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
	// Barre latérale
	g.rectColor(cSide, 0, 0, s(sideW), H)
	g.rectColor(cBorder, s(sideW)-1, 0, 1, H)
	gb := gradientBrush(0, 0, s(sideW), 0, cAccent, cAccent2)
	g.rect(gb, 0, 0, s(sideW), s(3))
	freeBrush(gb)
	dot, title, sub := statusInfo()
	g.roundColor(cBorder, s(12), s(508), s(186), s(76), s(10))
	g.roundColor(color{0x15, 0x18, 0x21}, s(12)+1, s(508)+1, s(186)-2, s(76)-2, s(10)-1)
	if boosterOn {
		g.ellipseColor(color{dot.r / 3, dot.g / 3, dot.b / 3}, s(22), s(520), s(16), s(16))
	}
	g.ellipseColor(dot, s(25), s(523), s(10), s(10))

	switch curPage {
	case 0:
		paintHomeShapes(g, dot)
	case 1:
		card(g, 230, 88, 650, 492)
	case 2:
		card(g, 230, 88, 650, 170)
		card(g, 230, 272, 650, 316)
		call(pGdipSetTextRenderingHint, g.g, 5)
		pill(g.g, s(246), s(128), true)
	case 3:
		card(g, 230, 88, 650, 132)
		box(g, 246, 126, 370, 34)
		card(g, 230, 234, 650, 354)
		box(g, 246, 272, 618, 250)
	case 4:
		card(g, 230, 88, 650, 500)
		box(g, 246, 104, 618, 430)
	case 5:
		card(g, 230, 88, 650, 210)
		card(g, 230, 312, 650, 276)
	}
	g.close()

	// Logo
	call(pDrawIconEx, mem, uintptr(s(20)), uintptr(s(24)), iconBig, uintptr(s(36)), uintptr(s(36)), 0, 0, 3)
	drawText(mem, "FPS BOOST", fLogo, cText, rc(66, 24, 205, 44), DT_SINGLELINE)
	drawText(mem, "Version "+appVersionStr, fSmall, cMuted, rc(66, 44, 205, 62), DT_SINGLELINE)
	// Statut dans la barre latérale
	st := "Booster désactivé"
	st2 := "Cliquez sur Activer"
	if boosterOn && sess != nil {
		st = "Boost actif"
		st2 = sess.Game
		if fpsValue > 0 {
			st2 += "  ·  " + itoa(fpsValue) + " FPS"
		}
	} else if boosterOn {
		st, st2 = "Booster activé", "En attente d'un jeu…"
	}
	if lastSelfWall.IsZero() {
		updateSelfUsage()
	}
	drawText(mem, "Consommation de FPS Boost", fSmall, cMuted, rc(14, 466, 205, 484), DT_SINGLELINE)
	drawText(mem, strings.Replace(fmt.Sprintf("%.1f %% CPU  ·  %.0f Mo", selfCPU, selfMemMB), ".", ",", 1), fBold, cText, rc(14, 484, 205, 504), DT_SINGLELINE)
	drawText(mem, st, fBold, cText, rc(42, 516, 192, 534), DT_SINGLELINE|DT_END_ELLIPSIS)
	drawText(mem, st2, fSmall, cMuted, rc(24, 540, 192, 576), DT_WORDBREAK|DT_END_ELLIPSIS)

	// En-tête de page
	drawText(mem, pageTitles[curPage][0], fH1, cText, rc(230, 18, 660, 52), DT_SINGLELINE)
	drawText(mem, pageTitles[curPage][1], fSub, cMuted, rc(231, 52, 880, 72), DT_SINGLELINE|DT_END_ELLIPSIS)

	switch curPage {
	case 0:
		paintHomeText(mem, title, sub)
	case 1:
	case 2:
		drawText(mem, "APERÇU", fSection, cMuted, rc(246, 102, 860, 118), DT_SINGLELINE)
		drawText(mem, "Touche  Inser  en jeu", fBold, cText, rc(492, 196, 864, 216), DT_SINGLELINE)
		drawText(mem, "pour ouvrir ou fermer l'overlay", fSmall, cMuted, rc(492, 216, 864, 234), DT_SINGLELINE)
		drawText(mem, "POSITION À L'ÉCRAN", fSection, cMuted, rc(246, 454, 860, 470), DT_SINGLELINE)
		drawText(mem, "L'overlay s'affiche par-dessus les jeux en « Plein écran fenêtré » ou « Sans bordure ». En plein écran exclusif, il peut ne pas apparaître : changez ce réglage dans les options vidéo du jeu. Les clics passent au travers, il ne gêne jamais.",
			fSmall, cMuted, rc(246, 520, 866, 584), DT_WORDBREAK)
	case 3:
		drawText(mem, "AJOUTER UN JEU", fSection, cMuted, rc(246, 102, 860, 118), DT_SINGLELINE)
		drawText(mem, "Détectés automatiquement : Steam, Epic Games, Riot, Xbox, Ubisoft, EA, GOG, Rockstar, Battle.net, Amazon Games, itch.io et plus de 60 jeux populaires. Laissez le champ vide et cliquez sur « Jeu ouvert… » pour choisir un jeu en cours.",
			fSmall, cMuted, rc(246, 168, 866, 214), DT_WORDBREAK)
		drawText(mem, "JEUX AJOUTÉS MANUELLEMENT", fSection, cMuted, rc(246, 248, 860, 264), DT_SINGLELINE)
		if len(cfg.CustomGames) == 0 {
			drawText(mem, "Aucun jeu ajouté pour l'instant", fSmall, cMuted, rc(246, 534, 864, 568), DT_SINGLELINE|DT_VCENTER|DT_RIGHT)
		}
	case 4:
	case 5:
		paintUpdateText(mem)
	}

	call(pBitBlt, hdc, 0, 0, uintptr(W), uintptr(H), mem, 0, 0, SRCCOPY)
	call(pSelectObject, mem, old)
	call(pDeleteObject, bmp)
	call(pDeleteDC, mem)
	call(pEndPaint, hwnd, uintptr(unsafe.Pointer(&ps)))
}

func paintHomeShapes(g gfx, dot color) {
	card(g, 230, 88, 650, 56)
	if boosterOn {
		g.ellipseColor(color{dot.r / 3, dot.g / 3, dot.b / 3}, s(246), s(106), s(20), s(20))
	}
	g.ellipseColor(dot, s(250), s(110), s(12), s(12))
	// Carte FPS avec léger dégradé
	g.roundColor(cBorder, s(230), s(158), s(316), s(156), s(12))
	b := gradientBrush(s(230), s(158), s(546), s(314), color{0x1E, 0x18, 0x36}, color{0x10, 0x22, 0x2C})
	g.round(b, s(231), s(159), s(314), s(154), s(11))
	freeBrush(b)
	for i := int32(0); i < 4; i++ {
		x := 562 + (i%2)*167
		y := 158 + (i/2)*86
		card(g, x, y, 151, 70)
	}
	card(g, 230, 330, 650, 76)
	card(g, 230, 420, 650, 166)
}

func paintHomeText(mem uintptr, title, sub string) {
	drawText(mem, title, fBold, cText, rc(272, 88, 560, 144), DT_SINGLELINE|DT_VCENTER|DT_END_ELLIPSIS)
	drawText(mem, sub, fSmall, cMuted, rc(520, 88, 864, 144), DT_SINGLELINE|DT_VCENTER|DT_RIGHT|DT_END_ELLIPSIS)

	// FPS
	fpsTxt, fpsC, fpsSub := "—", cMuted, "Lancez un jeu pour voir vos FPS"
	if sess != nil {
		fpsSub = sess.Game
		switch {
		case fpsValue > 0:
			fpsTxt, fpsC = itoa(fpsValue), cGreen
			if fpsValue < 60 {
				fpsC = color{0xF5, 0x9E, 0x0B}
			}
			if fpsAvg > 0 {
				fpsSub = "moyenne " + itoa(fpsAvg) + "  ·  " + sess.Game
			}
		case time.Since(fpsStarted) < 15*time.Second:
			fpsTxt, fpsSub = "…", "Mesure en cours…"
		default:
			fpsSub = "Non mesurable pour ce jeu (Vulkan/OpenGL)"
		}
	}
	drawText(mem, "FPS EN DIRECT", fSection, color{0xA9, 0xA4, 0xD8}, rc(250, 174, 540, 190), DT_SINGLELINE)
	drawText(mem, fpsTxt, fHuge, fpsC, rc(248, 188, 540, 270), DT_SINGLELINE|DT_VCENTER)
	drawText(mem, fpsSub, fSmall, cMuted, rc(250, 278, 536, 300), DT_SINGLELINE|DT_END_ELLIPSIS)

	// Petites stats
	m := memStatus()
	cur, _ := timerInfo()
	power, powerC := "Normale", cText
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
	_ = timerC
	sd := sensors()
	comp := func(name string, use, temp float64, useOk, tempOk bool, hot float64) (string, string, color) {
		c := cText
		switch {
		case tempOk:
			if temp >= hot {
				c = color{0xF5, 0x9E, 0x0B}
			}
			l := name
			if useOk {
				l += fmt.Sprintf(" · %.0f %%", use)
			}
			return l, fmt.Sprintf("%.0f °C", temp), c
		case useOk:
			return name, fmt.Sprintf("%.0f %%", use), c
		}
		return name, "—", cMuted
	}
	cl, cv, cc := comp("CPU", sd.CPUUse, sd.CPUTemp, sd.CPUUseOk, sd.CPUTempOk, 85)
	gl, gv, gc := comp("GPU", sd.GPUUse, sd.GPUTemp, sd.GPUUseOk, sd.GPUTempOk, 83)
	stats := []struct {
		label, value string
		c            color
	}{
		{cl, cv, cc},
		{gl, gv, gc},
		{fmt.Sprintf("RAM · %d GO", (m.TotalPhys+(1<<29))>>30), fmt.Sprintf("%d %%", m.MemoryLoad), ramC},
		{"ALIMENTATION", power, powerC},
	}
	for i, st := range stats {
		x := 562 + int32(i%2)*167
		y := 158 + int32(i/2)*86
		drawText(mem, st.label, fSection, cMuted, rc(x+14, y+12, x+145, y+28), DT_SINGLELINE)
		drawText(mem, st.value, fStat, st.c, rc(x+14, y+30, x+145, y+62), DT_SINGLELINE|DT_VCENTER)
	}

	// Activité récente
	drawText(mem, "ACTIVITÉ RÉCENTE", fSection, cMuted, rc(246, 434, 860, 450), DT_SINGLELINE)
	start := len(logLines) - 6
	if start < 0 {
		start = 0
	}
	for i, l := range logLines[start:] {
		y := 456 + int32(i)*20
		drawText(mem, l, fSmall, color{0xB8, 0xBD, 0xCC}, rc(246, y, 864, y+20), DT_SINGLELINE|DT_END_ELLIPSIS)
	}
}

func paintUpdateText(mem uintptr) {
	drawText(mem, "FPS Boost "+appVersionStr, fH1, cText, rc(250, 104, 860, 138), DT_SINGLELINE)
	var st string
	c := cMuted
	switch updState {
	case updChecking:
		st = "Recherche de mises à jour…"
	case updUpToDate:
		st, c = "✓  Vous avez la dernière version", cGreen
	case updAvailable:
		if pendingUpdate != nil {
			st, c = "★  Nouvelle version "+pendingUpdate.Name+" disponible !", cAccent2
		}
	case updError:
		st, c = "Impossible de vérifier (pas de connexion internet ?)", color{0xF5, 0x9E, 0x0B}
	case updDownloading:
		st, c = "Téléchargement de la mise à jour…", cAccent2
	default:
		st = "Pas encore vérifié"
	}
	drawText(mem, st, fBold, c, rc(250, 146, 860, 170), DT_SINGLELINE)
	lc := "Vérification automatique à chaque ouverture et toutes les 6 heures"
	if !updLastCheck.IsZero() {
		lc = "Dernière vérification : " + updLastCheck.Format("15:04") + "  ·  " + lc
	}
	drawText(mem, lc, fSmall, cMuted, rc(250, 174, 860, 194), DT_SINGLELINE|DT_END_ELLIPSIS)
	drawText(mem, "Vos réglages et vos jeux sont conservés à chaque mise à jour.", fSmall, cMuted, rc(250, 196, 860, 216), DT_SINGLELINE)

	t, notes := "NOUVEAUTÉS DE CETTE VERSION", releaseNotes
	if pendingUpdate != nil {
		t, notes = "NOUVEAUTÉS DE LA VERSION "+pendingUpdate.Name, pendingUpdate.Notes
	}
	drawText(mem, t, fSection, cMuted, rc(250, 328, 860, 344), DT_SINGLELINE)
	drawText(mem, notes, fNormal, cText, rc(250, 352, 864, 580), DT_WORDBREAK)
}

func buttonText(id int) (icon, text string) {
	switch id {
	case idRam:
		return "", "Libérer la RAM"
	case idOptimize:
		if backup.Applied {
			return "", "Windows optimisé"
		}
		return "", "Optimiser Windows"
	case idRestore:
		return "", "Tout restaurer"
	case idAdd:
		return "", "Ajouter"
	case idPick:
		return "", "Jeu ouvert…"
	case idRemove:
		return "", "Retirer la sélection"
	case idOpenLog:
		return "", "Ouvrir le fichier journal"
	case idClear:
		return "", "Effacer"
	case idCheckUpd:
		if pendingUpdate != nil {
			return "", "Installer la version " + pendingUpdate.Name
		}
		return "", "Rechercher des mises à jour"
	case idGithub:
		return "", "Page GitHub"
	case idOvShow:
		if overlayVisible {
			return "", "Masquer l'overlay"
		}
		return "", "Afficher l'overlay"
	}
	if id >= idOvPos0 && id < idOvPos0+len(posLabels) {
		return "", posLabels[id-idOvPos0]
	}
	return "", ""
}

func drawItem(d *DRAWITEMSTRUCT) {
	id := int(d.CtlID)
	W, H := d.RcItem.Right-d.RcItem.Left, d.RcItem.Bottom-d.RcItem.Top
	mem := call(pCreateCompatibleDC, d.HDC)
	bmp := call(pCreateCompatibleBitmap, d.HDC, uintptr(W), uintptr(H))
	old := call(pSelectObject, mem, bmp)
	pressed := d.ItemState&ODS_SELECTED != 0

	bg := cCard
	switch {
	case id >= idNav0 && id < idNav0+len(navItems):
		bg = cSide
	case id == idPower:
		bg = cBg
	}
	g := newGfx(mem)
	g.rectColor(bg, 0, 0, W, H)

	switch {
	case id >= idNav0 && id < idNav0+len(navItems):
		i := id - idNav0
		active := i == curPage
		if active || pressed {
			g.roundColor(cNavActive, 0, 0, W, H, s(8))
		}
		if active {
			b := gradientBrush(0, 0, 0, H, cAccent, cAccent2)
			g.round(b, 0, s(9), s(3), H-s(18), s(2))
			freeBrush(b)
		}
		g.close()
		c := cMuted
		if active {
			c = cText
		}
		ic := c
		if active {
			ic = cAccent2
		}
		drawText(mem, navItems[i].icon, fIcon, ic, RECT{s(14), 0, s(40), H}, DT_SINGLELINE|DT_VCENTER)
		f := fNormal
		if active {
			f = fBold
		}
		drawText(mem, navItems[i].label, f, c, RECT{s(46), 0, W, H}, DT_SINGLELINE|DT_VCENTER)
		if i == 5 && pendingUpdate != nil {
			g2 := newGfx(mem)
			g2.ellipseColor(cAccent2, W-s(22), H/2-s(4), s(8), s(8))
			g2.close()
		}

	case id == idPower:
		b := gradientBrush(0, 0, W, H, cAccent, cAccent2)
		g.round(b, 0, 0, W, H, s(12))
		freeBrush(b)
		label := "BOOSTER ACTIVÉ"
		if !boosterOn {
			g.roundColor(cBtn, 2, 2, W-4, H-4, s(11))
			label = "ACTIVER LE BOOSTER"
		}
		if pressed {
			g.roundColor(color{0x10, 0x10, 0x18}, 2, 2, W-4, H-4, s(11))
		}
		g.close()
		lw := textWidth(mem, label, fPower) + s(26)
		x := (W - lw) / 2
		drawText(mem, "", fIcon, cText, RECT{x, 0, x + s(22), H}, DT_SINGLELINE|DT_VCENTER)
		drawText(mem, label, fPower, cText, RECT{x + s(26), 0, W, H}, DT_SINGLELINE|DT_VCENTER)

	case func() bool { t, _ := toggleByID(id); return t != nil }():
		t, last := toggleByID(id)
		on := t.get()
		pw, ph := s(44), s(24)
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
		if !last {
			g.rectColor(cBorder, 0, H-1, W, 1)
		}
		g.close()
		drawText(mem, t.label, fBold, cText, RECT{s(2), s(5), W - s(60), H / 2}, DT_SINGLELINE|DT_END_ELLIPSIS)
		drawText(mem, t.desc, fSmall, cMuted, RECT{s(2), H / 2, W - s(60), H - s(5)}, DT_SINGLELINE|DT_END_ELLIPSIS)

	default:
		c := cBtn
		if id == idAdd || id == idOvShow || (id == idCheckUpd && pendingUpdate != nil) || id == idOvPos0+cfg.OverlayPos {
			c = cAccent
		}
		if pressed {
			c = color{c.r * 3 / 4, c.g * 3 / 4, c.b * 3 / 4}
		}
		g.roundColor(c, 0, 0, W, H, s(9))
		g.close()
		icon, label := buttonText(id)
		iw := s(24)
		if icon == "" {
			iw = 0
		}
		lw := textWidth(mem, label, fBtn) + iw
		x := (W - lw) / 2
		off := int32(0)
		if pressed {
			off = 1
		}
		drawText(mem, icon, fIcon, cText, RECT{x, off, x + s(20), H + off}, DT_SINGLELINE|DT_VCENTER)
		drawText(mem, label, fBtn, cText, RECT{x + iw, off, W, H + off}, DT_SINGLELINE|DT_VCENTER)
	}

	call(pBitBlt, d.HDC, uintptr(d.RcItem.Left), uintptr(d.RcItem.Top), uintptr(W), uintptr(H), mem, 0, 0, SRCCOPY)
	call(pSelectObject, mem, old)
	call(pDeleteObject, bmp)
	call(pDeleteDC, mem)
}

// ---- Rafraîchissement / journal --------------------------------------------------

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
	r := rc(0, 0, clientW, clientH)
	if curPage != 0 {
		r = rc(0, 500, sideW, clientH) // seulement le statut de la barre latérale
	}
	call(pInvalidateRect, hwndMain, uintptr(unsafe.Pointer(&r)), 0)
}

func addLog(msg string) {
	line := time.Now().Format("15:04:05") + "   " + msg
	logFile(line)
	logLines = append(logLines, line)
	if len(logLines) > 300 {
		logLines = logLines[1:]
	}
	if hLog != 0 {
		call(pSendMessageW, hLog, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(w(line))))
		n := call(pSendMessageW, hLog, LB_GETCOUNT, 0, 0)
		if n > 300 {
			call(pSendMessageW, hLog, LB_DELETESTRING, 0, 0)
			n--
		}
		call(pSendMessageW, hLog, LB_SETTOPINDEX, n-1, 0)
	}
	if hwndMain != 0 && curPage == 0 {
		r := rc(230, 420, 880, 586)
		call(pInvalidateRect, hwndMain, uintptr(unsafe.Pointer(&r)), 0)
	}
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
		call(pSendMessageW, hGames, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(w("   "+g+".exe"))))
	}
	if hwndMain != 0 {
		call(pInvalidateRect, hwndMain, 0, 0)
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
		msgBox("Lancez votre jeu, puis cliquez à nouveau sur « Jeu ouvert… » pour le choisir dans la liste.", MB_OK|MB_ICONINFO)
		return
	}
	menu := call(pCreatePopupMenu)
	call(pAppendMenuW, menu, MF_STRING|MF_GRAYED, 0, uintptr(unsafe.Pointer(w("Choisissez le jeu en cours :"))))
	call(pAppendMenuW, menu, MF_SEPARATOR, 0, 0)
	for i, n := range names {
		call(pAppendMenuW, menu, MF_STRING, uintptr(idPickBase+i), uintptr(unsafe.Pointer(w(n))))
	}
	var r RECT
	call(pGetWindowRect, buttons[idPick], uintptr(unsafe.Pointer(&r)))
	cmd := int(call(pTrackPopupMenu, menu, TPM_RETURNCMD|TPM_RIGHTBUTTON, uintptr(r.Left), uintptr(r.Bottom), 0, hwndMain, 0))
	call(pDestroyMenu, menu)
	if cmd >= idPickBase && cmd < idPickBase+len(names) {
		addCustom(names[cmd-idPickBase])
	}
}

func msgBox(text string, flags uintptr) uintptr {
	return call(pMessageBoxW, hwndMain, uintptr(unsafe.Pointer(w(text))), uintptr(unsafe.Pointer(w("FPS Boost"))), flags)
}

func openPath(p string) {
	call(pShellExecuteW, 0, uintptr(unsafe.Pointer(w("open"))), uintptr(unsafe.Pointer(w(p))), 0, 0, SW_SHOW)
}

// ---- Nettoyage de la RAM -----------------------------------------------------

func cleanRAM(reason string) {
	var excl uint32
	if sess != nil {
		excl = sess.Pid
	}
	mb, before, after := fullClean(excl)
	addLog(fmt.Sprintf("%s : %d Mo libérés (RAM %d %% → %d %%)", reason, mb, before, after))
	refreshStats()
}

// ---- Zone de notification -------------------------------------------------------

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
	call(pAppendMenuW, menu, MF_STRING, idTrayRam, uintptr(unsafe.Pointer(w("Libérer la RAM"))))
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
	case idTrayRam:
		cleanRAM("RAM nettoyée")
	case idTrayUpdate:
		showMain()
		showPage(5)
		checkForUpdate(true)
	case idTrayQuit:
		quit()
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
	showOverlay(false)
	endBoost(false)
	fpsStop()
	call(pDestroyWindow, hwndMain)
}

// ---- Actions ---------------------------------------------------------------------

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
	case id >= idNav0 && id < idNav0+len(navItems):
		showPage(id - idNav0)
		if id-idNav0 == 5 && updState == updNone {
			checkForUpdate(false)
		}
	case id == idPower:
		setBooster(!boosterOn)
	case func() bool { t, _ := toggleByID(id); return t != nil }():
		t, _ := toggleByID(id)
		t.set(!t.get())
		saveConfig()
		call(pInvalidateRect, buttons[id], 0, 0)
		if id >= idOvToggle0 {
			if overlayVisible {
				renderOverlay()
			}
			call(pInvalidateRect, hwndMain, 0, 0)
		}
	case id == idOvShow:
		showOverlay(!overlayVisible)
		overlayAutoOpen = false
	case id >= idOvPos0 && id < idOvPos0+len(posLabels):
		cfg.OverlayPos = id - idOvPos0
		saveConfig()
		if overlayVisible {
			renderOverlay()
		}
		refreshUI()
	case id == idOptimize:
		n := applyTweaks()
		addLog(fmt.Sprintf("Windows optimisé : %d réglages appliqués", n))
		addLog("Redémarrez le PC une fois pour activer l'accélération GPU")
		call(pInvalidateRect, buttons[id], 0, 0)
	case id == idRam:
		cleanRAM("RAM nettoyée")
	case id == idRestore:
		if msgBox("Remettre Windows exactement comme avant FPS Boost ?\n(désactive aussi le booster et le démarrage automatique)", MB_YESNO|MB_ICONQUESTION) == IDYES {
			restoreAll()
		}
	case id == idAdd:
		if t := strings.TrimSpace(windowText(hEdit)); t != "" {
			addCustom(t)
			call(pSetWindowTextW, hEdit, uintptr(unsafe.Pointer(w(""))))
		} else {
			msgBox("Écrivez le nom du jeu (ex : MonJeu.exe), ou cliquez sur « Jeu ouvert… » pendant que le jeu tourne.", MB_OK|MB_ICONINFO)
		}
	case id == idPick:
		pickGame()
	case id == idRemove || (id == idGames && code == LBN_DBLCLK):
		i := int(call(pSendMessageW, hGames, LB_GETCURSEL, 0, 0))
		if i >= 0 && i < len(cfg.CustomGames) {
			addLog("Jeu retiré : " + cfg.CustomGames[i] + ".exe")
			cfg.CustomGames = append(cfg.CustomGames[:i], cfg.CustomGames[i+1:]...)
			saveConfig()
			refreshGames()
		} else if id == idRemove {
			msgBox("Sélectionnez d'abord un jeu dans la liste.", MB_OK|MB_ICONINFO)
		}
	case id == idOpenLog:
		openPath(dataFile("journal.txt"))
	case id == idClear:
		call(pSendMessageW, hLog, LB_RESETCONTENT, 0, 0)
		logLines = nil
		refreshUI()
	case id == idCheckUpd:
		if pendingUpdate != nil {
			offerUpdate()
		} else {
			checkForUpdate(true)
		}
	case id == idGithub:
		openPath("https://github.com/LogicArt19/fpsboost")
	}
}

// ---- Procédure de fenêtre ----------------------------------------------------------

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
	if handleUpdateMessages(msg, wp, lp) {
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
			visible := call(pIsWindowVisible, hwnd) != 0
			setSensorsWanted(visible || overlayVisible)
			fpsUpdate()
			trayTick++
			if sess != nil && trayTick%3 == 0 {
				trayTip()
			}
			if visible {
				updateSelfUsage()
			}
			if overlayVisible {
				renderOverlay()
			}
			if call(pIsWindowVisible, hwnd) != 0 {
				refreshStats()
			}
		}
		return 0
	case WM_HOTKEY:
		if wp == hotkeyID {
			showOverlay(!overlayVisible)
			overlayAutoOpen = false
		}
		return 0
	case WM_LAUNCHRAM:
		cleanRAM("RAM nettoyée à l'ouverture")
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
			showMain()
			if balloonIsMaj {
				balloonIsMaj = false
				showPage(5)
				offerUpdate()
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
