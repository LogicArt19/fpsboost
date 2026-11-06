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

	// Pages
	pHome = iota - 1
	pDiag
	pSettings
	pOverlay
	pGames
	pApps
	pHistory
	pLog
	pUpdate
)

const (
	idNav0 = 100 // 100..108

	idPower    = 150
	idRam      = 151
	idOptimize = 152
	idRestore  = 153

	idDiagRun = 160
	idDiagFix = 161

	idToggle0 = 200 // 200..2xx

	idOvToggle0 = 250
	idOvShow    = 260
	idOvPos0    = 270

	idEdit   = 300
	idAdd    = 301
	idPick   = 302
	idGames  = 303
	idRemove = 304

	idAppEdit    = 310
	idAppAdd     = 311
	idAppPick    = 312
	idApps       = 313
	idAppRemove  = 314
	idAppToggle0 = 320

	idHistClear = 350

	idLog     = 400
	idOpenLog = 401
	idClear   = 402

	idCheckUpd = 500
	idGithub   = 501

	idTrayOpen   = 600
	idTrayToggle = 601
	idTrayQuit   = 602
	idTrayUpdate = 603
	idTrayRam    = 604
	idTrayOverlay = 605
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
	hInst, hwndMain                                    uintptr
	dpi                                                int32 = 96
	fH1, fSub, fNormal, fBold, fSmall, fStat, fSection uintptr
	fBtn, fPower, fHuge, fIcon, fIconBig, fLogo        uintptr
	brInput                                            uintptr
	hEdit, hGames, hLog, hAppEdit, hApps               uintptr
	buttons                                            = map[int]uintptr{}
	pageCtrls                                          = map[int][]uintptr{}
	curPage                                            int
	iconBig, iconSmall                                 uintptr
	nid                                                NOTIFYICONDATAW
	msgTaskbarCreated                                  uintptr
	startupOn                                          bool
	logLines                                           []string
	trayTick                                           int
	diagDone                                           bool
	pDrawIconEx                                        = modUser32.NewProc("DrawIconEx")
	cSide                                              = color{0x10, 0x12, 0x19}
	cNavActive                                         = color{0x1D, 0x21, 0x2E}
	cWarn                                              = color{0xF5, 0x9E, 0x0B}
	cInfo                                              = color{0x60, 0xA5, 0xFA}
)

func s(v int32) int32 { return v * dpi / 96 }

// ---- Menu et pages -------------------------------------------------------------

var navItems = [][2]string{
	{"", "Accueil"},
	{"", "Diagnostic"},
	{"", "Paramètres"},
	{"", "Overlay"},
	{"", "Jeux"},
	{"", "Applis à fermer"},
	{"", "Historique"},
	{"", "Journal"},
	{"", "Mises à jour"},
}

var pageTitles = [][2]string{
	{"Tableau de bord", "Booster automatique pour tous vos jeux"},
	{"Diagnostic du PC", ""},
	{"Paramètres", "Tout s'applique automatiquement, rien à faire en jeu"},
	{"Overlay en jeu", "Une petite barre par-dessus le jeu : FPS, processeur, carte graphique, RAM, ping"},
	{"Jeux", "FPS Boost détecte vos jeux tout seul. Ajoutez ici ceux qui ne sont pas reconnus."},
	{"Applis à fermer", "Ferme les applis gourmandes au lancement d'un jeu, puis les relance après"},
	{"Historique des parties", "Vos performances partie par partie (parties d'au moins 1 minute)"},
	{"Journal", "Tout ce que fait FPS Boost, en direct"},
	{"Mises à jour", "FPS Boost se met à jour tout seul depuis GitHub"},
}

type toggleDef struct {
	label, desc string
	get         func() bool
	set         func(bool)
}

func tg(label, desc string, p *bool) toggleDef {
	return toggleDef{label, desc, func() bool { return *p }, func(v bool) { *p = v }}
}

var toggles, ovToggles, appToggles []toggleDef

const gameToggleCount = 7

func initToggles() {
	toggles = []toggleDef{
		tg("Alimentation maximale", "Le processeur reste à pleine vitesse pendant la partie", &cfg.PowerPlan),
		tg("Timer système 0,5 ms", "Windows plus réactif, images plus régulières", &cfg.TimerRes),
		tg("RAM anti-saccades", "Vide le cache dès que la mémoire libre manque pendant la partie", &cfg.RamClean),
		tg("Priorité Haute au jeu", "Le jeu passe devant tous les autres programmes", &cfg.GamePriority),
		tg("Arrière-plan ralenti", "Navigateurs, Spotify, OneDrive… en priorité basse pendant la partie", &cfg.BgPriority),
		tg("Pause des tâches de fond", "Indexation et préchargement Windows en pause : moins d'à-coups", &cfg.BgServices),
		tg("Carte graphique dédiée", "Sur les PC à 2 cartes graphiques, force la plus puissante pour le jeu", &cfg.ForceDGPU),
		tg("Alerte de surchauffe", "Prévient si la carte graphique ou le processeur chauffe trop", &cfg.HeatAlert),
		tg("RAM nettoyée à l'ouverture", "Libère la mémoire dès que vous ouvrez FPS Boost", &cfg.RamOnLaunch),
		{"Lancer avec Windows", "Démarre tout seul, discrètement, près de l'horloge",
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
		tg("Booster activé à l'ouverture", "Plus besoin de cliquer sur « Activer le booster »", &cfg.AutoActivate),
		tg("Notifications", "Jeu boosté, surchauffe, mise à jour disponible", &cfg.Notifications),
	}
	ovToggles = []toggleDef{
		tg("Ouvrir automatiquement", "Dès qu'un jeu est détecté (sinon touche Inser)", &cfg.OverlayAuto),
		tg("Températures", "Processeur et carte graphique", &cfg.OverlayTemps),
		tg("1 % low", "Mesure des saccades (FPS des pires images)", &cfg.OverlayLow),
		tg("RAM", "Pourcentage de mémoire utilisée", &cfg.OverlayRAM),
		tg("Ping", "Latence de votre connexion internet", &cfg.OverlayPing),
		tg("Fluidité", "Régularité des images, en %", &cfg.OverlayFluid),
	}
	appToggles = []toggleDef{
		tg("Fermer ces applis en jeu", "Libère processeur et RAM pour la partie", &cfg.CloseAppsOn),
		tg("Les relancer après", "Elles reviennent quand vous quittez le jeu", &cfg.RelaunchApps),
	}
}

var posLabels = []string{"Haut gauche", "Haut droite", "Bas gauche", "Bas droite"}

func toggleByID(id int) (*toggleDef, bool) {
	switch {
	case id >= idToggle0 && id < idToggle0+len(toggles):
		i := id - idToggle0
		return &toggles[i], i == gameToggleCount-1 || i == len(toggles)-1
	case id >= idOvToggle0 && id < idOvToggle0+len(ovToggles):
		i := id - idOvToggle0
		return &ovToggles[i], i >= len(ovToggles)-2
	case id >= idAppToggle0 && id < idAppToggle0+len(appToggles):
		return &appToggles[id-idAppToggle0], true
	}
	return nil, false
}

// ---- Création de la fenêtre ------------------------------------------------------

func mkFont(px, weight int32, face string) uintptr {
	h := int32(-s(px))
	return call(pCreateFontW, uintptr(h), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0,
		uintptr(unsafe.Pointer(w(face))))
}

func createMainWindow() {
	initToggles()
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
	fIconBig = mkFont(20, 400, "Segoe MDL2 Assets")
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
	capc := cSide.ref()
	call(pDwmSetWindowAttribute, hwndMain, 35, uintptr(unsafe.Pointer(&capc)), 4)

	createControls()
	showPage(pHome)
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

func edit(page, id int, x, y, wd int32, cue string) uintptr {
	h := child("EDIT", WS_TABSTOP|ES_AUTOHSCROLL, id, x, y, wd, 20)
	call(pSendMessageW, h, WM_SETFONT, fNormal, 1)
	call(pSendMessageW, h, EM_SETCUEBANNER, 1, uintptr(unsafe.Pointer(w(cue))))
	pageCtrls[page] = append(pageCtrls[page], h)
	return h
}

func createControls() {
	for i := range navItems {
		buttons[idNav0+i] = child("BUTTON", WS_VISIBLE|WS_TABSTOP|BS_OWNERDRAW, idNav0+i, 12, 86+int32(i)*40, 186, 36)
	}
	// Accueil
	button(pHome, idPower, 676, 26, 204, 44)
	button(pHome, idRam, 246, 350, 196, 38)
	button(pHome, idOptimize, 454, 350, 196, 38)
	button(pHome, idRestore, 662, 350, 202, 38)
	// Diagnostic
	button(pDiag, idDiagRun, 246, 548, 220, 34)
	button(pDiag, idDiagFix, 478, 548, 250, 34)
	// Paramètres : 2 colonnes
	for i := range toggles {
		if i < gameToggleCount {
			button(pSettings, idToggle0+i, 244, 122+int32(i)*64, 291, 60)
		} else {
			button(pSettings, idToggle0+i, 575, 122+int32(i-gameToggleCount)*64, 291, 60)
		}
	}
	// Overlay
	button(pOverlay, idOvShow, 246, 196, 230, 38)
	for i := range ovToggles {
		x := int32(246)
		if i%2 == 1 {
			x = 566
		}
		button(pOverlay, idOvToggle0+i, x, 304+int32(i/2)*56, 298, 52)
	}
	for i := range posLabels {
		button(pOverlay, idOvPos0+i, 246+int32(i)*156, 498, 148, 32)
	}
	// Jeux
	hEdit = edit(pGames, idEdit, 258, 133, 346, "Nom du jeu, ex : MonJeu.exe")
	button(pGames, idAdd, 626, 126, 110, 34)
	button(pGames, idPick, 744, 126, 120, 34)
	hGames = listbox(pGames, idGames, LBS_NOTIFY, 252, 276, 606, 242)
	button(pGames, idRemove, 246, 534, 200, 34)
	// Applis à fermer
	button(pApps, idAppToggle0, 246, 120, 300, 52)
	button(pApps, idAppToggle0+1, 566, 120, 298, 52)
	hAppEdit = edit(pApps, idAppEdit, 258, 191, 346, "Nom de l'appli, ex : chrome.exe")
	button(pApps, idAppAdd, 626, 184, 110, 34)
	button(pApps, idAppPick, 744, 184, 120, 34)
	hApps = listbox(pApps, idApps, LBS_NOTIFY, 252, 290, 606, 228)
	button(pApps, idAppRemove, 246, 534, 200, 34)
	// Historique
	button(pHistory, idHistClear, 246, 548, 220, 32)
	// Journal
	hLog = listbox(pLog, idLog, LBS_NOSEL, 252, 108, 606, 422)
	button(pLog, idOpenLog, 246, 546, 220, 32)
	button(pLog, idClear, 476, 546, 120, 32)
	// Mises à jour
	button(pUpdate, idCheckUpd, 250, 236, 250, 40)
	button(pUpdate, idGithub, 512, 236, 180, 40)

	refreshGames()
	refreshApps()
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
	if p == pLog && hLog != 0 {
		if n := call(pSendMessageW, hLog, LB_GETCOUNT, 0, 0); n > 0 {
			call(pSendMessageW, hLog, LB_SETTOPINDEX, n-1, 0)
		}
	}
	if p == pDiag && !diagDone {
		runDiagnostic()
		diagDone = true
	}
	if p == pUpdate && updState == updNone {
		checkForUpdate(false)
	}
	refreshUI()
}

// ---- Dessin -----------------------------------------------------------------------

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
	case pHome:
		paintHomeShapes(g, dot)
	case pDiag:
		card(g, 230, 88, 650, 446)
	case pSettings:
		card(g, 230, 88, 319, 492)
		card(g, 561, 88, 319, 492)
	case pOverlay:
		card(g, 230, 88, 650, 170)
		card(g, 230, 272, 650, 316)
		call(pGdipSetTextRenderingHint, g.g, 5)
		pill(g.g, s(246), s(128), true)
	case pGames:
		card(g, 230, 88, 650, 132)
		box(g, 246, 126, 370, 34)
		card(g, 230, 234, 650, 354)
		box(g, 246, 272, 618, 250)
	case pApps:
		card(g, 230, 88, 650, 150)
		box(g, 246, 184, 370, 34)
		card(g, 230, 252, 650, 336)
		box(g, 246, 286, 618, 236)
	case pHistory:
		card(g, 230, 88, 650, 500)
		g.rectColor(cBorder, s(246), s(120), s(618), 1)
	case pLog:
		card(g, 230, 88, 650, 500)
		box(g, 246, 104, 618, 430)
	case pUpdate:
		card(g, 230, 88, 650, 210)
		card(g, 230, 312, 650, 276)
	}
	g.close()

	// Logo + consommation + statut
	call(pDrawIconEx, mem, uintptr(s(20)), uintptr(s(22)), iconBig, uintptr(s(36)), uintptr(s(36)), 0, 0, 3)
	drawText(mem, "FPS BOOST", fLogo, cText, rc(66, 22, 205, 42), DT_SINGLELINE)
	drawText(mem, "Version "+appVersionStr, fSmall, cMuted, rc(66, 42, 205, 60), DT_SINGLELINE)
	if lastSelfWall.IsZero() {
		updateSelfUsage()
	}
	drawText(mem, "Consommation de FPS Boost", fSmall, cMuted, rc(14, 462, 205, 480), DT_SINGLELINE)
	drawText(mem, strings.Replace(fmt.Sprintf("%.1f %% CPU  ·  %.0f Mo", selfCPU, selfMemMB), ".", ",", 1), fBold, cText, rc(14, 480, 205, 500), DT_SINGLELINE)
	st, st2 := "Booster désactivé", "Cliquez sur Activer"
	if boosterOn && sess != nil {
		st, st2 = "Boost actif", sess.Game
		if fpsValue > 0 {
			st2 += "  ·  " + itoa(fpsValue) + " FPS"
		}
	} else if boosterOn {
		st, st2 = "Booster activé", "En attente d'un jeu…"
	}
	drawText(mem, st, fBold, cText, rc(42, 516, 192, 534), DT_SINGLELINE|DT_END_ELLIPSIS)
	drawText(mem, st2, fSmall, cMuted, rc(24, 540, 192, 576), DT_WORDBREAK|DT_END_ELLIPSIS)

	// En-tête
	subt := pageTitles[curPage][1]
	if curPage == pDiag {
		if n := diagWarnings(); n == 0 {
			subt = "Tout est bon : votre PC est bien réglé pour jouer"
		} else {
			subt = fmt.Sprintf("%d point(s) à améliorer pour gagner des FPS", n)
		}
	}
	drawText(mem, pageTitles[curPage][0], fH1, cText, rc(230, 18, 660, 52), DT_SINGLELINE)
	drawText(mem, subt, fSub, cMuted, rc(231, 52, 880, 72), DT_SINGLELINE|DT_END_ELLIPSIS)

	switch curPage {
	case pHome:
		paintHomeText(mem, title, sub)
	case pDiag:
		paintDiag(mem)
	case pSettings:
		drawText(mem, "PENDANT LE JEU", fSection, cMuted, rc(246, 100, 540, 116), DT_SINGLELINE)
		drawText(mem, "DÉMARRAGE ET NOTIFICATIONS", fSection, cMuted, rc(577, 100, 870, 116), DT_SINGLELINE)
		drawText(mem, "Les réglages « pendant le jeu » s'activent au lancement de chaque jeu et sont retirés dès que vous le quittez : votre PC redevient exactement comme avant.",
			fSmall, cMuted, rc(577, 456, 866, 560), DT_WORDBREAK)
	case pOverlay:
		drawText(mem, "APERÇU", fSection, cMuted, rc(246, 102, 860, 118), DT_SINGLELINE)
		drawText(mem, "Touche  Inser  en jeu", fBold, cText, rc(492, 196, 864, 216), DT_SINGLELINE)
		drawText(mem, "pour ouvrir ou fermer l'overlay", fSmall, cMuted, rc(492, 216, 864, 234), DT_SINGLELINE)
		drawText(mem, "AFFICHAGE", fSection, cMuted, rc(246, 284, 860, 300), DT_SINGLELINE)
		drawText(mem, "POSITION À L'ÉCRAN", fSection, cMuted, rc(246, 478, 860, 494), DT_SINGLELINE)
		drawText(mem, "Visible par-dessus les jeux en « Plein écran fenêtré » ou « Sans bordure ». Les clics passent au travers.",
			fSmall, cMuted, rc(246, 540, 866, 584), DT_WORDBREAK)
	case pGames:
		drawText(mem, "AJOUTER UN JEU", fSection, cMuted, rc(246, 102, 860, 118), DT_SINGLELINE)
		drawText(mem, "Détectés automatiquement : Steam, Epic Games, Riot, Xbox, Ubisoft, EA, GOG, Rockstar, Battle.net, Amazon Games, itch.io et plus de 60 jeux populaires. Cliquez sur « Jeu ouvert… » pendant que le jeu tourne pour le choisir.",
			fSmall, cMuted, rc(246, 168, 866, 214), DT_WORDBREAK)
		drawText(mem, "JEUX AJOUTÉS MANUELLEMENT", fSection, cMuted, rc(246, 248, 860, 264), DT_SINGLELINE)
	case pApps:
		drawText(mem, "FERMER DES APPLIS PENDANT LE JEU", fSection, cMuted, rc(246, 100, 860, 116), DT_SINGLELINE)
		drawText(mem, "APPLIS À FERMER", fSection, cMuted, rc(246, 264, 860, 280), DT_SINGLELINE)
		drawText(mem, "Fermées comme avec la croix, puis de force après 5 s si elles restent ouvertes. Discord n'est jamais fermé tout seul.",
			fSmall, cMuted, rc(460, 532, 866, 572), DT_WORDBREAK)
	case pHistory:
		paintHistory(mem)
	case pUpdate:
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
	g.roundColor(cBorder, s(230), s(158), s(316), s(156), s(12))
	b := gradientBrush(s(230), s(158), s(546), s(314), color{0x1E, 0x18, 0x36}, color{0x10, 0x22, 0x2C})
	g.round(b, s(231), s(159), s(314), s(154), s(11))
	freeBrush(b)
	for i := int32(0); i < 4; i++ {
		card(g, 562+(i%2)*167, 158+(i/2)*86, 151, 70)
	}
	card(g, 230, 330, 650, 76)
	card(g, 230, 420, 650, 166)
}

func paintHomeText(mem uintptr, title, sub string) {
	drawText(mem, title, fBold, cText, rc(272, 88, 560, 144), DT_SINGLELINE|DT_VCENTER|DT_END_ELLIPSIS)
	drawText(mem, sub, fSmall, cMuted, rc(520, 88, 864, 144), DT_SINGLELINE|DT_VCENTER|DT_RIGHT|DT_END_ELLIPSIS)

	fpsTxt, fpsC, fpsSub := "—", cMuted, "Lancez un jeu pour voir vos FPS"
	if sess != nil {
		fpsSub = sess.Game
		switch {
		case fpsValue > 0:
			fpsTxt, fpsC = itoa(fpsValue), cGreen
			if fpsValue < 60 {
				fpsC = cWarn
			}
			parts := []string{}
			if fpsAvg > 0 {
				parts = append(parts, "moy. "+itoa(fpsAvg))
			}
			if fpsLow1 > 0 {
				parts = append(parts, "1 % low "+itoa(fpsLow1))
			}
			if fpsFluid > 0 {
				parts = append(parts, "fluidité "+itoa(fpsFluid)+" %")
			}
			if len(parts) > 0 {
				fpsSub = strings.Join(parts, "  ·  ")
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

	m := memStatus()
	power, powerC := "Normale", cText
	if sess != nil && (sess.PowerChanged || sess.OverlayChanged) {
		power, powerC = "Maximale", cGreen
	}
	ramC := cText
	if m.MemoryLoad >= 85 {
		ramC = cWarn
	}
	sd := sensors()
	comp := func(name string, use, temp float64, useOk, tempOk bool, hot float64) (string, string, color) {
		c := cText
		switch {
		case tempOk:
			if temp >= hot {
				c = cWarn
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

func paintDiag(mem uintptr) {
	for i, d := range diagItems {
		if i >= 9 {
			break
		}
		y := 100 + int32(i)*48
		icon, c := "", cGreen
		switch d.Status {
		case diagWarn:
			icon, c = "", cWarn
		case diagInfo:
			icon, c = "", cInfo
		}
		drawText(mem, icon, fIconBig, c, rc(250, y+4, 278, y+40), DT_SINGLELINE|DT_VCENTER)
		drawText(mem, d.Title, fBold, cText, rc(286, y+4, 866, y+24), DT_SINGLELINE|DT_END_ELLIPSIS)
		drawText(mem, d.Detail, fSmall, cMuted, rc(286, y+24, 866, y+44), DT_SINGLELINE|DT_END_ELLIPSIS)
	}
}

func paintHistory(mem uintptr) {
	cols := []struct {
		x, w  int32
		title string
	}{{246, 84, "DATE"}, {330, 176, "JEU"}, {506, 70, "DURÉE"}, {576, 72, "FPS MOY."}, {648, 72, "1 % LOW"}, {720, 72, "FLUIDITÉ"}, {792, 72, "GPU MAX"}}
	for _, c := range cols {
		drawText(mem, c.title, fSection, cMuted, rc(c.x, 100, c.x+c.w, 116), DT_SINGLELINE)
	}
	if len(history) == 0 {
		drawText(mem, "Aucune partie enregistrée pour l'instant.\nJouez au moins 1 minute avec le booster activé.", fNormal, cMuted, rc(246, 260, 864, 320), DT_CENTER|DT_WORDBREAK)
		return
	}
	temp := func(v int) string {
		if v <= 0 {
			return "—"
		}
		return itoa(v) + " °C"
	}
	num := func(v int) string {
		if v <= 0 {
			return "—"
		}
		return itoa(v)
	}
	maxRows := 19
	if adv := capAdvice(history[len(history)-1]); adv != "" {
		maxRows = 16
		drawText(mem, "\uE82F", fIcon, cWarn, rc(246, 488, 270, 506), DT_SINGLELINE)
		drawText(mem, adv, fSmall, cText, rc(272, 488, 866, 544), DT_WORDBREAK)
	}
	row := 0
	for i := len(history) - 1; i >= 0 && row < maxRows; i-- {
		h := history[i]
		y := 126 + int32(row)*22
		fl := "—"
		if h.Fluid > 0 {
			fl = itoa(h.Fluid) + " %"
		}
		vals := []string{h.Start.Format("02/01 15:04"), h.Game, fmtDur(time.Duration(h.Minutes) * time.Minute), num(h.AvgFPS), num(h.Low1), fl, temp(h.GPUMax)}
		for j, c := range cols {
			col := color{0xC8, 0xCC, 0xD8}
			if j == 3 && h.AvgFPS > 0 {
				col = cGreen
			}
			if j == 5 && h.Fluid > 0 {
				col = fluidColor(h.Fluid)
			}
			drawText(mem, vals[j], fSmall, col, rc(c.x, y, c.x+c.w-6, y+22), DT_SINGLELINE|DT_VCENTER|DT_END_ELLIPSIS)
		}
		row++
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
		st, c = "Impossible de vérifier (pas de connexion internet ?)", cWarn
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
	drawText(mem, "Vos réglages, vos jeux et votre historique sont conservés à chaque mise à jour.", fSmall, cMuted, rc(250, 196, 860, 216), DT_SINGLELINE)
	t, notes := "NOUVEAUTÉS DE CETTE VERSION", releaseNotes
	if pendingUpdate != nil {
		t, notes = "NOUVEAUTÉS DE LA VERSION "+pendingUpdate.Name, pendingUpdate.Notes
	}
	drawText(mem, t, fSection, cMuted, rc(250, 328, 860, 344), DT_SINGLELINE)
	drawText(mem, notes, fNormal, cText, rc(250, 352, 864, 584), DT_WORDBREAK)
}

func buttonText(id int) (icon, text string) {
	switch id {
	case idRam:
		return "", "Libérer la RAM"
	case idOptimize:
		if backup.Applied && backup.TweakVersion >= tweakVersion {
			return "", "Windows optimisé"
		}
		return "", "Optimiser Windows"
	case idRestore:
		return "", "Tout restaurer"
	case idDiagRun:
		return "", "Relancer le diagnostic"
	case idDiagFix:
		return "", "Corriger automatiquement"
	case idAdd, idAppAdd:
		return "", "Ajouter"
	case idPick:
		return "", "Jeu ouvert…"
	case idAppPick:
		return "", "Appli ouverte…"
	case idRemove, idAppRemove:
		return "", "Retirer la sélection"
	case idHistClear:
		return "", "Effacer l'historique"
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
	isNav := id >= idNav0 && id < idNav0+len(navItems)
	switch {
	case isNav:
		bg = cSide
	case id == idPower:
		bg = cBg
	}
	g := newGfx(mem)
	g.rectColor(bg, 0, 0, W, H)
	t, last := toggleByID(id)

	switch {
	case isNav:
		i := id - idNav0
		active := i == curPage
		if active || pressed {
			g.roundColor(cNavActive, 0, 0, W, H, s(8))
		}
		if active {
			b := gradientBrush(0, 0, 0, H, cAccent, cAccent2)
			g.round(b, 0, s(8), s(3), H-s(16), s(2))
			freeBrush(b)
		}
		badge := (i == pUpdate && pendingUpdate != nil) || (i == pDiag && diagDone && diagWarnings() > 0)
		if badge {
			bc := cAccent2
			if i == pDiag {
				bc = cWarn
			}
			g.ellipseColor(bc, W-s(20), H/2-s(4), s(8), s(8))
		}
		g.close()
		c, ic, f := cMuted, cMuted, fNormal
		if active {
			c, ic, f = cText, cAccent2, fBold
		}
		drawText(mem, navItems[i][0], fIcon, ic, RECT{s(12), 0, s(38), H}, DT_SINGLELINE|DT_VCENTER)
		drawText(mem, navItems[i][1], f, c, RECT{s(44), 0, W - s(24), H}, DT_SINGLELINE|DT_VCENTER|DT_END_ELLIPSIS)

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

	case t != nil:
		on := t.get()
		pw, ph := s(44), s(24)
		px, py := W-pw-s(2), (H-ph)/2
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
		drawText(mem, t.label, fBold, cText, RECT{s(2), s(5), W - s(56), s(24)}, DT_SINGLELINE|DT_END_ELLIPSIS)
		drawText(mem, t.desc, fSmall, cMuted, RECT{s(2), s(24), W - s(56), H - s(2)}, DT_WORDBREAK|DT_END_ELLIPSIS)

	default:
		c := cBtn
		if id == idAdd || id == idAppAdd || id == idOvShow || id == idDiagFix ||
			(id == idCheckUpd && pendingUpdate != nil) || id == idOvPos0+cfg.OverlayPos {
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
		if icon != "" {
			drawText(mem, icon, fIcon, cText, RECT{x, off, x + s(20), H + off}, DT_SINGLELINE|DT_VCENTER)
		}
		drawText(mem, label, fBtn, cText, RECT{x + iw, off, W, H + off}, DT_SINGLELINE|DT_VCENTER)
	}

	call(pBitBlt, d.HDC, uintptr(d.RcItem.Left), uintptr(d.RcItem.Top), uintptr(W), uintptr(H), mem, 0, 0, SRCCOPY)
	call(pSelectObject, mem, old)
	call(pDeleteObject, bmp)
	call(pDeleteDC, mem)
}

// ---- Rafraîchissement ----------------------------------------------------------------

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
	if curPage != pHome {
		r = rc(0, 455, sideW, clientH)
	}
	call(pInvalidateRect, hwndMain, uintptr(unsafe.Pointer(&r)), 0)
}

// ---- Listes (jeux / applis) -----------------------------------------------------------

func fillList(h uintptr, items []string) {
	call(pSendMessageW, h, LB_RESETCONTENT, 0, 0)
	for _, g := range items {
		call(pSendMessageW, h, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(w("   "+g+".exe"))))
	}
	if hwndMain != 0 {
		call(pInvalidateRect, hwndMain, 0, 0)
	}
}

func refreshGames() { fillList(hGames, cfg.CustomGames) }
func refreshApps()  { fillList(hApps, cfg.CloseApps) }

func cleanName(name string) string {
	b := baseName(strings.TrimSpace(strings.Trim(name, `"`)))
	if i := strings.LastIndexAny(b, `\/`); i >= 0 {
		b = b[i+1:]
	}
	return b
}

func addTo(list *[]string, name, what string) bool {
	b := cleanName(name)
	if b == "" {
		return false
	}
	for _, g := range *list {
		if g == b {
			return false
		}
	}
	*list = append(*list, b)
	saveConfig()
	addLog(what + " : " + b + ".exe")
	return true
}

func addCustom(name string) {
	if addTo(&cfg.CustomGames, name, "Jeu ajouté") {
		refreshGames()
	}
}

func addCloseApp(name string) {
	if baseName(cleanName(name)) == "discord" {
		msgBox("Discord n'est pas fermé automatiquement, pour garder votre vocal pendant la partie.", MB_OK|MB_ICONINFO)
		return
	}
	if addTo(&cfg.CloseApps, name, "Appli à fermer en jeu") {
		refreshApps()
	}
}

var pickIgnore = toSet("explorer", "applicationframehost", "textinputhost", "systemsettings", "shellexperiencehost",
	"searchhost", "startmenuexperiencehost", "fpsboost", "lockapp", "taskmgr")

func pickRunning(btnID int, empty string, add func(string)) {
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
		msgBox(empty, MB_OK|MB_ICONINFO)
		return
	}
	menu := call(pCreatePopupMenu)
	call(pAppendMenuW, menu, MF_STRING|MF_GRAYED, 0, uintptr(unsafe.Pointer(w("Programmes ouverts :"))))
	call(pAppendMenuW, menu, MF_SEPARATOR, 0, 0)
	for i, n := range names {
		call(pAppendMenuW, menu, MF_STRING, uintptr(idPickBase+i), uintptr(unsafe.Pointer(w(n))))
	}
	var r RECT
	call(pGetWindowRect, buttons[btnID], uintptr(unsafe.Pointer(&r)))
	cmd := int(call(pTrackPopupMenu, menu, TPM_RETURNCMD|TPM_RIGHTBUTTON, uintptr(r.Left), uintptr(r.Bottom), 0, hwndMain, 0))
	call(pDestroyMenu, menu)
	if cmd >= idPickBase && cmd < idPickBase+len(names) {
		add(names[cmd-idPickBase])
	}
}

func removeSelected(h uintptr, list *[]string, what string, refresh func(), fromButton bool) {
	i := int(call(pSendMessageW, h, LB_GETCURSEL, 0, 0))
	if i >= 0 && i < len(*list) {
		addLog(what + " : " + (*list)[i] + ".exe")
		*list = append((*list)[:i], (*list)[i+1:]...)
		saveConfig()
		refresh()
	} else if fromButton {
		msgBox("Sélectionnez d'abord un élément dans la liste.", MB_OK|MB_ICONINFO)
	}
}

// ---- Menu de l'icône ---------------------------------------------------------------------

func trayMenu() {
	menu := call(pCreatePopupMenu)
	add := func(id int, text string) { call(pAppendMenuW, menu, MF_STRING, uintptr(id), uintptr(unsafe.Pointer(w(text)))) }
	add(idTrayOpen, "Ouvrir FPS Boost")
	if boosterOn {
		add(idTrayToggle, "Désactiver le booster")
	} else {
		add(idTrayToggle, "Activer le booster")
	}
	if overlayVisible {
		add(idTrayOverlay, "Masquer l'overlay (Inser)")
	} else {
		add(idTrayOverlay, "Afficher l'overlay (Inser)")
	}
	add(idTrayRam, "Libérer la RAM")
	add(idTrayUpdate, "Rechercher des mises à jour")
	call(pAppendMenuW, menu, MF_SEPARATOR, 0, 0)
	add(idTrayQuit, "Quitter")
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
	case idTrayOverlay:
		showOverlay(!overlayVisible)
		overlayAutoOpen = false
	case idTrayRam:
		cleanRAM("RAM nettoyée")
	case idTrayUpdate:
		showMain()
		showPage(pUpdate)
		checkForUpdate(true)
	case idTrayQuit:
		quit()
	}
}

func quit() {
	showOverlay(false)
	endBoost(false)
	fpsStop()
	call(pDestroyWindow, hwndMain)
}

// ---- Actions ---------------------------------------------------------------------------

func onCommand(id, code int) {
	if t, _ := toggleByID(id); t != nil {
		t.set(!t.get())
		saveConfig()
		call(pInvalidateRect, buttons[id], 0, 0)
		if id >= idOvToggle0 && id < idOvToggle0+len(ovToggles) {
			if overlayVisible {
				renderOverlay()
			}
			call(pInvalidateRect, hwndMain, 0, 0)
		}
		return
	}
	switch {
	case id >= idNav0 && id < idNav0+len(navItems):
		showPage(id - idNav0)
	case id == idPower:
		setBooster(!boosterOn)
	case id == idOptimize:
		n := applyTweaks()
		addLog(fmt.Sprintf("Windows optimisé : %d réglages appliqués", n))
		addLog("Redémarrez le PC une fois pour activer l'accélération GPU")
		diagDone = false
		refreshUI()
	case id == idRam:
		cleanRAM("RAM nettoyée")
	case id == idRestore:
		if msgBox("Remettre Windows exactement comme avant FPS Boost ?\n(désactive aussi le booster et le démarrage automatique)", MB_YESNO|MB_ICONQUESTION) == IDYES {
			restoreAll()
		}
	case id == idDiagRun:
		runDiagnostic()
		refreshUI()
	case id == idDiagFix:
		fixAll()
		refreshUI()
		msgBox("Corrections automatiques appliquées.\nLes autres points (XMP, barrette de RAM, pilote…) se règlent à la main : suivez les conseils affichés.", MB_OK|MB_ICONINFO)
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
	case id == idAdd:
		if t := strings.TrimSpace(windowText(hEdit)); t != "" {
			addCustom(t)
			call(pSetWindowTextW, hEdit, uintptr(unsafe.Pointer(w(""))))
		} else {
			msgBox("Écrivez le nom du jeu (ex : MonJeu.exe), ou cliquez sur « Jeu ouvert… » pendant que le jeu tourne.", MB_OK|MB_ICONINFO)
		}
	case id == idPick:
		pickRunning(idPick, "Lancez votre jeu, puis cliquez à nouveau sur « Jeu ouvert… ».", addCustom)
	case id == idRemove || (id == idGames && code == LBN_DBLCLK):
		removeSelected(hGames, &cfg.CustomGames, "Jeu retiré", refreshGames, id == idRemove)
	case id == idAppAdd:
		if t := strings.TrimSpace(windowText(hAppEdit)); t != "" {
			addCloseApp(t)
			call(pSetWindowTextW, hAppEdit, uintptr(unsafe.Pointer(w(""))))
		} else {
			msgBox("Écrivez le nom de l'appli (ex : chrome.exe), ou cliquez sur « Appli ouverte… ».", MB_OK|MB_ICONINFO)
		}
	case id == idAppPick:
		pickRunning(idAppPick, "Aucune appli ouverte trouvée.", addCloseApp)
	case id == idAppRemove || (id == idApps && code == LBN_DBLCLK):
		removeSelected(hApps, &cfg.CloseApps, "Appli retirée", refreshApps, id == idAppRemove)
	case id == idHistClear:
		if msgBox("Effacer tout l'historique des parties ?", MB_YESNO|MB_ICONQUESTION) == IDYES {
			history = nil
			saveJSON("historique.json", history)
			refreshUI()
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

// ---- Procédure de fenêtre ---------------------------------------------------------------

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
		switch wp {
		case 1:
			tick()
		case 3:
			checkForUpdate(false)
		case 2:
			visible := call(pIsWindowVisible, hwnd) != 0
			lvl := int32(0)
			if visible || overlayVisible {
				lvl = 2
			} else if sess != nil {
				lvl = 1
			}
			setSensorsWanted(lvl)
			fpsUpdate()
			trayTick++
			if sess != nil && trayTick%3 == 0 {
				trayTip()
			}
			if overlayVisible {
				renderOverlay()
			}
			if visible {
				updateSelfUsage()
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
	case WM_HANDOFF:
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
				showPage(pUpdate)
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
