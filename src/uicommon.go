package main

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

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

func windowText(h uintptr) string {
	n := call(pGetWindowTextLengthW, h)
	buf := make([]uint16, n+1)
	call(pGetWindowTextW, h, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
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

func restoreAll() {
	setBooster(false)
	nvRestore(nvFrameLimiterID, &backup.NvFRL)
	nvRestore(nvLowLatencyID, &backup.NvLL)
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

