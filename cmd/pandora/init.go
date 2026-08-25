package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Mob-max30/pandoras-veil/internal/client"
	"github.com/Mob-max30/pandoras-veil/internal/crypto"
	"github.com/Mob-max30/pandoras-veil/internal/storage"
)

// runInit handles 'pandora init' command
func runInit(args []string, ui *UI, apiClient client.Client) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(ui.Out)

	handleFlag := fs.String("handle", "", "Desired human-readable device handle (e.g. PV-ALICE)")
	relayFlag := fs.String("relay", "http://127.0.0.1:8080", "Relay server URL")
	pathFlag := fs.String("config", "", "Custom path for identity file (defaults to ~/.pandora/identity.json)")
	forceFlag := fs.Bool("force", false, "Overwrite existing device identity if present")

	fs.Usage = func() {
		fmt.Fprintf(ui.Out, "Usage: pandora init [options]\n\n")
		fmt.Fprintf(ui.Out, "Initializes a device identity (X25519 keypair) and registers it with the relay.\n\n")
		fmt.Fprintf(ui.Out, "Options:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(normalizeArgs(args)); err != nil {
		return 1
	}

	targetPath := *pathFlag
	if targetPath == "" {
		var err error
		targetPath, err = storage.DefaultIdentityPath()
		if err != nil {
			ui.Error("Failed to determine default config path: %v", err)
			return 1
		}
	}

	// Check if identity already exists
	if _, err := os.Stat(targetPath); err == nil && !*forceFlag {
		ui.Warn("Device identity already exists at %s", targetPath)
		ui.Info("Use --force to overwrite, or run 'pandora identity' to view existing credentials.")
		return 1
	}

	ui.Info("Generating new X25519 device keypair...")
	deviceIdentity, err := crypto.GenerateIdentity()
	if err != nil {
		ui.Error("Key generation failed: %v", err)
		return 1
	}

	// If using real HTTP client, update base URL from flag
	if httpCl, ok := apiClient.(*client.HTTPClient); ok && *relayFlag != "" {
		httpCl.BaseURL = *relayFlag
	}

	ui.Info("Registering public key with relay (%s)...", *relayFlag)
	regInfo, err := apiClient.RegisterKey(*handleFlag, deviceIdentity.PublicKey)
	if err != nil {
		ui.Warn("Relay registration notice: %v", err)
		ui.Info("Saving identity locally anyway for offline/local use...")
		// Generate a local handle fallback if relay is offline
		if *handleFlag != "" {
			regInfo = &client.KeyInfo{
				Handle:      *handleFlag,
				PublicKey:   deviceIdentity.PublicKey,
				Fingerprint: deviceIdentity.Fingerprint,
			}
		} else {
			regInfo = &client.KeyInfo{
				Handle:      fmt.Sprintf("PV-%s", deviceIdentity.Fingerprint),
				PublicKey:   deviceIdentity.PublicKey,
				Fingerprint: deviceIdentity.Fingerprint,
			}
		}
	}

	// Save identity locally with 0600 permissions
	idFile := &storage.IdentityFile{
		Handle:      regInfo.Handle,
		PublicKey:   deviceIdentity.PublicKey,
		PrivateKey:  deviceIdentity.PrivateKey,
		Fingerprint: deviceIdentity.Fingerprint,
	}

	if err := storage.SaveIdentity(targetPath, idFile); err != nil {
		ui.Error("Failed to save identity to %s: %v", targetPath, err)
		return 1
	}

	ui.Success("Device initialized successfully!")
	fmt.Fprintf(ui.Out, "\n%sDevice Credentials:%s\n", ColorBold, ColorReset)
	fmt.Fprintf(ui.Out, "  Handle:      %s%s%s\n", ColorCyan, idFile.Handle, ColorReset)
	fmt.Fprintf(ui.Out, "  Fingerprint: %s%s%s\n", ColorYellow, idFile.Fingerprint, ColorReset)
	fmt.Fprintf(ui.Out, "  Public Key:  %s\n", idFile.PublicKey)
	fmt.Fprintf(ui.Out, "  Config File: %s (0600 permissions)\n\n", targetPath)

	return 0
}
