package main

import (
	"fmt"
	"os"

	"github.com/Mob-max30/pandoras-veil/internal/client"
)

const Version = "1.0.0-beta"

func printGlobalUsage(ui *UI) {
	ui.Banner()
	fmt.Fprintf(ui.Out, "%sUSAGE:%s\n", ColorBold, ColorReset)
	fmt.Fprintf(ui.Out, "  pandora <command> [arguments] [options]\n\n")
	fmt.Fprintf(ui.Out, "%sCOMMANDS:%s\n", ColorBold, ColorReset)
	fmt.Fprintf(ui.Out, "  %sinit%s      Initialize local device cryptographic identity & register with relay\n", ColorGreen, ColorReset)
	fmt.Fprintf(ui.Out, "  %sidentity%s  Display this device's handle, public key, and fingerprint\n", ColorGreen, ColorReset)
	fmt.Fprintf(ui.Out, "  %ssend%s      Encrypt a secret locally and store encrypted envelope on relay\n", ColorGreen, ColorReset)
	fmt.Fprintf(ui.Out, "  %sread%s      Retrieve and decrypt a secret using local device private key\n", ColorGreen, ColorReset)
	fmt.Fprintf(ui.Out, "  %sversion%s   Print version information\n", ColorGreen, ColorReset)
	fmt.Fprintf(ui.Out, "  %shelp%s      Show this help menu\n\n", ColorGreen, ColorReset)
	fmt.Fprintf(ui.Out, "%sEXAMPLES:%s\n", ColorBold, ColorReset)
	fmt.Fprintf(ui.Out, "  pandora init --handle PV-ALICE\n")
	fmt.Fprintf(ui.Out, "  pandora identity\n")
	fmt.Fprintf(ui.Out, "  pandora send --to PV-BOB \"Secret password123\"\n")
	fmt.Fprintf(ui.Out, "  pandora send --to PV-BOB --file ./api_keys.env --burn\n")
	fmt.Fprintf(ui.Out, "  pandora read pv_1700000000000000000\n\n")
}

func main() {
	ui := NewUI(os.Stdin, os.Stdout)
	apiClient := client.NewHTTPClient("http://127.0.0.1:8080")

	if len(os.Args) < 2 {
		printGlobalUsage(ui)
		os.Exit(0)
	}

	command := os.Args[1]
	args := os.Args[2:]

	var exitCode int
	switch command {
	case "init":
		exitCode = runInit(args, ui, apiClient)
	case "identity", "id":
		exitCode = runIdentity(args, ui)
	case "send":
		exitCode = runSend(args, ui, apiClient)
	case "read":
		exitCode = runRead(args, ui, apiClient)
	case "version", "-v", "--version":
		fmt.Fprintf(ui.Out, "Pandora's Veil CLI v%s\n", Version)
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
