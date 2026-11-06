package main

// Mises à jour automatiques via GitHub (LogicArt19/fpsboost)

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	updateManifestURL = "https://raw.githubusercontent.com/LogicArt19/fpsboost/main/version.json"
	WM_UPDATE_DONE    = WM_APP + 6
	NIN_BALLOONCLICK  = 0x0405
)

type updateInfo struct {
	Version int    `json:"version"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Notes   string `json:"notes"`
}

var (
	pendingUpdate  *updateInfo
	updateBusy     bool
	updateFile     string
	updateErr      string
	balloonIsMaj   bool
	httpClient     = &http.Client{Timeout: 60 * time.Second}
)

const (
	updNone = iota
	updChecking
	updUpToDate
	updAvailable
	updError
	updDownloading
)

var (
	updState     = updNone
	updLastCheck time.Time
)

const releaseNotes = "• Nouvelle interface avec menu : Accueil, Paramètres, Jeux, Journal, Mises à jour\n" +
	"• Nettoyage complet de la RAM à chaque ouverture de FPS Boost (désactivable)\n" +
	"• Bouton « Libérer la RAM » plus efficace : affiche les Mo libérés et la RAM avant/après\n" +
	"• Grand compteur de FPS en direct sur l'accueil\n" +
	"• Statut du booster et FPS toujours visibles dans le menu"

// Résultat transmis à l'interface : wp = 0 à jour, 1 erreur, 2 trouvée ; lp = 1 si vérification manuelle
const WM_UPDATE_RESULT = WM_APP + 9

func checkForUpdate(manual bool) {
	if updState == updDownloading {
		return
	}
	updState = updChecking
	refreshUI()
	m := uintptr(0)
	if manual {
		m = 1
	}
	go func() {
		req, _ := http.NewRequest("GET", updateManifestURL+"?t="+fmt.Sprint(time.Now().Unix()), nil)
		req.Header.Set("Cache-Control", "no-cache")
		resp, err := httpClient.Do(req)
		if err != nil {
			call(pPostMessageW, hwndMain, WM_UPDATE_RESULT, 1, m)
			return
		}
		defer resp.Body.Close()
		var u updateInfo
		if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&u) != nil || u.URL == "" {
			call(pPostMessageW, hwndMain, WM_UPDATE_RESULT, 1, m)
			return
		}
		if u.Version > appVersion {
			pendingUpdate = &u
			call(pPostMessageW, hwndMain, WM_UPDATE_RESULT, 2, m)
		} else {
			call(pPostMessageW, hwndMain, WM_UPDATE_RESULT, 0, m)
		}
	}()
}

// Fenêtre "Nouvelle mise à jour disponible"
func offerUpdate() {
	u := pendingUpdate
	if u == nil || updateBusy {
		return
	}
	if call(pIsWindowVisible, hwndMain) == 0 {
		balloon("Nouvelle mise à jour disponible", "FPS Boost "+u.Name+" est disponible. Cliquez ici pour l'installer.")
		balloonIsMaj = true
		return
	}
	notes := strings.TrimSpace(u.Notes)
	if notes != "" {
		notes = "\n\nNouveautés :\n" + notes
	}
	msg := "Une nouvelle mise à jour est disponible !\n\nVersion actuelle : " + appVersionStr +
		"\nNouvelle version : " + u.Name + notes + "\n\nInstaller maintenant ? (quelques secondes)"
	if msgBox(msg, MB_YESNO|MB_ICONQUESTION) == IDYES {
		startUpdate()
	} else {
		addLog("Mise à jour " + u.Name + " disponible (page Mises à jour)")
	}
}

func startUpdate() {
	u := pendingUpdate
	if u == nil || updateBusy {
		return
	}
	updateBusy = true
	updState = updDownloading
	refreshUI()
	addLog("Téléchargement de la mise à jour " + u.Name + "…")
	go func() {
		updateErr = ""
		dst := dataFile("FPSBoost-update.exe")
		if err := download(u.URL, dst); err != nil {
			updateErr = err.Error()
		}
		updateFile = dst
		call(pPostMessageW, hwndMain, WM_UPDATE_DONE, 0, 0)
	}()
}
func download(url, dst string) error {
	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("serveur : %s", resp.Status)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, 200<<20))
	f.Close()
	if err != nil {
		return err
	}
	b := make([]byte, 2)
	if rf, e := os.Open(dst); e == nil {
		rf.Read(b)
		rf.Close()
	}
	if n < 500<<10 || string(b) != "MZ" {
		os.Remove(dst)
		return fmt.Errorf("fichier téléchargé invalide")
	}
	return nil
}

// Remplace l'exe en cours par la nouvelle version et la lance
func finishUpdate() {
	updateBusy = false
	updState = updAvailable
	if updateErr != "" {
		addLog("Échec de la mise à jour : " + updateErr)
		msgBox("La mise à jour n'a pas pu être téléchargée.\n"+updateErr+"\n\nRéessayez plus tard.", MB_OK|MB_ICONINFO)
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	old := strings.TrimSuffix(exe, filepath.Ext(exe)) + ".old.exe"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		addLog("Échec de la mise à jour : " + err.Error())
		return
	}
	if err := copyFile(updateFile, exe); err != nil {
		os.Rename(old, exe)
		addLog("Échec de la mise à jour : " + err.Error())
		return
	}
	os.Remove(updateFile)
	args := []string{"--updated"}
	if call(pIsWindowVisible, hwndMain) == 0 {
		args = append(args, "--tray")
	}
	cmd := exec.Command(exe, args...)
	if err := cmd.Start(); err != nil {
		os.Remove(exe)
		os.Rename(old, exe)
		addLog("Échec du lancement de la nouvelle version : " + err.Error())
		return
	}
	// La nouvelle version prend le relais (elle demande à celle-ci de se fermer)
}

// Supprime l'ancienne version laissée après une mise à jour
func cleanupOldVersion() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	old := strings.TrimSuffix(exe, filepath.Ext(exe)) + ".old.exe"
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(old); err != nil || os.Remove(old) == nil {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func handleUpdateMessages(msg, wp, lp uintptr) bool {
	switch msg {
	case WM_UPDATE_RESULT:
		updLastCheck = time.Now()
		manual := lp == 1
		switch wp {
		case 2:
			updState = updAvailable
			addLog("Nouvelle mise à jour disponible : FPS Boost " + pendingUpdate.Name)
			if manual {
				showMain()
			}
			refreshUI()
			offerUpdate()
		case 1:
			updState = updError
			refreshUI()
			if manual {
				msgBox("Impossible de vérifier les mises à jour.\nVérifiez votre connexion internet.", MB_OK|MB_ICONINFO)
			}
		default:
			updState = updUpToDate
			refreshUI()
			if manual {
				msgBox("Vous avez déjà la dernière version de FPS Boost ("+appVersionStr+").", MB_OK|MB_ICONINFO)
			}
		}
		return true
	case WM_UPDATE_DONE:
		finishUpdate()
		refreshUI()
		return true
	}
	return false
}
