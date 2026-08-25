package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Mob-max30/pandoras-veil/internal/client"
	"github.com/Mob-max30/pandoras-veil/internal/crypto"
	"github.com/Mob-max30/pandoras-veil/internal/storage"
)

// runChat handles 'pandora chat' command
func runChat(args []string, ui *UI, apiClient client.RelayClient) int {
	fs := flag.NewFlagSet("chat", flag.ContinueOnError)
	fs.SetOutput(ui.Out)

	withFlag := fs.String("with", "", "Recipient handle to chat with (e.g. PV-BOB) (required)")
	relayFlag := fs.String("relay", client.DefaultRelayURL, "Relay server URL")
	pathFlag := fs.String("config", "", "Custom path for local identity file")

	fs.Usage = func() {
		fmt.Fprintf(ui.Out, "Usage: pandora chat --with <handle> [options]\n\n")
		fmt.Fprintf(ui.Out, "Starts a real-time, end-to-end encrypted live chat session with another device.\n\n")
		fmt.Fprintf(ui.Out, "Options:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(normalizeArgs(args)); err != nil {
		return 1
	}

	if *withFlag == "" {
		ui.Error("Recipient handle is required. Specify with --with <handle>")
		fs.Usage()
		return 1
	}

	// If using real HTTP client, update base URL from flag
	if httpCl, ok := apiClient.(*client.HTTPClient); ok && *relayFlag != "" {
		httpCl.BaseURL = *relayFlag
	}

	// 1. Strict Server Health Check
	if err := apiClient.Health(); err != nil {
		ui.Error("SERVER OFFLINE: Cannot reach relay server at %s. Exiting.", *relayFlag)
		return 1
	}

	// 2. Load Local Identity
	localIdFile, err := storage.LoadIdentity(*pathFlag)
	if err != nil {
		ui.Error("Failed to load local device identity: %v", err)
		ui.Info("Run 'pandora init' first to initialize your device.")
		return 1
	}

	devIdentity, err := crypto.ParseIdentity(localIdFile.PrivateKey)
	if err != nil {
		ui.Error("Corrupted local private key: %v", err)
		return 1
	}

	// 3. Resolve Recipient Key & Fingerprint
	ui.Info("Resolving recipient '%s' from relay...", *withFlag)
	recipientInfo, err := apiClient.GetKey(*withFlag)
	if err != nil {
		ui.Error("Failed to resolve recipient '%s': %v", *withFlag, err)
		return 1
	}

	expectedFingerprint := recipientInfo.Fingerprint
	if expectedFingerprint == "" {
		expectedFingerprint = crypto.ComputeFingerprint(recipientInfo.PublicKey)
	}

	// 4. Mandatory Fingerprint Verification Hard-Stop Upfront
	fmt.Fprintf(ui.Out, "\n%s================ LIVE CHAT SECURITY VERIFICATION ===============%s\n", ColorBold, ColorReset)
	fmt.Fprintf(ui.Out, "  Your Handle:           %s%s%s (Fingerprint: %s)\n", ColorCyan, localIdFile.Handle, ColorReset, localIdFile.Fingerprint)
	fmt.Fprintf(ui.Out, "  Recipient Handle:      %s%s%s\n", ColorCyan, *withFlag, ColorReset)
	fmt.Fprintf(ui.Out, "  Recipient Fingerprint: %s%s%s\n", ColorYellow, expectedFingerprint, ColorReset)
	fmt.Fprintf(ui.Out, "  Target Public Key:     %s%s%s\n", ColorDim, recipientInfo.PublicKey, ColorReset)
	fmt.Fprintf(ui.Out, "%s=================================================================%s\n", ColorBold, ColorReset)
	fmt.Fprintf(ui.Out, "%s[SECURITY CHECK]%s Verify that the recipient's device fingerprint matches.\n\n", ColorYellow, ColorReset)

	promptText := fmt.Sprintf("Establish encrypted live session with %s (%s)?", *withFlag, expectedFingerprint)
	if !ui.PromptConfirm(promptText) {
		ui.Error("Verification aborted by user. Chat session terminated.")
		return 1
	}

	// 5. Render Live Chat Header
	fmt.Fprintf(ui.Out, "\n%s%s================================================================================%s\n", ColorBold, ColorCyan, ColorReset)
	fmt.Fprintf(ui.Out, "%s%s  🔒 PANDORA LIVE RELAY | End-to-End Encrypted Session with %s%s\n", ColorBold, ColorGreen, *withFlag, ColorReset)
	fmt.Fprintf(ui.Out, "%s%s  Device Fingerprint: [%s] | Zero Knowledge Relay Active%s\n", ColorDim, ColorWhite, expectedFingerprint, ColorReset)
	fmt.Fprintf(ui.Out, "%s%s  Type your message and press [Enter] to send live. Press [Ctrl+C] to exit.%s\n", ColorDim, ColorCyan, ColorReset)
	fmt.Fprintf(ui.Out, "%s%s================================================================================%s\n\n", ColorBold, ColorCyan, ColorReset)

	stopCh := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	const chatWidth = 72

	// 6. Background Listener Goroutine (SSE Stream - Left Aligned)
	go func() {
		_ = apiClient.ListenStream(localIdFile.Handle, func(msg client.StreamEvent) {
			// Strict 1-on-1 session isolation: drop messages from any sender other than the active recipient (*withFlag)
			if msg.Sender != "" && !strings.EqualFold(msg.Sender, *withFlag) {
				return
			}

			plaintext, err := crypto.Decrypt([]byte(msg.Ciphertext), devIdentity)
			if err != nil {
				// Silently skip corrupted or unaddressed messages
				return
			}
			timestamp := time.Now().Format("15:04:05")
			senderName := msg.Sender
			if senderName == "" {
				senderName = *withFlag
			}

			// Left-aligned incoming bubble (WhatsApp style)
			incomingMsg := fmt.Sprintf("%s[%s]%s %s[%s] ❯%s %s",
				ColorDim, timestamp, ColorReset,
				ColorBold+ColorMagenta, senderName, ColorReset,
				string(plaintext),
			)

			// Clear current line, print incoming message, and redraw prompt
			fmt.Fprintf(ui.Out, "\r\033[K%s\n%s[%s] > %s", incomingMsg, ColorBold+ColorCyan, localIdFile.Handle, ColorReset)
		}, stopCh)
	}()

	// 7. Foreground Sender Loop (Right Aligned)
	scanner := bufio.NewScanner(ui.In)
	promptPrompt := func() {
		fmt.Fprintf(ui.Out, "%s[%s] > %s", ColorBold+ColorCyan, localIdFile.Handle, ColorReset)
	}

	promptPrompt()

	go func() {
		<-sigCh
		close(stopCh)
		fmt.Fprintf(ui.Out, "\n\n%s[i] Live chat session closed.%s\n", ColorYellow, ColorReset)
		os.Exit(0)
	}()

	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			promptPrompt()
			continue
		}

		if text == "/quit" || text == "/exit" {
			close(stopCh)
			fmt.Fprintf(ui.Out, "%s[i] Live chat session ended.%s\n", ColorYellow, ColorReset)
			return 0
		}

		// Encrypt message locally targeting recipient's public key
		ciphertext, err := crypto.Encrypt([]byte(text), recipientInfo.PublicKey)
		if err != nil {
			ui.Error("Encryption failed: %v", err)
			promptPrompt()
			continue
		}

		// Send live message envelope to relay
		_, err = apiClient.PostChatMessage(*withFlag, localIdFile.Handle, string(ciphertext))
		if err != nil {
			ui.Error("Delivery failed: %v", err)
			promptPrompt()
			continue
		}

		timestamp := time.Now().Format("15:04:05")
		
		// Right-aligned outgoing bubble (WhatsApp style)
		// Format: <message in green>  [YOU in pink] [timestamp in gray]
		visibleLen := len(text) + len(timestamp) + 12
		pad := chatWidth - visibleLen
		if pad < 2 {
			pad = 2
		}
		spaces := strings.Repeat(" ", pad)

		// Move cursor up 1 line, clear it, and print right-aligned sent message
		fmt.Fprintf(ui.Out, "\033[1A\r\033[K%s%s%s%s %s[YOU]%s %s[%s]%s\n",
			spaces,
			ColorBold+ColorGreen, text, ColorReset,
			ColorBold+ColorMagenta, ColorReset,
			ColorDim, timestamp, ColorReset,
		)

		promptPrompt()
	}

	close(stopCh)
	return 0
}
