package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	appVersion    = 22
	appVersionStr = "2.2"
)

func init() { runtime.LockOSThread() }

// Si une autre version tourne déjà : une plus ancienne est remplacée (mise à jour),
// sinon on affiche simplement la fenêtre existante.
func handleOtherInstance() (updated bool, exit bool) {
	other := call(pFindWindowW, uintptr(unsafe.Pointer(w(className))), 0)
	if other == 0 {
		return false, false
	}
	switch call(pSendMessageW, other, WM_HANDOFF, appVersion, 0) {
	case 2:
		call(pPostMessageW, other, WM_SHOWME, 0, 0)
		return false, true
	case 1:
	default: // version 2.0, sans mise à jour intégrée : on la ferme
		var pid uint32
		call(pGetWindowThreadProcessId, other, uintptr(unsafe.Pointer(&pid)))
		if h, err := syscall.OpenProcess(syscall.PROCESS_TERMINATE|syscall.SYNCHRONIZE, false, pid); err == nil {
			syscall.TerminateProcess(h, 0)
			syscall.WaitForSingleObject(h, 5000)
			syscall.CloseHandle(h)
		}
	}
	for i := 0; i < 50 && call(pFindWindowW, uintptr(unsafe.Pointer(w(className))), 0) != 0; i++ {
		time.Sleep(200 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
	return true, false
}

// Garde la copie utilisée au démarrage de Windows à jour
func refreshStartupCopy() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	target := dataFile("FPSBoost.exe")
	if strings.EqualFold(filepath.Clean(exe), filepath.Clean(target)) {
		return
	}
	for i := 0; i < 10; i++ {
		if copyFile(exe, target) == nil {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func main() {
	// Une seule instance : si FPS Boost tourne déjà, on affiche sa fenêtre
	_, _, err := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(w(`Local\FPSBoost_SingleInstance`))))
	updated := false
	if err == syscall.ERROR_ALREADY_EXISTS {
		var exit bool
		if updated, exit = handleOtherInstance(); exit {
			return
		}
	}

	trayMode, justUpdated := false, false
	for _, a := range os.Args[1:] {
		if strings.EqualFold(a, "--tray") {
			trayMode = true
		}
		if strings.EqualFold(a, "--updated") {
			justUpdated = true
		}
	}

	loadConfig()
	loadBackup()
	enablePrivilege("SeProfileSingleProcessPrivilege")
	enablePrivilege("SeIncreaseBasePriorityPrivilege")
	gdipInit()
	createMainWindow()

	stopTraceSession() // au cas où une ancienne session de mesure serait restée ouverte
	addLog("FPS Boost " + appVersionStr + " démarré")
	go cleanupOldVersion()
	if updated || justUpdated {
		addLog("Mise à jour installée : FPS Boost " + appVersionStr)
		balloon("FPS Boost mis à jour", "Version "+appVersionStr+" installée. Vos réglages ont été conservés.")
	}
	recoverSession()
	if !backup.Applied {
		n := applyTweaks()
		addLog(fmt.Sprintf("Optimisations Windows appliquées (%d réglages)", n))
		addLog("Redémarrez le PC une fois pour profiter de tous les réglages")
	}
	startupOn = startupInstalled()
	if startupOn {
		refreshStartupCopy()
	}
	if cfg.AutoActivate || trayMode {
		setBooster(true)
	} else {
		addLog("Cliquez sur « Activer le booster » pour commencer")
	}

	call(pSetTimer, hwndMain, 1, 2500, 0)
	call(pSetTimer, hwndMain, 2, 1000, 0)
	call(pSetTimer, hwndMain, 3, 6*60*60*1000, 0) // vérifie les mises à jour toutes les 6 h
	checkForUpdate(false)
	if !trayMode {
		showMain()
	}

	var m MSG
	for {
		r := int32(call(pGetMessageW, uintptr(unsafe.Pointer(&m)), 0, 0, 0))
		if r <= 0 {
			break
		}
		call(pTranslateMessage, uintptr(unsafe.Pointer(&m)))
		call(pDispatchMessageW, uintptr(unsafe.Pointer(&m)))
	}
}
