package main

import (
	"encoding/binary"
	"encoding/json"
	"html"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// ---- Dossiers / fichiers -------------------------------------------------

func dataDir() string {
	d := filepath.Join(os.Getenv("LOCALAPPDATA"), "FPSBoost")
	os.MkdirAll(d, 0755)
	return d
}
func dataFile(n string) string { return filepath.Join(dataDir(), n) }

func saveJSON(name string, v interface{}) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err == nil {
		os.WriteFile(dataFile(name), b, 0644)
	}
}
func loadJSON(name string, v interface{}) bool {
	b, err := os.ReadFile(dataFile(name))
	if err != nil {
		return false
	}
	return json.Unmarshal(b, v) == nil
}

// ---- Configuration -------------------------------------------------------

type Config struct {
	PowerPlan     bool
	TimerRes      bool
	RamClean      bool
	GamePriority  bool
	BgPriority    bool
	AutoActivate  bool
	RamOnLaunch   bool
	OverlayAuto   bool
	OverlayTemps  bool
	OverlayRAM    bool
	OverlayPos    int
	Notifications bool
	CustomGames   []string
	GamesBoosted  int
	TrayHintShown bool
}

var cfg = &Config{PowerPlan: true, TimerRes: true, RamClean: true, GamePriority: true, BgPriority: true,
	AutoActivate: true, RamOnLaunch: true, Notifications: true, OverlayTemps: true, OverlayRAM: true}

func loadConfig() { loadJSON("config.json", cfg) }
func saveConfig() { saveJSON("config.json", cfg) }

// ---- Commandes cachées ---------------------------------------------------

func run(name string, args ...string) (string, int) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = -1
		}
	}
	return string(out), code
}

// ---- Registre -------------------------------------------------------------

const (
	HKCU = 0x80000001
	HKLM = 0x80000002
)

func rootKey(name string) uintptr {
	if name == "HKLM" {
		return HKLM
	}
	return HKCU
}

func regGet(root uintptr, path, name string) (kind string, dw uint32, sz string, ok bool) {
	var hk uintptr
	if call(pRegOpenKeyExW, root, uintptr(unsafe.Pointer(w(path))), 0, 0x20019, uintptr(unsafe.Pointer(&hk))) != 0 {
		return
	}
	defer call(pRegCloseKey, hk)
	var typ, size uint32
	if call(pRegQueryValueExW, hk, uintptr(unsafe.Pointer(w(name))), 0, uintptr(unsafe.Pointer(&typ)), 0, uintptr(unsafe.Pointer(&size))) != 0 {
		return
	}
	buf := make([]byte, size+4)
	if call(pRegQueryValueExW, hk, uintptr(unsafe.Pointer(w(name))), 0, uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))) != 0 {
		return
	}
	switch typ {
	case 4:
		return "dword", binary.LittleEndian.Uint32(buf), "", true
	case 1, 2:
		u := make([]uint16, size/2)
		for i := range u {
			u[i] = binary.LittleEndian.Uint16(buf[i*2:])
		}
		return "sz", 0, syscall.UTF16ToString(u), true
	}
	return "other", 0, "", true
}

func regOpenWrite(root uintptr, path string) (uintptr, bool) {
	var hk uintptr
	r := call(pRegCreateKeyExW, root, uintptr(unsafe.Pointer(w(path))), 0, 0, 0, 0xF003F, 0, uintptr(unsafe.Pointer(&hk)), 0)
	return hk, r == 0
}

func regSetDword(root uintptr, path, name string, v uint32) bool {
	hk, ok := regOpenWrite(root, path)
	if !ok {
		return false
	}
	defer call(pRegCloseKey, hk)
	return call(pRegSetValueExW, hk, uintptr(unsafe.Pointer(w(name))), 0, 4, uintptr(unsafe.Pointer(&v)), 4) == 0
}

func regSetSz(root uintptr, path, name, v string) bool {
	hk, ok := regOpenWrite(root, path)
	if !ok {
		return false
	}
	defer call(pRegCloseKey, hk)
	u, _ := syscall.UTF16FromString(v)
	return call(pRegSetValueExW, hk, uintptr(unsafe.Pointer(w(name))), 0, 1, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)*2)) == 0
}

func regDelete(root uintptr, path, name string) {
	var hk uintptr
	if call(pRegOpenKeyExW, root, uintptr(unsafe.Pointer(w(path))), 0, 0x20006, uintptr(unsafe.Pointer(&hk))) != 0 {
		return
	}
	call(pRegDeleteValueW, hk, uintptr(unsafe.Pointer(w(name))))
	call(pRegCloseKey, hk)
}

// ---- Optimisations Windows (avec sauvegarde) -----------------------------

type RegEntry struct {
	Root, Path, Name, Kind string
	Existed                bool
	DW                     uint32
	SZ                     string
}

type Backup struct {
	Entries       []RegEntry
	PowerScheme   string
	CreatedScheme string
	Applied       bool
}

var backup = &Backup{}

func loadBackup() { loadJSON("backup.json", backup) }
func saveBackup() { saveJSON("backup.json", backup) }

type tweak struct {
	root, path, name, kind string
	dw                     uint32
	sz                     string
}

const mmcss = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Multimedia\SystemProfile`

var tweaks = []tweak{
	{"HKCU", `System\GameConfigStore`, "GameDVR_Enabled", "dword", 0, ""},
	{"HKCU", `Software\Microsoft\Windows\CurrentVersion\GameDVR`, "AppCaptureEnabled", "dword", 0, ""},
	{"HKCU", `Software\Microsoft\GameBar`, "AutoGameModeEnabled", "dword", 1, ""},
	{"HKCU", `Software\Microsoft\GameBar`, "AllowAutoGameMode", "dword", 1, ""},
	{"HKLM", mmcss, "SystemResponsiveness", "dword", 10, ""},
	{"HKLM", mmcss + `\Tasks\Games`, "GPU Priority", "dword", 8, ""},
	{"HKLM", mmcss + `\Tasks\Games`, "Priority", "dword", 6, ""},
	{"HKLM", mmcss + `\Tasks\Games`, "Scheduling Category", "sz", 0, "High"},
	{"HKLM", mmcss + `\Tasks\Games`, "SFIO Priority", "sz", 0, "High"},
	{"HKLM", `SYSTEM\CurrentControlSet\Control\Session Manager\kernel`, "GlobalTimerResolutionRequests", "dword", 1, ""},
	{"HKLM", `SYSTEM\CurrentControlSet\Control\GraphicsDrivers`, "HwSchMode", "dword", 2, ""},
	{"HKLM", `SYSTEM\CurrentControlSet\Control\Power\PowerThrottling`, "PowerThrottlingOff", "dword", 1, ""},
}

func hasBackup(root, path, name string) bool {
	for _, e := range backup.Entries {
		if e.Root == root && strings.EqualFold(e.Path, path) && strings.EqualFold(e.Name, name) {
			return true
		}
	}
	return false
}

func applyTweaks() int {
	if backup.PowerScheme == "" {
		backup.PowerScheme = activeScheme()
	}
	n := 0
	for _, t := range tweaks {
		root := rootKey(t.root)
		if !hasBackup(t.root, t.path, t.name) {
			kind, dw, sz, ok := regGet(root, t.path, t.name)
			if kind == "other" {
				continue // type inattendu : on ne touche pas
			}
			backup.Entries = append(backup.Entries, RegEntry{t.root, t.path, t.name, t.kind, ok, dw, sz})
		}
		var ok bool
		if t.kind == "dword" {
			ok = regSetDword(root, t.path, t.name, t.dw)
		} else {
			ok = regSetSz(root, t.path, t.name, t.sz)
		}
		if ok {
			n++
		}
	}
	backup.Applied = true
	saveBackup()
	return n
}

func restoreTweaks() {
	for _, e := range backup.Entries {
		root := rootKey(e.Root)
		if !e.Existed {
			regDelete(root, e.Path, e.Name)
		} else if e.Kind == "dword" {
			regSetDword(root, e.Path, e.Name, e.DW)
		} else {
			regSetSz(root, e.Path, e.Name, e.SZ)
		}
	}
	backup.Entries = nil
	backup.Applied = false
}

// ---- Alimentation --------------------------------------------------------

var guidRe = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

const (
	ultimateGUID = "e9a42b02-d5df-448d-aa00-03f14749eb61"
	highPerfGUID = "8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c"
)

var bestPerfOverlay = GUID{0xded574b5, 0x45a0, 0x4f42, [8]byte{0x87, 0x37, 0x46, 0x34, 0x5c, 0x09, 0xc2, 0x38}}

func activeScheme() string {
	out, _ := run("powercfg", "/getactivescheme")
	return strings.ToLower(guidRe.FindString(out))
}

func setScheme(g string) bool {
	_, code := run("powercfg", "/setactive", g)
	return code == 0
}

func perfScheme() string {
	list, _ := run("powercfg", "/list")
	list = strings.ToLower(list)
	if backup.CreatedScheme != "" && strings.Contains(list, backup.CreatedScheme) {
		return backup.CreatedScheme
	}
	out, code := run("powercfg", "-duplicatescheme", ultimateGUID)
	if g := strings.ToLower(guidRe.FindString(out)); code == 0 && g != "" && g != ultimateGUID {
		backup.CreatedScheme = g
		saveBackup()
		run("powercfg", "/changename", g, "FPS Boost", "Performances maximales pour le jeu")
		return g
	}
	if strings.Contains(list, highPerfGUID) {
		return highPerfGUID
	}
	return ""
}

func getOverlay() (GUID, bool) {
	var g GUID
	if pPowerGetActualOverlayScheme.Find() != nil {
		return g, false
	}
	return g, call(pPowerGetActualOverlayScheme, uintptr(unsafe.Pointer(&g))) == 0
}

func setOverlay(g GUID) bool {
	if pPowerSetActiveOverlayScheme.Find() != nil {
		return false
	}
	return call(pPowerSetActiveOverlayScheme, uintptr(unsafe.Pointer(&g))) == 0
}

// ---- Privilèges / mémoire / timer ----------------------------------------

type luid struct {
	Low  uint32
	High int32
}
type tokenPriv struct {
	Count uint32
	Luid  luid
	Attr  uint32
}

func enablePrivilege(name string) {
	var tok uintptr
	if call(pOpenProcessToken, ^uintptr(0), 0x28, uintptr(unsafe.Pointer(&tok))) == 0 {
		return
	}
	tp := tokenPriv{Count: 1, Attr: 2}
	call(pLookupPrivilegeValueW, 0, uintptr(unsafe.Pointer(w(name))), uintptr(unsafe.Pointer(&tp.Luid)))
	call(pAdjustTokenPrivileges, tok, 0, uintptr(unsafe.Pointer(&tp)), 0, 0, 0)
	syscall.CloseHandle(syscall.Handle(tok))
}

func memStatus() MEMORYSTATUSEX {
	var m MEMORYSTATUSEX
	m.Length = uint32(unsafe.Sizeof(m))
	call(pGlobalMemoryStatusEx, uintptr(unsafe.Pointer(&m)))
	return m
}

type memListInfo struct {
	Zero, Free, Modified, ModifiedNoWrite, Bad uintptr
	ByPriority                                 [8]uintptr
	Repurposed                                 [8]uintptr
	ModifiedPageFile                           uintptr
}

func standbyBytes() (uint64, bool) {
	var m memListInfo
	var ret uint32
	if call(pNtQuerySystemInformation, 80, uintptr(unsafe.Pointer(&m)), unsafe.Sizeof(m), uintptr(unsafe.Pointer(&ret))) != 0 {
		return 0, false
	}
	var s uint64
	for _, v := range m.ByPriority {
		s += uint64(v)
	}
	return s * 4096, true
}

// Vide la liste de mémoire "standby" ; renvoie les Mo libérés
func purgeStandby() (int64, bool) {
	before, okB := standbyBytes()
	cmd := uint32(4) // MemoryPurgeStandbyList
	if call(pNtSetSystemInformation, 80, uintptr(unsafe.Pointer(&cmd)), 4) != 0 {
		return 0, false
	}
	after, okA := standbyBytes()
	if !okB || !okA || after > before {
		return 0, true
	}
	return int64(before-after) / (1024 * 1024), true
}

var pEmptyWorkingSet = modKernel32.NewProc("K32EmptyWorkingSet")

var noTrim = map[string]bool{"system": true, "registry": true, "memory compression": true, "dwm.exe": true,
	"csrss.exe": true, "audiodg.exe": true, "smss.exe": true, "wininit.exe": true, "lsass.exe": true}

// Réduit la mémoire utilisée par les programmes en arrière-plan
func trimWorkingSets(exclude uint32) int {
	n := 0
	for _, p := range snapshot() {
		if p.Pid <= 4 || p.Pid == exclude || noTrim[strings.ToLower(p.Name)] {
			continue
		}
		h, err := syscall.OpenProcess(0x0400|0x0100, false, p.Pid)
		if err != nil {
			continue
		}
		if call(pEmptyWorkingSet, uintptr(h)) != 0 {
			n++
		}
		syscall.CloseHandle(h)
	}
	return n
}

// Nettoyage complet : mémoire des programmes + cache standby
func fullClean(exclude uint32) (freedMB int64, before, after uint32) {
	m1 := memStatus()
	trimWorkingSets(exclude)
	purgeStandby()
	m2 := memStatus()
	d := int64(m2.AvailPhys) - int64(m1.AvailPhys)
	if d < 0 {
		d = 0
	}
	return d / (1024 * 1024), m1.MemoryLoad, m2.MemoryLoad
}

func timerInfo() (cur, finest uint32) {
	var a, b, c uint32
	call(pNtQueryTimerResolution, uintptr(unsafe.Pointer(&a)), uintptr(unsafe.Pointer(&b)), uintptr(unsafe.Pointer(&c)))
	finest = a
	if b < a {
		finest = b
	}
	return c, finest
}

func setTimer(on bool) float64 {
	_, finest := timerInfo()
	var cur uint32
	set := uintptr(0)
	if on {
		set = 1
	}
	call(pNtSetTimerResolution, uintptr(finest), set, uintptr(unsafe.Pointer(&cur)))
	return float64(cur) / 10000
}

// ---- Processus ------------------------------------------------------------

type procEntry struct {
	Pid  uint32
	Name string
}

func snapshot() []procEntry {
	h, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer syscall.CloseHandle(h)
	var e syscall.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var list []procEntry
	for err = syscall.Process32First(h, &e); err == nil; err = syscall.Process32Next(h, &e) {
		list = append(list, procEntry{e.ProcessID, syscall.UTF16ToString(e.ExeFile[:])})
	}
	return list
}

func procPath(pid uint32) string {
	h, err := syscall.OpenProcess(0x1000, false, pid)
	if err != nil {
		return ""
	}
	defer syscall.CloseHandle(h)
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	if call(pQueryFullProcessImageNameW, uintptr(h), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n))) == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

func getPriority(pid uint32) (uint32, bool) {
	h, err := syscall.OpenProcess(0x1000, false, pid)
	if err != nil {
		return 0, false
	}
	defer syscall.CloseHandle(h)
	r := uint32(call(pGetPriorityClass, uintptr(h)))
	return r, r != 0
}

func setPriority(pid uint32, cls uint32) bool {
	h, err := syscall.OpenProcess(0x0200|0x1000, false, pid)
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	return call(pSetPriorityClass, uintptr(h), uintptr(cls)) != 0
}

const (
	prioNormal      = 0x20
	prioHigh        = 0x80
	prioBelowNormal = 0x4000
)

// Fenêtres visibles -> PID
var enumPids map[uint32]bool
var enumCb = syscall.NewCallback(func(h, l uintptr) uintptr {
	if call(pIsWindowVisible, h) != 0 && call(pGetWindow, h, 4) == 0 && call(pGetWindowTextLengthW, h) > 0 {
		var pid uint32
		call(pGetWindowThreadProcessId, h, uintptr(unsafe.Pointer(&pid)))
		enumPids[pid] = true
	}
	return 1
})

func windowPids() map[uint32]bool {
	enumPids = map[uint32]bool{}
	call(pEnumWindows, enumCb, 0)
	return enumPids
}

// ---- Démarrage automatique ----------------------------------------------

const taskName = "FPS Boost"

func startupInstalled() bool {
	_, code := run("schtasks", "/Query", "/TN", taskName)
	return code == 0
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func installStartup() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	target := dataFile("FPSBoost.exe")
	if !strings.EqualFold(filepath.Clean(exe), filepath.Clean(target)) {
		if copyFile(exe, target) != nil {
			target = exe
		}
	}
	user := html.EscapeString(os.Getenv("USERDOMAIN") + `\` + os.Getenv("USERNAME"))
	xml := `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo><Description>Lance FPS Boost a l'ouverture de session</Description></RegistrationInfo>
  <Triggers><LogonTrigger><Enabled>true</Enabled><UserId>` + user + `</UserId><Delay>PT5S</Delay></LogonTrigger></Triggers>
  <Principals><Principal id="Author"><UserId>` + user + `</UserId><LogonType>InteractiveToken</LogonType><RunLevel>HighestAvailable</RunLevel></Principal></Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>4</Priority>
    <Enabled>true</Enabled>
  </Settings>
  <Actions Context="Author"><Exec><Command>` + html.EscapeString(target) + `</Command><Arguments>--tray</Arguments></Exec></Actions>
</Task>`
	u := utf16.Encode([]rune(xml))
	b := make([]byte, 2+len(u)*2)
	b[0], b[1] = 0xFF, 0xFE
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2+i*2:], c)
	}
	xmlPath := dataFile("task.xml")
	if os.WriteFile(xmlPath, b, 0644) != nil {
		return false
	}
	defer os.Remove(xmlPath)
	_, code := run("schtasks", "/Create", "/TN", taskName, "/XML", xmlPath, "/F")
	return code == 0
}

func removeStartup() {
	run("schtasks", "/Delete", "/TN", taskName, "/F")
}
