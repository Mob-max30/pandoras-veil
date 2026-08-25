package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/Mob-max30/pandoras-veil/internal/client"
	"github.com/Mob-max30/pandoras-veil/internal/storage"
	"github.com/Mob-max30/pandoras-veil/internal/web"
)

func runWeb(args []string, ui *UI, apiClient client.RelayClient) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(ui.Out)

	portFlag := fs.Int("port", 8080, "Local port to host web interface")
	openFlag := fs.Bool("open", true, "Automatically open default browser")
	relayFlag := fs.String("relay", client.DefaultRelayURL, "Relay server URL")
	configFlag := fs.String("config", "", "Custom path for local identity file")

	fs.Usage = func() {
		fmt.Fprintf(ui.Out, "Usage: pv run [options]\n")
		fmt.Fprintf(ui.Out, "   or: pv serve [options]\n\n")
		fmt.Fprintf(ui.Out, "Starts the local Pandora's Veil Web Dashboard interface on localhost.\n\n")
		fmt.Fprintf(ui.Out, "Options:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(normalizeArgs(args)); err != nil {
		return 1
	}

	// 1. Verify local device identity exists
	idFile, err := storage.LoadIdentity(*configFlag)
	if err != nil {
		ui.Error("No local identity found: %v", err)
		ui.Info("Initializing new default device identity for web interface...")
		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = "USER"
		}
		defaultHandle := "PV-" + sanitizeHandle(hostname)
		if defaultHandle == "PV-" {
			defaultHandle = "PV-LOCAL"
		}
		initArgs := []string{"--handle", defaultHandle}
		if *configFlag != "" {
			initArgs = append(initArgs, "--path", *configFlag)
		}
		if code := runInit(initArgs, ui, apiClient); code != 0 {
			ui.Error("Failed to initialize identity automatically. Please run 'pv init' first.")
			return 1
		}
		idFile, _ = storage.LoadIdentity(*configFlag)
	}

	// 2. Health check relay
	if httpCl, ok := apiClient.(*client.HTTPClient); ok && *relayFlag != "" {
		httpCl.BaseURL = *relayFlag
	}
	if err := apiClient.Health(); err != nil {
		ui.Warn("Relay server health check warning: %v (operating in local buffer mode)", err)
	}

	// 3. Find open port
	port := *portFlag
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		// Fallback to random free port
		l, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			ui.Error("Failed to bind local port: %v", err)
			return 1
		}
		port = l.Addr().(*net.TCPAddr).Port
		addr = fmt.Sprintf("127.0.0.1:%d", port)
	}
	_ = l.Close()

	webServer := web.NewServer(apiClient, *relayFlag, *configFlag)
	srv := &http.Server{
		Addr:         addr,
		Handler:      webServer.Routes(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // SSE streaming requires infinite write timeout
	}

	localURL := fmt.Sprintf("http://localhost:%d", port)

	ui.Banner()
	fmt.Fprintf(ui.Out, "%s================================================================================%s\n", ColorCyan+ColorBold, ColorReset)
	fmt.Fprintf(ui.Out, "  🌐 %sPANDORA'S VEIL WEB DASHBOARD RUNNING%s\n", ColorGreen+ColorBold, ColorReset)
	fmt.Fprintf(ui.Out, "  👉 %sLocal URL:%s  %s%s%s\n", ColorBold, ColorReset, ColorYellow+ColorBold, localURL, ColorReset)
	fmt.Fprintf(ui.Out, "  🔒 %sDevice:%s     %s%s%s (Fingerprint: %s%s%s)\n", ColorBold, ColorReset, ColorCyan, idFile.Handle, ColorReset, ColorYellow, idFile.Fingerprint, ColorReset)
	fmt.Fprintf(ui.Out, "  ☁️  %sRelay:%s      %s%s%s\n", ColorBold, ColorReset, ColorDim, *relayFlag, ColorReset)
	fmt.Fprintf(ui.Out, "  ⚡ %sZero-Knowledge E2E Encryption Active (age/X25519)%s\n", ColorGreen, ColorReset)
	fmt.Fprintf(ui.Out, "%s================================================================================%s\n\n", ColorCyan+ColorBold, ColorReset)
	fmt.Fprintf(ui.Out, "%s[i] Press [Ctrl+C] to stop the local web server.%s\n\n", ColorDim, ColorReset)

	// Automatically open browser if requested
	if *openFlag {
		go openBrowser(localURL)
	}

	// Graceful shutdown channel
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			ui.Error("Web server error: %v", err)
		}
	}()

	<-stopCh
	fmt.Fprintf(ui.Out, "\n%s[i] Shutting down local web server...%s\n", ColorYellow, ColorReset)
	_ = srv.Close()
	return 0
}

func openBrowser(url string) {
	time.Sleep(300 * time.Millisecond)
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

func sanitizeHandle(s string) string {
	var out []rune
	for _, r := range strings.ToUpper(s) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			out = append(out, r)
		}
	}
	if len(out) > 16 {
		out = out[:16]
	}
	return string(out)
}
