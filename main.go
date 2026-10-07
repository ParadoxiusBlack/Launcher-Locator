// Launcher-Locator: interactive Sniper Elite mission maps.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/ParadoxiusBlack/Launcher-Locator/assets"
	"github.com/ParadoxiusBlack/Launcher-Locator/internal/server"
	"github.com/ParadoxiusBlack/Launcher-Locator/internal/store"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8765", "address to listen on")
	dataDir := flag.String("data", "", "data directory (default: per-user config directory)")
	admin := flag.Bool("admin", false, "enable developer tools (add maps/official indicators, review submissions)")
	noBrowser := flag.Bool("no-browser", false, "do not open the browser on start")
	flag.Parse()

	dir := *dataDir
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			log.Fatal(err)
		}
		dir = filepath.Join(base, "LauncherLocator")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "locator.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()
	if err := seed(st); err != nil {
		log.Fatal(err)
	}
	srv, err := server.New(st, assets.Static(), filepath.Join(dir, "uploads"), *admin)
	if err != nil {
		log.Fatal(err)
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	url := "http://" + ln.Addr().String()
	fmt.Println("Launcher Locator running at", url, "(data:", dir+")")
	if !*noBrowser {
		openBrowser(url)
	}
	log.Fatal(http.Serve(ln, srv))
}

func seed(st *store.Store) error {
	raw, err := assets.Seed()
	if err != nil {
		return err
	}
	var d struct {
		Types []store.IndicatorType `json:"types"`
		Maps  []store.Map           `json:"maps"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return err
	}
	for _, t := range d.Types {
		if _, err := st.UpsertType(t); err != nil {
			return err
		}
	}
	for _, m := range d.Maps {
		if _, err := st.UpsertMap(m); err != nil {
			return err
		}
	}
	return nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
