package main

// Fluidité (3.2) : réglages anti-saccades et mesure de la régularité des images

import (
	"fmt"
	"regexp"
	"time"
	"unsafe"
)

// ---- Plan d'alimentation « FPS Boost » réglé pour la fluidité ------------------------
// (uniquement le plan créé par FPS Boost, actif seulement pendant les parties)

const usbSub, usbSuspend = "2a737441-1930-4402-8d77-b2bebba308a3", "48e6b7a6-50f5-4782-a5d4-53bb8f07e226"

func tuneScheme(g string) {
	set := func(sub, setting, v string) {
		run("powercfg", "/setacvalueindex", g, sub, setting, v)
		run("powercfg", "/setdcvalueindex", g, sub, setting, v)
	}
	set("SUB_PROCESSOR", "CPMINCORES", "100")     // aucun cœur mis en veille
	set("SUB_PROCESSOR", "PROCTHROTTLEMIN", "100") // fréquence mini = maxi
	set("SUB_PROCESSOR", "PERFBOOSTMODE", "2")     // turbo agressif
	set("SUB_PROCESSOR", "PERFEPP", "0")           // priorité aux performances
	set("SUB_PCIEXPRESS", "ASPM", "0")             // pas de mise en veille du lien carte graphique
	set(usbSub, usbSuspend, "0")                   // pas de veille USB (souris / clavier)
	set("SUB_DISK", "DISKIDLE", "0")               // le disque ne s'endort pas
	backup.SchemeTuned = 1
	saveBackup()
}

// ---- Mémoire : nettoyage anti-saccades (comme ISLC) ----------------------------------

func memLists() (freeMB, standbyMB uint64, ok bool) {
	var m memListInfo
	var ret uint32
	if call(pNtQuerySystemInformation, 80, uintptr(unsafe.Pointer(&m)), unsafe.Sizeof(m), uintptr(unsafe.Pointer(&ret))) != 0 {
		return 0, 0, false
	}
	var sb uint64
	for _, v := range m.ByPriority {
		sb += uint64(v)
	}
	return (uint64(m.Zero) + uint64(m.Free)) * 4096 >> 20, sb * 4096 >> 20, true
}

// Vide le cache seulement quand la mémoire vraiment libre manque (cause de saccades)
func smartPurge(s *Session) {
	if time.Since(s.lastPurge) < 30*time.Second {
		return
	}
	free, sb, ok := memLists()
	if !ok || free >= 1024 || sb < 512 {
		return
	}
	s.lastPurge = time.Now()
	if mb, ok := purgeStandby(); ok && mb > 0 {
		addLog(fmt.Sprintf("   Anti-saccades : %d Mo de cache libérés (mémoire libre faible)", mb))
	}
}

// ---- Pause des tâches de fond Windows pendant le jeu ---------------------------------

var bgServices = []string{"WSearch", "SysMain"} // indexation et préchargement : activité disque en arrière-plan
var runningRe = regexp.MustCompile(`:\s*4\s`)

func pauseServices(s *Session) {
	if !cfg.BgServices {
		return
	}
	for _, sv := range bgServices {
		out, _ := run("sc", "query", sv)
		if runningRe.MatchString(out) {
			if _, code := run("sc", "stop", sv); code == 0 {
				s.StoppedSvcs = append(s.StoppedSvcs, sv)
			}
		}
	}
	if len(s.StoppedSvcs) > 0 {
		addLog("   Tâches de fond Windows en pause (indexation, préchargement)")
	}
}

func resumeServices(s *Session) {
	for _, sv := range s.StoppedSvcs {
		run("sc", "start", sv)
	}
	s.StoppedSvcs = nil
}

// ---- Mesure de la fluidité ---------------------------------------------------------------

var (
	fpsFluid      int // 1 % low / moyenne, en % (100 = parfaitement régulier)
	fpsStutterMin int // saccades par minute
	sessStutters  int
)

func fluidColor(f int) color {
	switch {
	case f >= 75:
		return cGreen
	case f >= 55:
		return cWarn
	}
	return color{0xEF, 0x44, 0x44}
}

// Conseil de limite de FPS pour des images plus régulières
func capAdvice(h HistEntry) string {
	if h.Fluid == 0 || h.Fluid >= 70 || h.Low1 <= 0 {
		return ""
	}
	hz := 0
	if cur, _, ok := displayModes(); ok {
		hz = int(cur.u32(184))
	}
	limit := (h.Low1 * 115 / 100) / 10 * 10
	if hz > 0 && h.Low1 >= hz-3 {
		limit = hz - 3
	}
	if limit < 30 {
		return ""
	}
	return fmt.Sprintf("Conseil pour %s : limitez les FPS à environ %d dans les options du jeu (ou le panneau NVIDIA / AMD). Les images seront bien plus régulières et la latence plus basse.", h.Game, limit)
}
