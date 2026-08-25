package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Mob-max30/pandoras-veil/internal/client"
	"github.com/Mob-max30/pandoras-veil/internal/crypto"
)

// runSend handles 'pandora send' command
func runSend(args []string, ui *UI, apiClient client.Client) int {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	fs.SetOutput(ui.Out)

	toFlag := fs.String("to", "", "Recipient handle (e.g. PV-ALICE) or public key (required)")
	fileFlag := fs.String("file", "", "Path to file containing secret payload")
	ttlFlag := fs.Int("ttl", 86400, "Time-to-live in seconds (default: 86400 = 24 hours)")
	burnFlag := fs.Bool("burn", false, "Burn after reading (destroy secret immediately upon first decryption)")
	relayFlag := fs.String("relay", client.DefaultRelayURL, "Relay server URL")
	_ = fs.String("config", "", "Optional path for local config file")

	fs.Usage = func() {
		fmt.Fprintf(ui.Out, "Usage: pandora send --to <handle> [options] [secret text]\n\n")
		fmt.Fprintf(ui.Out, "Encrypts a secret locally for a specific recipient device and stores ciphertext on the relay.\n\n")
		fmt.Fprintf(ui.Out, "Options:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(normalizeArgs(args)); err != nil {
		return 1
	}

	if *toFlag == "" {
		ui.Error("Recipient handle is required. Specify with --to <handle>")
		fs.Usage()
		return 1
	}

	// 1. Resolve Recipient Key & Fingerprint
	var recipientPubKey string
	var expectedFingerprint string

	// If using real HTTP client, update base URL from flag
	if httpCl, ok := apiClient.(*client.HTTPClient); ok && *relayFlag != "" {
		httpCl.BaseURL = *relayFlag
	}

	// Check if server is reachable before doing anything
	if err := apiClient.Health(); err != nil {
		ui.Error("SERVER OFFLINE: Cannot reach relay server at %s (%v)", *relayFlag, err)
		return 1
	}

	if strings.HasPrefix(*toFlag, "age1") {
		recipientPubKey = *toFlag
		expectedFingerprint = crypto.ComputeFingerprint(recipientPubKey)
	} else {
		ui.Info("Resolving recipient '%s' from relay...", *toFlag)
		keyInfo, err := apiClient.GetKey(*toFlag)
		if err != nil {
			ui.Error("Failed to resolve recipient '%s': %v", *toFlag, err)
			return 1
		}
		recipientPubKey = keyInfo.PublicKey
		expectedFingerprint = keyInfo.Fingerprint
		if expectedFingerprint == "" {
			expectedFingerprint = crypto.ComputeFingerprint(recipientPubKey)
		}
	}

	// 2. Display Recipient Verification Banner
	fmt.Fprintf(ui.Out, "\n%s================ RECIPIENT VERIFICATION ===============%s\n", ColorBold, ColorReset)
	fmt.Fprintf(ui.Out, "  Recipient Handle:      %s%s%s\n", ColorCyan, *toFlag, ColorReset)
	fmt.Fprintf(ui.Out, "  Device Fingerprint:    %s%s%s\n", ColorYellow, expectedFingerprint, ColorReset)
	fmt.Fprintf(ui.Out, "  Target Public Key:     %s%s%s\n", ColorDim, recipientPubKey, ColorReset)
	fmt.Fprintf(ui.Out, "%s=======================================================%s\n", ColorBold, ColorReset)
	fmt.Fprintf(ui.Out, "%s[SECURITY CHECK]%s Confirm that the fingerprint matches the recipient's device out-of-band.\n\n", ColorYellow, ColorReset)

	// 3. HARD STOP: Mandatory Fingerprint Verification Prompt (No bypass)
	promptText := fmt.Sprintf("Verify device %s?", expectedFingerprint)
	confirmed := ui.PromptConfirm(promptText)
	if !confirmed {
		ui.Error("Fingerprint verification aborted by user. Encryption stopped.")
		return 1
	}

	// 4. Read Plaintext
	var plaintext []byte
	if *fileFlag != "" {
		data, err := os.ReadFile(*fileFlag)
		if err != nil {
			ui.Error("Failed to read secret file %s: %v", *fileFlag, err)
			return 1
		}
		plaintext = data
	} else if len(fs.Args()) > 0 {
		plaintext = []byte(strings.Join(fs.Args(), " "))
	} else {
		// Try reading from piped stdin
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			// Data is being piped in
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				ui.Error("Failed to read from stdin: %v", err)
				return 1
			}
			plaintext = data
		} else {
			ui.Error("No secret payload provided. Provide text as argument, use --file, or pipe via stdin.")
			return 1
		}
	}

	if len(plaintext) == 0 {
		ui.Error("Secret payload is empty.")
		return 1
	}

	// 5. Local Device-Bound Encryption via age
	ui.Info("Encrypting secret locally for recipient device key...")
	ciphertext, err := crypto.Encrypt(plaintext, recipientPubKey)
	if err != nil {
		ui.Error("Encryption failed: %v", err)
		return 1
	}

	// 6. Upload Ciphertext to Relay
	ui.Info("Uploading encrypted envelope to relay (%s)...", *relayFlag)
	pasteID, err := apiClient.PostPaste(ciphertext, *ttlFlag, *burnFlag)
	if err != nil {
		ui.Error("Failed to upload secret to relay: %v", err)
		return 1
	}

	// 7. Output Share Link & Details
	shareURL := fmt.Sprintf("%s/paste/%s", strings.TrimRight(*relayFlag, "/"), pasteID)
	ui.Success("Secret encrypted and deposited successfully!")
	fmt.Fprintf(ui.Out, "\n%sShare Details:%s\n", ColorBold, ColorReset)
	fmt.Fprintf(ui.Out, "  Share ID:    %s%s%s\n", ColorGreen, pasteID, ColorReset)
	fmt.Fprintf(ui.Out, "  Share Link:  %s%s%s\n", ColorCyan, shareURL, ColorReset)
	fmt.Fprintf(ui.Out, "  Target:      %s (Fingerprint: %s)\n", *toFlag, expectedFingerprint)
	if *burnFlag {
		fmt.Fprintf(ui.Out, "  Policy:      %sBURN AFTER READING%s (auto-deleted upon first read)\n", ColorRed, ColorReset)
	} else {
		fmt.Fprintf(ui.Out, "  Policy:      TTL %d seconds\n", *ttlFlag)
	}
	fmt.Fprintf(ui.Out, "\n%sRecipient Decryption Command:%s\n", ColorBold, ColorReset)
	fmt.Fprintf(ui.Out, "  %spandora read %s%s\n\n", ColorWhite, pasteID, ColorReset)

	return 0
}
