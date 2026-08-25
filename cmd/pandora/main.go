package main

import (
	"fmt"
	"os"

	"github.com/Mob-max30/pandoras-veil/internal/client"
)

const Version = "1.2.5"

func printGlobalUsage(ui *UI) {
	ui.Banner()
	fmt.Fprintf(ui.Out, "%sUSAGE:%s\n", ColorBold, ColorReset)
	fmt.Fprintf(ui.Out, "  pv <command> [arguments] [options]\n")
	fmt.Fprintf(ui.Out, "  (or: pandora <command> ...)\n\n")
	fmt.Fprintf(ui.Out, "%sCOMMANDS:%s\n", ColorBold, ColorReset)
	fmt.Fprintf(ui.Out, "  %sshell%s     Launch standalone Cyberpunk App Shell window (Edge App Mode)\n", ColorGreen, ColorReset)
	fmt.Fprintf(ui.Out, "  %stui%s       Launch multi-pane Terminal User Interface (ANSI Box Frame)\n", ColorGreen, ColorReset)
	fmt.Fprintf(ui.Out, "  %srun%s       Launch local Cyberpunk Web Dashboard at http://localhost:8080\n", ColorGreen, ColorReset)
	fmt.Fprintf(ui.Out, "  %sinit%s      Initialize local device cryptographic identity & register with relay\n", ColorGreen, ColorReset)
	fmt.Fprintf(ui.Out, "  %sidentity%s  Display this device's handle, public key, and fingerprint\n", ColorGreen, ColorReset)
	fmt.Fprintf(ui.Out, "  %ssend%s      Encrypt a secret locally and store encrypted envelope on relay\n", ColorGreen, ColorReset)
	fmt.Fprintf(ui.Out, "  %sread%s      Retrieve and decrypt a secret using local device private key\n", ColorGreen, ColorReset)
	fmt.Fprintf(ui.Out, "  %schat%s      Start real-time end-to-end encrypted live terminal chat\n", ColorGreen, ColorReset)
	fmt.Fprintf(ui.Out, "  %sversion%s   Print version information\n", ColorGreen, ColorReset)
	fmt.Fprintf(ui.Out, "  %shelp%s      Show this help menu\n\n", ColorGreen, ColorReset)
	fmt.Fprintf(ui.Out, "%sEXAMPLES:%s\n", ColorBold, ColorReset)
	fmt.Fprintf(ui.Out, "  pv run\n")
	fmt.Fprintf(ui.Out, "  pv init --handle PV-ALICE\n")
	fmt.Fprintf(ui.Out, "  pv identity\n")
	fmt.Fprintf(ui.Out, "  pv chat --with PV-BOB\n")
	fmt.Fprintf(ui.Out, "  pv send --to PV-BOB \"Secret password123\"\n")
	fmt.Fprintf(ui.Out, "  pv read pv_1700000000000000000\n\n")
}

func main() {
	ui := NewUI(os.Stdin, os.Stdout)
	apiClient := client.NewHTTPClient(client.DefaultRelayURL)

	if len(os.Args) < 2 {
		printGlobalUsage(ui)
		os.Exit(0)
	}

	command := os.Args[1]
	args := os.Args[2:]

	var exitCode int
	switch command {
	case "run", "serve", "web":
		exitCode = runWeb(args, ui, apiClient)
	case "init":
		exitCode = runInit(args, ui, apiClient)
	case "identity", "id":
		exitCode = runIdentity(args, ui, apiClient)
	case "send":
		exitCode = runSend(args, ui, apiClient)
	case "read":
		exitCode = runRead(args, ui, apiClient)
	case "chat":
		exitCode = runChat(args, ui, apiClient)
	case "tui":
		exitCode = runTUI(args, ui, apiClient)
	case "shell":
		exitCode = runShell(args, ui, apiClient)
	case "version", "-v", "--version":
		fmt.Fprintf(ui.Out, "Pandora's Veil v%s\n", Version)
		exitCode = 0
	case "help", "-h", "--help":
		printGlobalUsage(ui)
		exitCode = 0
	default:
		ui.Error("Unknown command '%s'", command)
		printGlobalUsage(ui)
		exitCode = 1
	}

	os.Exit(exitCode)
}
