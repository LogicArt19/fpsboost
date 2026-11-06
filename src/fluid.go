package main

// Page Fluidité (3.4) : réglages qui se ressentent vraiment à l'écran

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// ---- Pilote NVIDIA (NVAPI, réglages du profil global du pilote) -----------------------

var (
	modNvapi = syscall.NewLazyDLL("nvapi64.dll")
	pNvQI    = modNvapi.NewProc("nvapi_QueryInterface")
)

const (
	nvInitialize      = 0x0150E828
	nvCreateSession   = 0x0694D52E
	nvLoadSettings    = 0x375DBD6B
	nvSaveSettings    = 0xFCBC7E14
	nvGetBaseProfile  = 0xDA8466A0
	nvSetSetting      = 0x577DD202
	nvGetSetting      = 0x73BF8338
	nvDeleteSetting   = 0xE4A26362
	nvDestroySession  = 0xDAD9CFF8
	nvSettingSize     = 12320
	nvSettingVersion  = nvSettingSize | 1<<16
	nvFrameLimiterID  = 0x10835002 // « Fréquence d'images maximale »
	nvLowLatencyID    = 0x007BA09E // « Mode faible latence »
)

func nvFn(id uint32) uintptr {
	if modNvapi.Load() != nil || pNvQI.Find() != nil {
		return 0
	}
	return call(pNvQI, uintptr(id))
}

func nvCall(id uint32, args ...uintptr) int32 {
	f := nvFn(id)
	if f == 0 {
		return -1
	}
	r, _, _ := syscall.SyscallN(f, args...)
	return int32(r)
}

func hasNvidia() bool {
	if cfg.NvapiBroken {
		return false
	}
	for _, g := range gpuList() {
		if strings.Contains(strings.ToUpper(g.Name), "NVIDIA") {
			return modNvapi.Load() == nil
		}
	}
	return false
}

func hasAMD() bool {
	for _, g := range gpuList() {
		n := strings.ToUpper(g.Name)
		if strings.Contains(n, "AMD") || strings.Contains(n, "RADEON") {
			return true
		}
	}
	return false
}

// Ouvre une session sur le profil global du pilote NVIDIA
func nvSession(fn func(s, p uintptr) bool) bool {
	lock := dataFile("nvapi.lock") // si FPS Boost plante ici, la fonction sera désactivée au prochain lancement
	os.WriteFile(lock, []byte("1"), 0644)
	defer os.Remove(lock)
	if nvCall(nvInitialize) != 0 {
		return false
	}
	var s, p uintptr
	if nvCall(nvCreateSession, uintptr(unsafe.Pointer(&s))) != 0 {
		return false
	}
	defer nvCall(nvDestroySession, s)
	if nvCall(nvLoadSettings, s) != 0 || nvCall(nvGetBaseProfile, s, uintptr(unsafe.Pointer(&p))) != 0 {
		return false
	}
	if !fn(s, p) {
		return false
	}
	return nvCall(nvSaveSettings, s) == 0
}

func nvGet(s, p uintptr, id uint32) (uint32, bool) {
	buf := make([]byte, nvSettingSize)
	binary.LittleEndian.PutUint32(buf[0:], nvSettingVersion)
	if nvCall(nvGetSetting, s, p, uintptr(id), uintptr(unsafe.Pointer(&buf[0]))) != 0 {
		return 0, false
	}
	return binary.LittleEndian.Uint32(buf[8220:]), true
}

func nvSet(s, p uintptr, id, val uint32) bool {
	buf := make([]byte, nvSettingSize)
	binary.LittleEndian.PutUint32(buf[0:], nvSettingVersion)
	binary.LittleEndian.PutUint32(buf[4100:], id)
	binary.LittleEndian.PutUint32(buf[4104:], 0) // DWORD
	binary.LittleEndian.PutUint32(buf[8220:], val)
	return nvCall(nvSetSetting, s, p, uintptr(unsafe.Pointer(&buf[0]))) == 0
}

type nvSaved struct {
	Saved, Existed bool
	Val            uint32
}

// État actuel des deux réglages NVIDIA (lu une fois quand on ouvre la page)
var nvFRL, nvLL uint32
var nvRead bool

func nvRefresh() {
	nvRead = false
	if !hasNvidia() {
		return
	}
	nvSession(func(s, p uintptr) bool {
		nvFRL, _ = nvGet(s, p, nvFrameLimiterID)
		nvLL, _ = nvGet(s, p, nvLowLatencyID)
		nvRead = true
		return false // lecture seule : rien à enregistrer
	})
}

func nvApply(id, val uint32, saved *nvSaved) bool {
	return nvSession(func(s, p uintptr) bool {
		if !saved.Saved {
			v, ok := nvGet(s, p, id)
			*saved = nvSaved{true, ok, v}
			saveBackup()
		}
		return nvSet(s, p, id, val)
	})
}

func nvRestore(id uint32, saved *nvSaved) {
	if !saved.Saved || !hasNvidia() {
		return
	}
	nvSession(func(s, p uintptr) bool {
		if saved.Existed {
			return nvSet(s, p, id, saved.Val)
		}
		return nvCall(nvDeleteSetting, s, p, uintptr(id)) == 0
	})
	*saved = nvSaved{}
}

// ---- Écran ----------------------------------------------------------------------------

func screenHz() (cur, max int) {
	c, b, ok := displayModes()
	if !ok {
		return 0, 0
	}
	return int(c.u32(184)), int(b.u32(184))
}

func recommendedCap() int {
	_, max := screenHz()
	if max <= 0 {
		return 0
	}
	if max <= 60 {
		return 58
	}
	return max - 3
}

// ---- Ouvrir les bons panneaux -------------------------------------------------------------

func openGPUPanel() {
	if hasNvidia() || strings.Contains(strings.ToUpper(firstGPU()), "NVIDIA") {
		p := filepath.Join(os.Getenv("ProgramFiles"), "NVIDIA Corporation", "Control Panel Client", "nvcplui.exe")
		if _, err := os.Stat(p); err == nil {
			openPath(p)
			return
		}
		exec.Command("explorer.exe", `shell:AppsFolder\NVIDIACorp.NVIDIAControlPanel_56jybvy8sckqj!NVIDIACorp.NVIDIAControlPanel`).Start()
		return
	}
	if hasAMD() {
		p := filepath.Join(os.Getenv("ProgramFiles"), "AMD", "CNext", "CNext", "RadeonSoftware.exe")
		if _, err := os.Stat(p); err == nil {
			openPath(p)
			return
		}
	}
	openPath("ms-settings:display-advancedgraphics")
}

func firstGPU() string {
	for _, g := range gpuList() {
		if !strings.Contains(strings.ToUpper(g.Name), "INTEL") {
			return g.Name
		}
	}
	if l := gpuList(); len(l) > 0 {
		return l[0].Name
	}
	return ""
}

// ---- Étapes de la page -------------------------------------------------------------------------

type fluidStep struct {
	Status        int // diagOK / diagWarn / diagInfo
	Title, Detail string
}

func fluidSteps() []fluidStep {
	var st []fluidStep
	cur, max := screenHz()
	if max > cur {
		st = append(st, fluidStep{diagWarn, "1. Écran réglé à " + itoa(cur) + " Hz au lieu de " + itoa(max) + " Hz",
			"C'est la cause n°1 d'un jeu qui paraît saccadé. Un clic pour passer à " + itoa(max) + " Hz."})
	} else {
		st = append(st, fluidStep{diagOK, "1. Écran à sa fréquence maximale : " + itoa(cur) + " Hz",
			"Vérifiez aussi le câble : DisplayPort (ou HDMI 2.0+), branché sur la carte graphique."})
	}
	st = append(st, fluidStep{diagInfo, "2. Activez G-Sync / FreeSync",
		"L'écran attend chaque image au lieu de la couper : la vraie sensation de fluidité. À activer dans le panneau de la carte graphique ET dans le menu de l'écran."})
	if backup.Applied && backup.TweakVersion >= tweakVersion {
		st = append(st, fluidStep{diagOK, "3. Fréquence variable Windows activée",
			"Fluidité adaptative et faible latence aussi pour les jeux en fenêtré / sans bordure."})
	} else {
		st = append(st, fluidStep{diagWarn, "3. Fréquence variable Windows désactivée", "Un clic pour l'activer."})
	}
	capv := recommendedCap()
	switch {
	case hasNvidia() && nvRead && nvFRL > 0:
		st = append(st, fluidStep{diagOK, "4. FPS limités à " + itoa(int(nvFRL)) + " (pilote NVIDIA)",
			"Images parfaitement régulières avec G-Sync. Pour tous vos jeux. Retirable à tout moment."})
	case hasNvidia():
		st = append(st, fluidStep{diagWarn, "4. Limitez les FPS à " + itoa(capv),
			"Avec G-Sync, limiter juste sous la fréquence de l'écran rend chaque image régulière. Appliqué dans le pilote, pour tous les jeux."})
	default:
		st = append(st, fluidStep{diagInfo, "4. Limitez les FPS à " + itoa(capv),
			"Dans les options vidéo du jeu, ou AMD Adrenalin > Jeux > « Frame Rate Target Control »."})
	}
	switch {
	case hasNvidia() && nvRead && nvLL == 1:
		st = append(st, fluidStep{diagOK, "5. Mode faible latence NVIDIA activé", "La souris réagit plus vite, les images arrivent plus tôt."})
	case hasNvidia():
		st = append(st, fluidStep{diagWarn, "5. Mode faible latence NVIDIA désactivé", "Un clic pour l'activer dans le pilote."})
	default:
		st = append(st, fluidStep{diagInfo, "5. Activez AMD Anti-Lag", "AMD Adrenalin > Jeux > Graphismes > « Radeon Anti-Lag »."})
	}
	game := ""
	if len(history) > 0 {
		game = history[len(history)-1].Game
	}
	det := "Activez NVIDIA Reflex « Activé + Boost » (ou AMD Anti-Lag 2) et le mode « Plein écran » dans les options vidéo du jeu."
	if strings.Contains(strings.ToLower(game), "valorant") {
		det = "Valorant > Paramètres > Vidéo : NVIDIA Reflex « Activé + Boost », mode d'affichage « Plein écran », V-Sync désactivé."
	}
	st = append(st, fluidStep{diagInfo, "6. Dans le jeu", det})
	return st
}

func fluidButtonText(id int) (string, string) {
	switch id {
	case idFlHz:
		if cur, max := screenHz(); max > cur {
			return "", "Passer à " + itoa(max) + " Hz"
		}
		return "", "Déjà au maximum"
	case idFlPanel:
		return "", "Ouvrir le panneau"
	case idFlVRR:
		if backup.Applied && backup.TweakVersion >= tweakVersion {
			return "", "Activée"
		}
		return "", "Activer"
	case idFlCap:
		if !hasNvidia() {
			return "", "Ouvrir le panneau"
		}
		if nvRead && nvFRL > 0 {
			return "", "Retirer la limite"
		}
		return "", "Limiter à " + itoa(recommendedCap())
	case idFlLat:
		if !hasNvidia() {
			return "", "Ouvrir le panneau"
		}
		if nvRead && nvLL == 1 {
			return "", "Désactiver"
		}
		return "", "Activer"
	}
	return "", ""
}

func fluidAction(id int) {
	switch id {
	case idFlHz:
		if setBestRefresh() {
			addLog("Écran réglé à sa fréquence maximale")
		}
	case idFlPanel:
		openGPUPanel()
	case idFlVRR:
		n := applyTweaks()
		addLog("Fréquence variable Windows activée (" + itoa(n) + " réglages)")
	case idFlCap:
		if !hasNvidia() {
			openGPUPanel()
			return
		}
		if nvRead && nvFRL > 0 {
			nvRestore(nvFrameLimiterID, &backup.NvFRL)
			saveBackup()
			addLog("Limite de FPS du pilote NVIDIA retirée")
		} else if c := recommendedCap(); c > 0 && nvApply(nvFrameLimiterID, uint32(c), &backup.NvFRL) {
			addLog("FPS limités à " + itoa(c) + " dans le pilote NVIDIA (tous les jeux)")
		} else {
			msgBox("Impossible de régler le pilote NVIDIA automatiquement.\nRéglez « Fréquence d'images maximale » dans le Panneau de configuration NVIDIA.", MB_OK|MB_ICONINFO)
			openGPUPanel()
		}
		nvRefresh()
	case idFlLat:
		if !hasNvidia() {
			openGPUPanel()
			return
		}
		if nvRead && nvLL == 1 {
			nvRestore(nvLowLatencyID, &backup.NvLL)
			saveBackup()
			addLog("Mode faible latence NVIDIA désactivé")
		} else if nvApply(nvLowLatencyID, 1, &backup.NvLL) {
			addLog("Mode faible latence NVIDIA activé")
		} else {
			msgBox("Impossible de régler le pilote NVIDIA automatiquement.\nActivez « Mode faible latence » dans le Panneau de configuration NVIDIA.", MB_OK|MB_ICONINFO)
			openGPUPanel()
		}
		nvRefresh()
	}
	refreshUI()
}

// Au démarrage : si une précédente tentative NVAPI a planté, on désactive la fonction
func checkNvapiCrash() {
	if _, err := os.Stat(dataFile("nvapi.lock")); err == nil {
		os.Remove(dataFile("nvapi.lock"))
		cfg.NvapiBroken = true
		saveConfig()
		addLog("Réglage automatique du pilote NVIDIA désactivé (incompatible avec ce PC)")
	}
}
