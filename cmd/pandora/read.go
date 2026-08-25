package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/Mob-max30/pandoras-veil/internal/client"
	"github.com/Mob-max30/pandoras-veil/internal/crypto"
	"github.com/Mob-max30/pandoras-veil/internal/storage"
)

// extractPasteID parses a raw paste ID or a full URL
func extractPasteID(input string) string {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		u, err := url.Parse(input)
		if err == nil {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(parts) > 0 {
				return parts[len(parts)-1]
			}
		}
	}
	return input
}

// runRead handles 'pandora read' command
func runRead(args []string, ui *UI, apiClient client.Client) int {
	fs := flag.NewFlagSet("read", flag.ContinueOnError)
	fs.SetOutput(ui.Out)

	saveFlag := fs.String("save", "", "Path to save decrypted secret file (default: print to stdout)")
	relayFlag := fs.String("relay", "http://127.0.0.1:8080", "Relay server URL")
	pathFlag := fs.String("config", "", "Custom path for local identity file")

	fs.Usage = func() {
		fmt.Fprintf(ui.Out, "Usage: pandora read <share-id-or-url> [options]\n\n")
		fmt.Fprintf(ui.Out, "Retrieves and decrypts a secret using this device's private identity key.\n\n")
		fmt.Fprintf(ui.Out, "Options:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(normalizeArgs(args)); err != nil {
		return 1
	}

	if fs.NArg() < 1 {
		ui.Error("Missing Share ID or Link. Usage: pandora read <id-or-url>")
		fs.Usage()
		return 1
	}

	rawTarget := fs.Arg(0)
	pasteID := extractPasteID(rawTarget)
	if pasteID == "" {
		ui.Error("Invalid Share ID or Link provided: %s", rawTarget)
		return 1
	}

	// 1. Load Local Identity
	localIdFile, err := storage.LoadIdentity(*pathFlag)
	if err != nil {
		ui.Error("Failed to load local device identity: %v", err)
		ui.Info("Run 'pandora init' to initialize this device first.")
		return 1
	}

	devIdentity, err := crypto.ParseIdentity(localIdFile.PrivateKey)
	if err != nil {
		ui.Error("Corrupted local private key: %v", err)
		return 1
	}

	// 2. Fetch Ciphertext from Relay
	if httpCl, ok := apiClient.(*client.HTTPClient); ok && *relayFlag != "" {
		httpCl.BaseURL = *relayFlag
	}

	ui.Info("Fetching encrypted secret '%s' from relay...", pasteID)
	ciphertext, err := apiClient.GetPaste(pasteID)
	if err != nil {
		if err == client.ErrNotFound {
			ui.Error("Secret not found. It may have expired or already been burned after reading.")
		} else {
			ui.Error("Failed to retrieve secret from relay: %v", err)
		}
		return 1
	}

	// 3. Local Decryption Attempt with Device Key
	ui.Info("Attempting device-bound decryption...")
	plaintext, err := crypto.Decrypt(ciphertext, devIdentity.Identity)
	if err != nil {
		// Generic security failure message - never leak whether it was wrong key vs corrupted ciphertext
		ui.Error("ACCESS DENIED: %v", crypto.ErrDecryptionFailed)
		return 1
	}

	// 4. Output Decrypted Secret
	ui.Success("Decryption successful! (Authorized device key matched)")

	if *saveFlag != "" {
		if err := os.WriteFile(*saveFlag, plaintext, 0600); err != nil {
			ui.Error("Failed to save secret to %s: %v", *saveFlag, err)
			return 1
		}
		ui.Success("Secret saved to %s (permissions 0600)", *saveFlag)
	} else {
		fmt.Fprintf(ui.Out, "\n%s================ DECRYPTED SECRET ===============%s\n", ColorBold, ColorReset)
		fmt.Fprintf(ui.Out, "%s\n", string(plaintext))
		fmt.Fprintf(ui.Out, "%s=================================================%s\n\n", ColorBold, ColorReset)
	}

	return 0
}
