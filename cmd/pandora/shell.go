package main

import (
	"flag"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/Mob-max30/pandoras-veil/internal/client"
	"github.com/Mob-max30/pandoras-veil/internal/shell"
)

// runShell handles 'pandora shell' command to launch the dedicated standalone App Shell UI window
func runShell(args []string, ui *UI, apiClient client.RelayClient) int {
	fs := flag.NewFlagSet("shell", flag.ContinueOnError)
	fs.SetOutput(ui.Out)

	portFlag := fs.Int("port", 8080, "Port for local App Shell server")

	if err := fs.Parse(normalizeArgs(args)); err != nil {
		return 1
	}

	ui.Info("Launching Pandora Dedicated Standalone App Shell UI...")
	appURL, err := shell.StartShellServer(*portFlag)
	if err != nil {
		ui.Error("Failed to start local App Shell server: %v", err)
		return 1
	}

	ui.Success("Pandora App Shell Server active at %s", appURL)
	ui.Info("Opening dedicated App Shell window (bypassing terminal line-wrapping)...")

	// Launch standalone application window (Edge App Mode on Windows)
	go func() {
		time.Sleep(500 * time.Millisecond)
		if runtime.GOOS == "windows" {
			_ = exec.Command("cmd", "/c", "start", "msedge", fmt.Sprintf("--app=%s", appURL), "--window-size=1280,820").Run()
		} else {
			_ = exec.Command("open", appURL).Run()
		}
	}()

	fmt.Fprintf(ui.Out, "\n%s================ PANDORA STANDALONE APP SHELL ===============%s\n", ColorBold, ColorReset)
	fmt.Fprintf(ui.Out, "  App Shell URL: %s%s%s\n", ColorCyan, appURL, ColorReset)
	fmt.Fprintf(ui.Out, "  Status:        %sACTIVE (Dedicated Window Opened)%s\n", ColorGreen, ColorReset)
	fmt.Fprintf(ui.Out, "  Press [Ctrl+C] in this terminal to stop the App Shell server.\n")
	fmt.Fprintf(ui.Out, "%s==============================================================%s\n\n", ColorBold, ColorReset)

	select {}
}
