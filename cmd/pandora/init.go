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
	relayFlag := fs.String("relay", client.DefaultRelayURL, "Relay server URL")
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

	// If using real HTTP client, update base URL from flag
	if httpCl, ok := apiClient.(*client.HTTPClient); ok && *relayFlag != "" {
		httpCl.BaseURL = *relayFlag
	}

	// Strict server check: fail immediately if server is offline
	if err := apiClient.Health(); err != nil {
		ui.Error("SERVER OFFLINE: Cannot connect to relay server at %s. Exiting.", *relayFlag)
		return 1
	}

	// Check if identity already exists locally
	if _, err := os.Stat(targetPath); err == nil && !*forceFlag {
		existingId, err := storage.LoadIdentity(targetPath)
		if err == nil && existingId.PublicKey != "" {
			// Check if already registered on the server
			serverKey, err := apiClient.GetKey(existingId.Handle)
			if err == nil && serverKey.PublicKey == existingId.PublicKey {
				ui.Success("Device identity is already registered on the relay server!")
				fmt.Fprintf(ui.Out, "\n%sDevice Credentials:%s\n", ColorBold, ColorReset)
				fmt.Fprintf(ui.Out, "  Handle:      %s%s%s\n", ColorCyan, existingId.Handle, ColorReset)
				fmt.Fprintf(ui.Out, "  Fingerprint: %s%s%s\n", ColorYellow, existingId.Fingerprint, ColorReset)
				fmt.Fprintf(ui.Out, "  Public Key:  %s\n", existingId.PublicKey)
				fmt.Fprintf(ui.Out, "  Config File: %s (0600 permissions)\n\n", targetPath)
				return 0
			}

			// Server restarted or lost key -> automatically re-register existing identity!
			ui.Info("Existing local identity found. Re-registering with relay server (%s)...", *relayFlag)
			handleToRegister := existingId.Handle
			if *handleFlag != "" {
				handleToRegister = *handleFlag
			}
			regInfo, err := apiClient.RegisterKey(handleToRegister, existingId.PublicKey)
			if err != nil {
				if err == client.ErrConflict {
					ui.Error("Handle '%s' is already registered by another device on the relay! Use --force to generate a new identity.", handleToRegister)
					return 1
				}
				ui.Error("Failed to re-register with relay: %v", err)
				return 1
			}
			existingId.Handle = regInfo.Handle
			_ = storage.SaveIdentity(targetPath, existingId)
			ui.Success("Device re-registered on relay server successfully!")
			fmt.Fprintf(ui.Out, "\n%sDevice Credentials:%s\n", ColorBold, ColorReset)
			fmt.Fprintf(ui.Out, "  Handle:      %s%s%s\n", ColorCyan, existingId.Handle, ColorReset)
			fmt.Fprintf(ui.Out, "  Fingerprint: %s%s%s\n", ColorYellow, existingId.Fingerprint, ColorReset)
			fmt.Fprintf(ui.Out, "  Public Key:  %s\n", existingId.PublicKey)
			fmt.Fprintf(ui.Out, "  Config File: %s (0600 permissions)\n\n", targetPath)
			return 0
		}
	}

	ui.Info("Generating new X25519 device keypair...")
	deviceIdentity, err := crypto.GenerateIdentity()
	if err != nil {
		ui.Error("Key generation failed: %v", err)
		return 1
	}

	ui.Info("Registering public key with relay (%s)...", *relayFlag)
	regInfo, err := apiClient.RegisterKey(*handleFlag, deviceIdentity.PublicKey)
	if err != nil {
		if err == client.ErrConflict {
			ui.Error("Handle '%s' is already registered on the relay! Please choose a different handle name or omit --handle for an automatic unique handle.", *handleFlag)
			return 1
		}
		ui.Error("Relay registration failed: %v", err)
		ui.Info("The relay server (%s) must be running to register your device handle.", *relayFlag)
		return 1
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
