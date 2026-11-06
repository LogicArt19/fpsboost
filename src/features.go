package main

// Fonctions de la 3.0 : diagnostic, historique, fermeture d'applis, carte graphique dédiée, ping

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	modIphlp = syscall.NewLazyDLL("iphlpapi.dll")

	pEnumDisplaySettingsW     = modUser32.NewProc("EnumDisplaySettingsW")
	pChangeDisplaySettingsExW = modUser32.NewProc("ChangeDisplaySettingsExW")
	pGetSystemFirmwareTable   = modKernel32.NewProc("GetSystemFirmwareTable")
	pGetSystemPowerStatus     = modKernel32.NewProc("GetSystemPowerStatus")
	pGetDiskFreeSpaceExW      = modKernel32.NewProc("GetDiskFreeSpaceExW")
	pTerminateProcess         = modKernel32.NewProc("TerminateProcess")
	pIcmpCreateFile           = modIphlp.NewProc("IcmpCreateFile")
	pIcmpSendEcho             = modIphlp.NewProc("IcmpSendEcho")
)

// ---- Écran -------------------------------------------------------------------

type devMode [220]byte

func (d *devMode) u32(off int) uint32 { return binary.LittleEndian.Uint32(d[off:]) }

func displayModes() (cur devMode, best devMode, ok bool) {
	cur[68] = 220
	if call(pEnumDisplaySettingsW, 0, ^uintptr(0), uintptr(unsafe.Pointer(&cur[0]))) == 0 { // ENUM_CURRENT_SETTINGS
		return
	}
	best = cur
	for i := uintptr(0); i < 2000; i++ {
		var m devMode
		m[68] = 220
		if call(pEnumDisplaySettingsW, 0, i, uintptr(unsafe.Pointer(&m[0]))) == 0 {
			break
		}
		if m.u32(172) == cur.u32(172) && m.u32(176) == cur.u32(176) && m.u32(168) == cur.u32(168) &&
			m.u32(184) > best.u32(184) {
			best = m
		}
	}
	return cur, best, true
}

func setBestRefresh() bool {
	cur, best, ok := displayModes()
	if !ok || best.u32(184) <= cur.u32(184) {
		return false
	}
	binary.LittleEndian.PutUint32(best[72:], 0x80000|0x100000|0x400000|0x40000) // largeur, hauteur, fréquence, bits
	return call(pChangeDisplaySettingsExW, 0, uintptr(unsafe.Pointer(&best[0])), 0, 1, 0) == 0
}

// ---- Mémoire (SMBIOS) ------------------------------------------------------

type ramInfo struct {
	Sticks   int
	Speed    int
	Type     string
	Ok       bool
}

func readRAM() ramInfo {
	var r ramInfo
	if pGetSystemFirmwareTable.Find() != nil {
		return r
	}
	sig := uintptr(0x52534D42) // 'RSMB'
	n := call(pGetSystemFirmwareTable, sig, 0, 0, 0)
	if n == 0 {
		return r
	}
	buf := make([]byte, n)
	if call(pGetSystemFirmwareTable, sig, 0, uintptr(unsafe.Pointer(&buf[0])), n) == 0 || len(buf) < 8 {
		return r
	}
	ln := int(binary.LittleEndian.Uint32(buf[4:]))
	data := buf[8:]
	if ln < len(data) {
		data = data[:ln]
	}
	for i := 0; i+4 <= len(data); {
		t, l := data[i], int(data[i+1])
		if l < 4 || i+l > len(data) {
			break
		}
		st := data[i : i+l]
		if t == 17 && l >= 0x17 {
			size := binary.LittleEndian.Uint16(st[0x0C:])
			if size != 0 && size != 0xFFFF {
				r.Sticks++
				switch st[0x12] {
				case 0x1A:
					r.Type = "DDR4"
				case 0x22:
					r.Type = "DDR5"
				case 0x18:
					r.Type = "DDR3"
				case 0x1E, 0x23:
					r.Type = "LPDDR"
				}
				sp := int(binary.LittleEndian.Uint16(st[0x15:]))
				if l >= 0x22 {
					if c := int(binary.LittleEndian.Uint16(st[0x20:])); c > 0 && c != 0xFFFF {
						sp = c
					}
				}
				if sp > r.Speed {
					r.Speed = sp
				}
				r.Ok = true
			}
		}
		if t == 127 {
			break
		}
		j := i + l
		for j+1 < len(data) && !(data[j] == 0 && data[j+1] == 0) {
			j++
		}
		i = j + 2
	}
	return r
}

// ---- Cartes graphiques (registre) -----------------------------------------------

type gpuInfo struct {
	Name   string
	Driver time.Time
}

var gpuCache []gpuInfo

func gpuList() []gpuInfo {
	if gpuCache != nil {
		return gpuCache
	}
	base := `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}\`
	list := []gpuInfo{}
	for i := 0; i < 10; i++ {
		k := base + fmt.Sprintf("%04d", i)
		_, _, name, ok := regGet(HKLM, k, "DriverDesc")
		if !ok || name == "" || strings.Contains(strings.ToLower(name), "microsoft") || strings.Contains(strings.ToLower(name), "virtual") {
			continue
		}
		g := gpuInfo{Name: name}
		if _, _, d, ok := regGet(HKLM, k, "DriverDate"); ok {
			p := strings.Split(d, "-")
			if len(p) == 3 {
				m, _ := strconv.Atoi(p[0])
				dd, _ := strconv.Atoi(p[1])
				y, _ := strconv.Atoi(p[2])
				g.Driver = time.Date(y, time.Month(m), dd, 0, 0, 0, 0, time.Local)
			}
		}
		list = append(list, g)
	}
	gpuCache = list
	return list
}

// ---- Alimentation / disque ---------------------------------------------------------

type powerStatus struct {
	ACLine, BatteryFlag, Percent, Reserved uint8
	LifeTime, FullLifeTime                 uint32
}

func power() (laptop, onBattery bool) {
	var p powerStatus
	if call(pGetSystemPowerStatus, uintptr(unsafe.Pointer(&p))) == 0 {
		return
	}
	laptop = p.BatteryFlag != 128 && p.BatteryFlag != 255
	return laptop, laptop && p.ACLine == 0
}

func diskFreeGB(root string) float64 {
	var free, total, tfree uint64
	call(pGetDiskFreeSpaceExW, uintptr(unsafe.Pointer(w(root))), uintptr(unsafe.Pointer(&free)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&tfree)))
	return float64(free) / (1 << 30)
}

// ---- Diagnostic ---------------------------------------------------------------------

const (
	diagOK = iota
	diagWarn
	diagInfo
)

type diagItem struct {
	Status        int
	Title, Detail string
	Fixable       bool
}

var diagItems []diagItem

func runDiagnostic() {
	var d []diagItem
	add := func(st int, t, det string, fix bool) { d = append(d, diagItem{st, t, det, fix}) }

	// Écran
	if cur, best, ok := displayModes(); ok {
		c, b := cur.u32(184), best.u32(184)
		if b > c {
			add(diagWarn, fmt.Sprintf("Écran réglé à %d Hz alors qu'il peut aller à %d Hz", c, b),
				"Gros gain de fluidité : cliquez sur « Corriger automatiquement ».", true)
		} else {
			add(diagOK, fmt.Sprintf("Écran à sa fréquence maximale (%d Hz)", c), fmt.Sprintf("Résolution %d × %d", cur.u32(172), cur.u32(176)), false)
		}
	}

	// Mémoire
	m := memStatus()
	gb := float64(m.TotalPhys) / (1 << 30)
	ram := readRAM()
	laptop, onBattery := power()
	switch {
	case ram.Ok && ram.Sticks == 1:
		add(diagWarn, fmt.Sprintf("Mémoire : %.0f Go sur une seule barrette", gb),
			"Ajoutez une 2e barrette identique (double canal) : souvent +10 à +25 % de FPS.", false)
	case gb < 15:
		add(diagWarn, fmt.Sprintf("Mémoire : %.0f Go", gb), "16 Go sont recommandés pour les jeux récents.", false)
	default:
		det := "Quantité suffisante pour jouer"
		if ram.Ok && ram.Sticks >= 2 {
			det = fmt.Sprintf("%d barrettes, double canal", ram.Sticks)
		}
		add(diagOK, fmt.Sprintf("Mémoire : %.0f Go", gb), det, false)
	}
	if ram.Ok && ram.Speed > 0 {
		slow := (ram.Type == "DDR4" && ram.Speed <= 2666) || (ram.Type == "DDR5" && ram.Speed <= 4800)
		t := strings.TrimSpace(fmt.Sprintf("%s à %d MT/s", ram.Type, ram.Speed))
		switch {
		case slow && !laptop:
			add(diagWarn, "Vitesse de la RAM : "+t, "La RAM tourne à sa vitesse de base : activez le profil XMP / EXPO dans le BIOS.", false)
		case slow:
			add(diagInfo, "Vitesse de la RAM : "+t, "Normal sur un PC portable (non modifiable).", false)
		default:
			add(diagOK, "Vitesse de la RAM : "+t, "La RAM tourne à bonne vitesse", false)
		}
	}

	// Cartes graphiques
	gpus := gpuList()
	for _, g := range gpus {
		if g.Driver.IsZero() {
			continue
		}
		age := time.Since(g.Driver)
		det := "Pilote du " + g.Driver.Format("02/01/2006")
		if age > 180*24*time.Hour {
			add(diagWarn, g.Name+" : pilote ancien", det+" : mettez-le à jour (NVIDIA App, AMD Adrenalin ou Intel). Gain souvent réel.", false)
		} else {
			add(diagOK, g.Name, det+" : à jour", false)
		}
	}
	if len(gpus) > 1 {
		if cfg.ForceDGPU {
			add(diagOK, fmt.Sprintf("%d cartes graphiques détectées", len(gpus)), "FPS Boost force la carte graphique dédiée pour vos jeux", false)
		} else {
			add(diagWarn, fmt.Sprintf("%d cartes graphiques détectées", len(gpus)), "Activez « Forcer la carte graphique dédiée » dans Paramètres.", false)
		}
	}

	// Alimentation
	if laptop {
		if onBattery {
			add(diagWarn, "PC portable sur batterie", "Branchez le chargeur : sur batterie, les FPS chutent fortement.", false)
		} else {
			add(diagOK, "PC portable branché sur secteur", "Toujours jouer branché pour garder toute la puissance", false)
		}
	}

	// Réglages Windows
	if backup.Applied && backup.TweakVersion >= tweakVersion {
		add(diagOK, "Réglages Windows optimisés", "Mode Jeu, Game DVR coupé, priorités jeux, accélération GPU…", false)
	} else {
		add(diagWarn, "Réglages Windows non optimisés", "Cliquez sur « Corriger automatiquement ».", true)
	}

	// Disque
	sys := os.Getenv("SystemDrive") + `\`
	if free := diskFreeGB(sys); free < 20 {
		add(diagWarn, fmt.Sprintf("Disque système presque plein (%.0f Go libres)", free), "Libérez au moins 20 Go pour éviter les ralentissements.", false)
	} else {
		add(diagOK, fmt.Sprintf("Disque système : %.0f Go libres", free), "Espace suffisant", false)
	}

	sort.SliceStable(d, func(i, j int) bool { return d[i].Status == diagWarn && d[j].Status != diagWarn })
	diagItems = d
}

func diagWarnings() int {
	n := 0
	for _, d := range diagItems {
		if d.Status == diagWarn {
			n++
		}
	}
	return n
}

func fixAll() {
	if setBestRefresh() {
		addLog("Écran réglé à sa fréquence maximale")
	}
	n := applyTweaks()
	addLog(fmt.Sprintf("Réglages Windows optimisés (%d)", n))
	runDiagnostic()
}

// ---- Carte graphique dédiée pour le jeu ------------------------------------------

const gpuPrefKey = `Software\Microsoft\DirectX\UserGpuPreferences`

func forceDedicatedGPU(pid uint32, game string) {
	if !cfg.ForceDGPU || len(gpuList()) < 2 {
		return
	}
	path := procPath(pid)
	if path == "" {
		return
	}
	if _, _, _, exists := regGet(HKCU, gpuPrefKey, path); exists {
		return
	}
	setTracked("HKCU", gpuPrefKey, path, "sz", 0, "GpuPreference=2;")
	saveBackup()
	addLog("   Carte graphique dédiée forcée pour " + game + " (actif au prochain lancement)")
}

// ---- Historique ----------------------------------------------------------------------

type HistEntry struct {
	Game    string
	Start   time.Time
	Minutes int
	AvgFPS  int
	Low1    int
	CPUMax  int
	GPUMax  int
}

var history []HistEntry

func loadHistory() { loadJSON("historique.json", &history) }

func addHistory(e HistEntry) {
	history = append(history, e)
	if len(history) > 100 {
		history = history[len(history)-100:]
	}
	saveJSON("historique.json", history)
}

// ---- Fermeture d'applis au lancement du jeu --------------------------------------

type closedApp struct {
	Name, Path string
}

var enumWins []struct{ h uintptr; pid uint32 }
var enumAllCb = syscall.NewCallback(func(h, l uintptr) uintptr {
	if call(pIsWindowVisible, h) != 0 && call(pGetWindow, h, 4) == 0 {
		var pid uint32
		call(pGetWindowThreadProcessId, h, uintptr(unsafe.Pointer(&pid)))
		enumWins = append(enumWins, struct {
			h   uintptr
			pid uint32
		}{h, pid})
	}
	return 1
})

func closeApps(s *Session) {
	if !cfg.CloseAppsOn || len(cfg.CloseApps) == 0 {
		return
	}
	want := map[string]bool{}
	for _, a := range cfg.CloseApps {
		want[baseName(a)] = true
	}
	enumWins = nil
	call(pEnumWindows, enumAllCb, 0)
	seenPath := map[string]bool{}
	n := 0
	for _, p := range snapshot() {
		if p.Pid == s.Pid || !want[baseName(p.Name)] {
			continue
		}
		if path := procPath(p.Pid); path != "" && !seenPath[strings.ToLower(path)] {
			seenPath[strings.ToLower(path)] = true
			s.Closed = append(s.Closed, closedApp{p.Name, path})
			n++
		}
		s.toKill = append(s.toKill, procEntry{p.Pid, p.Name})
		for _, wn := range enumWins {
			if wn.pid == p.Pid {
				call(pPostMessageW, wn.h, WM_CLOSE, 0, 0)
			}
		}
	}
	if n > 0 {
		s.killDue = time.Now().Add(5 * time.Second)
		addLog(fmt.Sprintf("   %d applis fermées pour la partie", n))
	}
}

func killLeftovers(s *Session) {
	alive := map[uint32]string{}
	for _, p := range snapshot() {
		alive[p.Pid] = p.Name
	}
	for _, k := range s.toKill {
		if name, ok := alive[k.Pid]; ok && strings.EqualFold(name, k.Name) {
			if h, err := syscall.OpenProcess(0x0001, false, k.Pid); err == nil {
				call(pTerminateProcess, uintptr(h), 0)
				syscall.CloseHandle(h)
			}
		}
	}
	s.toKill = nil
}

func relaunchApps(s *Session) {
	if !cfg.RelaunchApps || len(s.Closed) == 0 {
		return
	}
	for _, a := range s.Closed {
		// via l'explorateur : l'appli se relance normalement (sans droits administrateur)
		cmd := exec.Command("explorer.exe", a.Path)
		cmd.Start()
	}
	addLog(fmt.Sprintf("   %d applis relancées", len(s.Closed)))
}

// ---- Ping (latence internet) -------------------------------------------------------

var pingMs int32 = -1
var overlayOn int32

func startPinger() {
	go func() {
		if modIphlp.Load() != nil {
			return
		}
		h := call(pIcmpCreateFile)
		if h == 0 || h == ^uintptr(0) {
			return
		}
		data := []byte("fpsboost")
		reply := make([]byte, 64)
		for {
			time.Sleep(2 * time.Second)
			if atomic.LoadInt32(&overlayOn) == 0 || !cfg.OverlayPing {
				continue
			}
			best := int32(-1)
			for _, ip := range []uint32{0x01010101, 0x08080808} { // 1.1.1.1 puis 8.8.8.8
				n := call(pIcmpSendEcho, h, uintptr(ip), uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 0,
					uintptr(unsafe.Pointer(&reply[0])), uintptr(len(reply)), 1000)
				if n > 0 && binary.LittleEndian.Uint32(reply[4:]) == 0 {
					best = int32(binary.LittleEndian.Uint32(reply[8:]))
					break
				}
			}
			atomic.StoreInt32(&pingMs, best)
		}
	}()
}

// ---- Alerte de surchauffe ------------------------------------------------------------

var lastHeatAlert time.Time

func checkHeat() {
	if !cfg.HeatAlert || time.Since(lastHeatAlert) < 10*time.Minute {
		return
	}
	sd := sensors()
	msg := ""
	if sd.GPUTempOk && sd.GPUTemp >= 90 {
		msg = fmt.Sprintf("Carte graphique à %.0f °C", sd.GPUTemp)
	} else if sd.CPUTempOk && sd.CPUTemp >= 95 {
		msg = fmt.Sprintf("Processeur à %.0f °C", sd.CPUTemp)
	}
	if msg != "" {
		lastHeatAlert = time.Now()
		addLog("⚠ Surchauffe : " + msg)
		balloon("Attention : surchauffe", msg+". Vérifiez la ventilation de votre PC.")
	}
}
