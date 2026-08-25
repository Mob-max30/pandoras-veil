package main

import (
	"flag"
	"fmt"

	"github.com/Mob-max30/pandoras-veil/internal/client"
	"github.com/Mob-max30/pandoras-veil/internal/storage"
)

// runIdentity handles 'pandora identity' command
func runIdentity(args []string, ui *UI, apiClient client.Client) int {
	fs := flag.NewFlagSet("identity", flag.ContinueOnError)
	fs.SetOutput(ui.Out)

	pathFlag := fs.String("config", "", "Custom path for identity file (defaults to ~/.pandora/identity.json)")
	relayFlag := fs.String("relay", "http://127.0.0.1:8080", "Relay server URL")

	fs.Usage = func() {
		fmt.Fprintf(ui.Out, "Usage: pandora identity [options]\n\n")
		fmt.Fprintf(ui.Out, "Displays local device handle, public key, and cryptographic fingerprint.\n\n")
		fmt.Fprintf(ui.Out, "Options:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(normalizeArgs(args)); err != nil {
		return 1
	}

	// If using real HTTP client, update base URL from flag
	if httpCl, ok := apiClient.(*client.HTTPClient); ok && *relayFlag != "" {
		httpCl.BaseURL = *relayFlag
	}

	// Strict server check
	if err := apiClient.Health(); err != nil {
		ui.Error("SERVER OFFLINE: Cannot reach relay server at %s. Exiting.", *relayFlag)
		return 1
	}

	idFile, err := storage.LoadIdentity(*pathFlag)
	if err != nil {
		ui.Error("Failed to load device identity: %v", err)
		ui.Info("Run 'pandora init' first to generate your device credentials.")
		return 1
	}

	fmt.Fprintf(ui.Out, "%sDevice Identity:%s\n", ColorBold, ColorReset)
	fmt.Fprintf(ui.Out, "  Handle:      %s%s%s\n", ColorCyan, idFile.Handle, ColorReset)
	fmt.Fprintf(ui.Out, "  Fingerprint: %s%s%s\n", ColorYellow, idFile.Fingerprint, ColorReset)
	fmt.Fprintf(ui.Out, "  Public Key:  %s\n", idFile.PublicKey)
	fmt.Fprintf(ui.Out, "\n%sSecurity Tip:%s Share your Handle or Fingerprint out-of-band with senders to verify authenticity.\n", ColorDim, ColorReset)

	return 0
}
