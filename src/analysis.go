package main

// Analyse automatique de fluidité (3.5) : pendant la partie, FPS Boost note ce qui se passe
// à chaque seconde et quels programmes utilisaient le processeur pendant les saccades.
// À la fin, il explique ce qui limite la fluidité sur CE PC.

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type perfSample struct {
	FPS, Fluid, Stutters int
	CPU, GPU, GPUTemp    float64
	CPUOk, GPUOk, TempOk bool
	RAM                  uint32
}

type Finding struct {
	Severity int // 2 = important, 1 = conseil, 0 = tout va bien
	Title    string
	Detail   string
	App      string // programme à fermer en jeu (bouton)
}

type Analysis struct {
	Game     string
	Date     time.Time
	Findings []Finding
}

var (
	samples      []perfSample
	lastSecStut  int
	culprits     = map[string]float64{}
	procTimes    = map[uint32]uint64{}
	procSampleAt time.Time
	stutSinceProc int
	lastAnalysis *Analysis
)

func loadAnalysis() {
	var a Analysis
	if loadJSON("analyse.json", &a) && a.Game != "" {
		lastAnalysis = &a
	}
}

func resetAnalysis() {
	samples = nil
	culprits = map[string]float64{}
	procTimes = map[uint32]uint64{}
	procSampleAt = time.Time{}
	stutSinceProc = 0
}

// Appelé chaque seconde pendant une partie (jeu à l'écran)
func recordSample() {
	if sess == nil || sess.paused || fpsValue <= 0 {
		return
	}
	sd := sensors()
	samples = append(samples, perfSample{
		FPS: fpsValue, Fluid: fpsFluid, Stutters: lastSecStut,
		CPU: sd.CPUUse, GPU: sd.GPUUse, GPUTemp: sd.GPUTemp,
		CPUOk: sd.CPUUseOk, GPUOk: sd.GPUUseOk, TempOk: sd.GPUTempOk,
		RAM: memStatus().MemoryLoad,
	})
	stutSinceProc += lastSecStut
	if time.Since(procSampleAt) >= 3*time.Second {
		sampleProcesses()
	}
}

// Utilisation du processeur par programme (toutes les 3 s, très léger)
func sampleProcesses() {
	now := time.Now()
	elapsed := now.Sub(procSampleAt).Seconds()
	first := procSampleAt.IsZero()
	procSampleAt = now
	self := uint32(os.Getpid())
	ncpu := float64(runtime.NumCPU())
	cur := map[uint32]uint64{}
	names := map[uint32]string{}
	for _, p := range snapshot() {
		if p.Pid <= 4 || p.Pid == self || (sess != nil && p.Pid == sess.Pid) {
			continue
		}
		h, err := syscall.OpenProcess(0x1000, false, p.Pid)
		if err != nil {
			continue
		}
		var c, e, k, u [2]uint32
		if call(pGetProcessTimes, uintptr(h), uintptr(unsafe.Pointer(&c)), uintptr(unsafe.Pointer(&e)),
			uintptr(unsafe.Pointer(&k)), uintptr(unsafe.Pointer(&u))) != 0 {
			cur[p.Pid] = fileTime(k) + fileTime(u)
			names[p.Pid] = p.Name
		}
		syscall.CloseHandle(h)
	}
	if !first && elapsed > 0 && stutSinceProc > 0 {
		for pid, t := range cur {
			prev, ok := procTimes[pid]
			if !ok || t < prev {
				continue
			}
			pct := 100 * float64(t-prev) / (elapsed * 1e7 * ncpu)
			if pct >= 2 {
				culprits[names[pid]] += pct * float64(stutSinceProc)
			}
		}
	}
	procTimes = cur
	stutSinceProc = 0
}

var ignoreCulprit = toSet("system", "registry", "dwm", "csrss", "audiodg", "svchost", "explorer", "fpsboost",
	"discord", "steam", "steamwebhelper", "vgc", "vgtray", "easyanticheat", "beservice", "nvcontainer",
	"nvdisplay.container", "radeonsoftware", "msmpeng", "searchindexer", "memory compression", "lsass", "services", "wininit")

// Analyse de fin de partie
func analyzeSession(game string) *Analysis {
	n := len(samples)
	if n < 60 {
		return nil
	}
	_, hz := screenHz()
	var sumFPS, sumCPU, sumGPU float64
	var gpuN, cpuN, gpuBusy, ramFull, fluidSum int
	maxTemp := 0.0
	for _, s := range samples {
		sumFPS += float64(s.FPS)
		fluidSum += s.Fluid
		if s.GPUOk {
			gpuN++
			sumGPU += s.GPU
			if s.GPU >= 95 {
				gpuBusy++
			}
		}
		if s.CPUOk {
			cpuN++
			sumCPU += s.CPU
		}
		if s.TempOk && s.GPUTemp > maxTemp {
			maxTemp = s.GPUTemp
		}
		if s.RAM >= 90 {
			ramFull++
		}
	}
	avgFPS := sumFPS / float64(n)
	fluid := fluidSum / n
	var f []Finding
	add := func(sev int, t, d, app string) { f = append(f, Finding{sev, t, d, app}) }

	gpuLimited := gpuN > 0 && float64(gpuBusy)/float64(gpuN) > 0.6
	if gpuLimited && (fluid < 80 || (hz > 0 && avgFPS < float64(hz))) {
		add(2, fmt.Sprintf("Carte graphique à fond %.0f %% du temps", 100*float64(gpuBusy)/float64(gpuN)),
			"C'est elle qui limite. Baissez 1 ou 2 réglages (ombres, effets, anticrénelage) ou activez DLSS / FSR : images bien plus régulières.", "")
	}
	if cpuN > 0 && sumCPU/float64(cpuN) >= 85 {
		add(2, fmt.Sprintf("Processeur saturé (%.0f %% en moyenne)", sumCPU/float64(cpuN)),
			"Fermez les applis en arrière-plan et baissez les réglages liés au processeur (distance de vue, foule, physique).", "")
	}
	// Programme qui tournait pendant les saccades
	type kv struct {
		k string
		v float64
	}
	var list []kv
	for k, v := range culprits {
		if !ignoreCulprit[baseName(k)] {
			list = append(list, kv{k, v})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v })
	closing := map[string]bool{}
	for _, a := range cfg.CloseApps {
		closing[baseName(a)] = true
	}
	for _, c := range list {
		if c.v < 150 || closing[baseName(c.k)] {
			continue
		}
		add(2, prettyName(c.k)+" tournait pendant vos saccades",
			"Ce programme utilisait le processeur au moment des à-coups. Fermez-le pendant vos parties (bouton à droite).", c.k)
		break
	}
	if maxTemp >= 85 {
		add(2, fmt.Sprintf("Carte graphique très chaude (%.0f °C)", maxTemp),
			"Elle ralentit pour se protéger : dépoussiérez le PC, dégagez les aérations, vérifiez les ventilateurs.", "")
	}
	if float64(ramFull)/float64(n) > 0.2 {
		add(2, "Mémoire presque pleine pendant la partie",
			"Fermez des applis (page « Applis à fermer ») ou passez à 16 Go de RAM.", "")
	}
	if hz > 0 && avgFPS < float64(hz)*0.9 && !gpuLimited {
		add(1, fmt.Sprintf("%.0f FPS en moyenne pour un écran %d Hz", avgFPS, hz),
			fmt.Sprintf("Baissez quelques réglages graphiques pour atteindre %d FPS : c'est là que la fluidité est maximale.", hz), "")
	}
	if hz > 0 && avgFPS > float64(hz)*1.15 && fluid < 75 && !(hasNvidia() && nvRead && nvFRL > 0) {
		add(1, fmt.Sprintf("FPS irréguliers au-dessus de %d Hz", hz),
			fmt.Sprintf("Limitez les FPS à %d (étape 4 ci-dessous) : chaque image arrivera à intervalle régulier.", recommendedCap()), "")
	}
	if len(f) == 0 {
		add(0, fmt.Sprintf("Partie fluide : score de fluidité %d %%", fluid),
			"Rien ne limite votre PC. Pour aller plus loin : G-Sync / FreeSync et limite de FPS (étapes ci-dessous).", "")
	}
	sort.SliceStable(f, func(i, j int) bool { return f[i].Severity > f[j].Severity })
	if len(f) > 3 {
		f = f[:3]
	}
	return &Analysis{game, time.Now(), f}
}

func finishAnalysis(game string) {
	a := analyzeSession(game)
	resetAnalysis()
	if a == nil {
		return
	}
	lastAnalysis = a
	saveJSON("analyse.json", a)
	problems := 0
	for _, x := range a.Findings {
		if x.Severity > 0 {
			problems++
			addLog("► Analyse : " + x.Title)
		}
	}
	if problems > 0 && cfg.Notifications {
		balloon("Analyse de fluidité terminée", fmt.Sprintf("%d point(s) à corriger pour %s. Ouvrez la page Fluidité.", problems, game))
	}
	updateCulpritButton()
}

func culpritApp() string {
	if lastAnalysis == nil {
		return ""
	}
	for _, x := range lastAnalysis.Findings {
		if x.App != "" {
			return x.App
		}
	}
	return ""
}

func updateCulpritButton() {
	h := buttons[idFlCulprit]
	if h == 0 {
		return
	}
	if curPage == pFluid && culpritApp() != "" {
		call(pShowWindow, h, SW_SHOW)
	} else {
		call(pShowWindow, h, SW_HIDE)
	}
	call(pInvalidateRect, h, 0, 0)
}

func closeCulprit() {
	app := culpritApp()
	if app == "" {
		return
	}
	addCloseApp(app)
	if !cfg.CloseAppsOn {
		cfg.CloseAppsOn = true
		saveConfig()
	}
	// retire le constat devenu inutile
	var keep []Finding
	for _, x := range lastAnalysis.Findings {
		if !strings.EqualFold(x.App, app) {
			keep = append(keep, x)
		}
	}
	lastAnalysis.Findings = keep
	saveJSON("analyse.json", lastAnalysis)
	updateCulpritButton()
	refreshUI()
	msgBox(prettyName(app)+" sera fermé automatiquement au lancement de vos jeux, puis relancé après.", MB_OK|MB_ICONINFO)
}
