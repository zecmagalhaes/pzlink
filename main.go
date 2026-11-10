package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"tailscale.com/envknob"
)

var version = "dev"

func main() {
	uiPort := flag.Int("ui", 7468, "porta da interface web local")
	noBrowser := flag.Bool("no-browser", false, "não abrir o navegador automaticamente")
	flag.Parse()

	// Never upload logs to Tailscale's log service: friends did not agree to it.
	envknob.SetNoLogsNoSupport()

	cfgDir := configDir()
	logFile, err := os.OpenFile(filepath.Join(cfgDir, "pzlink.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err == nil {
		log.SetOutput(&redactWriter{w: logFile})
		defer logFile.Close()
	} else {
		log.SetOutput(io.Discard)
	}

	app := newApp(cfgDir)

	ln, port, err := listenUI(*uiPort)
	if err != nil {
		fmt.Println("Não consegui abrir a interface:", err)
		waitEnter()
		os.Exit(1)
	}
	url := fmt.Sprintf("http://127.0.0.1:%d", port)

	fmt.Println("==============================================")
	fmt.Println(" PZLink", version)
	fmt.Println(" Interface:", url)
	fmt.Println(" Deixe esta janela aberta enquanto joga.")
	fmt.Println(" Feche a janela (ou Ctrl+C) para sair.")
	fmt.Println("==============================================")

	go func() {
		if err := http.Serve(ln, app.routes(port)); err != nil {
			log.Println("http:", err)
		}
	}()
	if !*noBrowser {
		openBrowser(url)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	fmt.Println("Encerrando...")
	app.stop()
}

func listenUI(start int) (net.Listener, int, error) {
	var lastErr error
	for p := start; p < start+20; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			return ln, p, nil
		}
		lastErr = err
	}
	return nil, 0, lastErr
}

func configDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "pzlink")
	os.MkdirAll(dir, 0o700)
	return dir
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
	if err := cmd.Start(); err != nil {
		fmt.Println("Abra no navegador:", url)
	}
}

func waitEnter() {
	fmt.Println("Pressione Enter para sair.")
	fmt.Scanln()
}
