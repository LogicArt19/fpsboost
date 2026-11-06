package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var knownGames = toSet(
	"valorant-win64-shipping", "cs2", "csgo", "fortniteclient-win64-shipping", "r5apex", "r5apex_dx12",
	"rocketleague", "gta5", "gta5_enhanced", "playgtav", "league of legends", "overwatch", "cod", "codmw",
	"modernwarfare", "blackopscoldwar", "blackops6", "javaw", "minecraft.windows", "rainbowsix", "rainbowsix_vulkan",
	"robloxplayerbeta", "eldenring", "cyberpunk2077", "dota2", "tslgame", "deadbydaylight-win64-shipping",
	"marvel-win64-shipping", "destiny2", "warframe.x64", "fc25", "fc26", "fifa23", "escapefromtarkov",
	"rustclient", "bf2042", "bf6", "discovery", "helldivers2", "witcher3", "hogwartslegacy",
	"palworld-win64-shipping", "sotgame", "deltaforceclient-win64-shipping", "arc-win64-shipping",
	"pathofexile", "pathofexile_x64", "pathofexilesteam", "wow", "wowclassic", "diablo iv", "starfield",
	"baldur's gate 3", "bg3", "bg3_dx11", "msfs", "flightsimulator", "forzahorizon5", "forzahorizon4",
	"rdr2", "tarkov", "halo infinite", "haloinfinite", "splitgate", "brawlhalla", "smite", "paladins",
)

var gameDirs = []string{
	`\steamapps\common\`, `\epic games\`, `\riot games\`, `\xboxgames\`, `\ubisoft game launcher\games\`,
	`\ea games\`, `\gog galaxy\games\`, `\gog games\`, `\rockstar games\`, `\call of duty\`, `\overwatch\`,
	`\battle.net\`, `\amazon games\library\`, `\itch\apps\`,
}

var excludeRe = regexp.MustCompile(`launcher|crash|report|helper|setup|install|unins|update|redist|dxsetup|cef|webview|anticheat|beservice|^vgc$|vgtray|riotclient|leagueclient|^steam$|epicgames|shadercompil|bugsplat|^battle\.net$|agent|overlay|service|wallpaper|soundpad|lossless|^vr|^obs|server`)

var bgApps = toSet(
	"chrome", "msedge", "firefox", "opera", "opera_gx", "brave", "vivaldi", "onedrive", "teams", "ms-teams",
	"spotify", "steamwebhelper", "epicwebhelper", "eadesktop", "widgets", "phoneexperiencehost", "yourphone",
	"lghub_agent", "icue", "searchhost", "searchapp", "msedgewebview2", "dropbox", "googledrivefs", "adobe desktop service",
	"creative cloud", "skype", "whatsapp", "outlook", "olk",
)

func toSet(items ...string) map[string]bool {
	m := map[string]bool{}
	for _, s := range items {
		m[s] = true
	}
	return m
}

func baseName(n string) string { return strings.TrimSuffix(strings.ToLower(n), ".exe") }

func prettyName(n string) string {
	n = strings.TrimSuffix(n, ".exe")
	n = strings.TrimSuffix(n, ".EXE")
	for _, s := range []string{"-Win64-Shipping", "-win64-shipping", "Client-Win64-Shipping"} {
		n = strings.Replace(n, s, "", 1)
	}
	return n
}

// ---- Session de boost ----------------------------------------------------

type bgRec struct {
	Pid  uint32
	Name string
	Prio uint32
}

type Session struct {
	Game           string
	Exe            string
	Pid            uint32
	Start          time.Time
	PrevScheme     string
	PowerChanged   bool
	PrevOverlay    GUID
	OverlayChanged bool
	TimerSet       bool
	Bg             []bgRec

	StoppedSvcs []string
	Closed    []closedApp
	toKill    []procEntry
	killDue   time.Time
	cpuMax    float64
	gpuMax    float64
	prioDue   time.Time
	prioDone  bool
	lastBg    time.Time
	lastPurge time.Time
	handle    syscall.Handle
}

var (
	boosterOn bool
	sess      *Session
	pathCache = map[uint32]string{}
)

func cachedPath(p procEntry) string {
	key := p.Pid
	if v, ok := pathCache[key]; ok && strings.HasSuffix(strings.ToLower(v), strings.ToLower(p.Name)) {
		return v
	}
	v := procPath(p.Pid)
	pathCache[key] = v
	return v
}

func findGame() *procEntry {
	procs := snapshot()
	self := uint32(os.Getpid())
	custom := map[string]bool{}
	for _, g := range cfg.CustomGames {
		custom[baseName(g)] = true
	}
	alive := map[uint32]bool{}
	var cands []procEntry
	for _, p := range procs {
		alive[p.Pid] = true
		if p.Pid == self || p.Pid == 0 {
			continue
		}
		b := baseName(p.Name)
		if knownGames[b] || custom[b] {
			pp := p
			return &pp
		}
		if excludeRe.MatchString(b) {
			continue
		}
		path := strings.ToLower(cachedPath(p))
		if path == "" {
			continue
		}
		for _, d := range gameDirs {
			if strings.Contains(path, d) {
				cands = append(cands, p)
				break
			}
		}
	}
	for pid := range pathCache {
		if !alive[pid] {
			delete(pathCache, pid)
		}
	}
	if len(cands) == 0 {
		return nil
	}
	wins := windowPids()
	for _, c := range cands {
		if wins[c.Pid] {
			cc := c
			return &cc
		}
	}
	return nil
}

func gameAlive(s *Session) bool {
	if s.handle != 0 {
		r, _ := syscall.WaitForSingleObject(s.handle, 0)
		return r == syscall.WAIT_TIMEOUT
	}
	for _, p := range snapshot() {
		if p.Pid == s.Pid && strings.EqualFold(p.Name, s.Exe) {
			return true
		}
	}
	return false
}

func lowerBackground(s *Session) int {
	done := map[uint32]bool{}
	for _, b := range s.Bg {
		done[b.Pid] = true
	}
	n := 0
	for _, p := range snapshot() {
		if done[p.Pid] || !bgApps[baseName(p.Name)] {
			continue
		}
		cur, ok := getPriority(p.Pid)
		if !ok || cur != prioNormal {
			continue
		}
		if setPriority(p.Pid, prioBelowNormal) {
			s.Bg = append(s.Bg, bgRec{p.Pid, p.Name, cur})
			n++
		}
	}
	return n
}

func boostPower(s *Session) {
	prev := activeScheme()
	perf := perfScheme()
	if perf != "" && perf == prev {
		addLog("   Alimentation déjà en performances maximales")
		return
	}
	if perf != "" && setScheme(perf) {
		s.PrevScheme = prev
		s.PowerChanged = true
		addLog("   Alimentation : performances maximales")
		return
	}
	if cur, ok := getOverlay(); ok && setOverlay(bestPerfOverlay) {
		s.PrevOverlay = cur
		s.OverlayChanged = true
		addLog("   Mode d'alimentation : meilleures performances")
	}
}

func startBoost(p *procEntry) {
	now := time.Now()
	s := &Session{Game: prettyName(p.Name), Exe: p.Name, Pid: p.Pid, Start: now}
	if h, err := syscall.OpenProcess(syscall.SYNCHRONIZE|0x1000, false, p.Pid); err == nil {
		s.handle = h
	}
	addLog("► Jeu détecté : " + s.Game)
	fpsStart(p.Pid)
	forceDedicatedGPU(p.Pid, s.Game)
	closeApps(s)
	pauseServices(s)
	if cfg.PowerPlan {
		boostPower(s)
	}
	if cfg.TimerRes {
		r := setTimer(true)
		s.TimerSet = true
		addLog(fmt.Sprintf("   Timer système : %s ms", fmtMs(r)))
	}
	if cfg.RamClean {
		if mb, ok := purgeStandby(); ok {
			addLog(fmt.Sprintf("   RAM optimisée : %d Mo de cache libérés", mb))
		}
	}
	if cfg.BgPriority {
		if n := lowerBackground(s); n > 0 {
			addLog(fmt.Sprintf("   %d applis d'arrière-plan en priorité basse", n))
		}
	}
	s.prioDue = now.Add(6 * time.Second)
	s.prioDone = !cfg.GamePriority
	s.lastBg, s.lastPurge = now, now
	sess = s
	saveJSON("session.json", s)
	sessStutters = 0
	cfg.GamesBoosted++
	saveConfig()
	addLog("✔ BOOST ACTIF sur " + s.Game)
	if cfg.OverlayAuto && !overlayVisible {
		showOverlay(true)
		overlayAutoOpen = true
	}
	if cfg.Notifications {
		balloon("Boost activé : "+s.Game, "FPS Boost optimise votre PC pendant la partie.")
	}
	refreshUI()
}

func endBoost(quiet bool) {
	s := sess
	if s == nil {
		return
	}
	if s.PowerChanged && s.PrevScheme != "" {
		setScheme(s.PrevScheme)
	}
	if s.OverlayChanged {
		setOverlay(s.PrevOverlay)
	}
	if s.TimerSet {
		setTimer(false)
	}
	for _, b := range s.Bg {
		for _, p := range snapshot() {
			if p.Pid == b.Pid && strings.EqualFold(p.Name, b.Name) {
				setPriority(b.Pid, b.Prio)
				break
			}
		}
	}
	if s.handle != 0 {
		syscall.CloseHandle(s.handle)
	}
	if overlayAutoOpen {
		overlayAutoOpen = false
		showOverlay(false)
	}
	fpsAvgFinal := fpsAvg
	low1 := sessionLow1()
	fpsStop()
	relaunchApps(s)
	resumeServices(s)
	fluidFinal := 0
	if fpsAvgFinal > 0 && low1 > 0 {
		fluidFinal = clampInt(100*low1/fpsAvgFinal, 0, 100)
	}
	if !quiet && time.Since(s.Start) >= time.Minute {
		mins := int(time.Since(s.Start).Minutes())
		stm := 0
		if mins > 0 {
			stm = sessStutters / mins
		}
		addHistory(HistEntry{s.Game, s.Start, mins, fpsAvgFinal, low1, int(s.cpuMax), int(s.gpuMax), fluidFinal, stm})
		if adv := capAdvice(history[len(history)-1]); adv != "" {
			addLog("► " + adv)
		}
	}
	os.Remove(dataFile("session.json"))
	sess = nil
	if !quiet {
		if fpsAvgFinal > 0 {
			addLog(fmt.Sprintf("■ %s fermé (%s de jeu, %d FPS en moyenne, 1 %% low %d)", s.Game, fmtDur(time.Since(s.Start)), fpsAvgFinal, low1))
		} else {
			addLog(fmt.Sprintf("■ %s fermé (%s de jeu)", s.Game, fmtDur(time.Since(s.Start))))
		}
		addLog("   PC remis en mode normal")
	}
	refreshUI()
}

// Si l'appli a été coupée brutalement pendant un jeu, on remet tout en place
func recoverSession() {
	var s Session
	if !loadJSON("session.json", &s) {
		return
	}
	resumeServices(&s)
	sess = &s
	endBoost(true)
	addLog("Réglages de la session précédente restaurés")
}

func tick() {
	if !boosterOn {
		return
	}
	if sess != nil {
		if !gameAlive(sess) {
			endBoost(false)
			addLog("En attente d'un jeu…")
			return
		}
		now := time.Now()
		if !sess.killDue.IsZero() && now.After(sess.killDue) {
			sess.killDue = time.Time{}
			killLeftovers(sess)
		}
		sd := sensors()
		if sd.CPUTempOk && sd.CPUTemp > sess.cpuMax {
			sess.cpuMax = sd.CPUTemp
		}
		if sd.GPUTempOk && sd.GPUTemp > sess.gpuMax {
			sess.gpuMax = sd.GPUTemp
		}
		checkHeat()
		if !sess.prioDone && now.After(sess.prioDue) {
			sess.prioDone = true
			if setPriority(sess.Pid, prioHigh) {
				addLog("   Priorité du jeu : Haute")
			} else {
				addLog("   Priorité du jeu gérée par son anti-triche")
			}
		}
		if cfg.BgPriority && now.Sub(sess.lastBg) > 30*time.Second {
			sess.lastBg = now
			if lowerBackground(sess) > 0 {
				saveJSON("session.json", sess)
			}
		}
		if cfg.RamClean {
			smartPurge(sess)
		}
		return
	}
	if g := findGame(); g != nil {
		startBoost(g)
	}
}

func setBooster(on bool) {
	if on == boosterOn {
		return
	}
	boosterOn = on
	if on {
		addLog("Booster activé — en attente d'un jeu…")
		tick()
	} else {
		endBoost(false)
		addLog("Booster désactivé")
	}
	refreshUI()
}

func fmtMs(v float64) string { return strings.Replace(fmt.Sprintf("%.2f", v), ".", ",", 1) }

func fmtDur(d time.Duration) string {
	m := int(d.Minutes())
	if m < 60 {
		return fmt.Sprintf("%d min", m)
	}
	return fmt.Sprintf("%dh%02d", m/60, m%60)
}
