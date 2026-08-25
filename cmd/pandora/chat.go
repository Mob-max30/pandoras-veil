package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Mob-max30/pandoras-veil/internal/client"
	"github.com/Mob-max30/pandoras-veil/internal/crypto"
	"github.com/Mob-max30/pandoras-veil/internal/storage"
)

type groupMember struct {
	handle      string
	publicKey   string
	fingerprint string
}

// runChat handles 'pandora chat' command for 1-on-1 and Group Chat
func runChat(args []string, ui *UI, apiClient client.RelayClient) int {
	fs := flag.NewFlagSet("chat", flag.ContinueOnError)
	fs.SetOutput(ui.Out)

	withFlag := fs.String("with", "", "Recipient handle or comma-separated handles for group chat (e.g. PV-BOB or PV-BOB,PV-ALICE)")
	groupFlag := fs.String("group", "", "Comma-separated group member handles (e.g. PV-BOB,PV-ALICE)")
	relayFlag := fs.String("relay", client.DefaultRelayURL, "Relay server URL")
	pathFlag := fs.String("config", "", "Custom path for local identity file")

	fs.Usage = func() {
		fmt.Fprintf(ui.Out, "Usage: pandora chat --with <handle> [options]\n")
		fmt.Fprintf(ui.Out, "   or: pandora chat --group <handle1,handle2> [options]\n\n")
		fmt.Fprintf(ui.Out, "Starts a real-time, end-to-end encrypted live chat session (1-on-1 or Group).\n\n")
		fmt.Fprintf(ui.Out, "Options:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(normalizeArgs(args)); err != nil {
		return 1
	}

	targetHandlesStr := *withFlag
	if targetHandlesStr == "" {
		targetHandlesStr = *groupFlag
	}

	if targetHandlesStr == "" {
		ui.Error("Recipient handle is required. Specify with --with <handle> or --group <handle1,handle2>")
		fs.Usage()
		return 1
	}

	// Parse recipient handles
	var rawHandles []string
	if strings.Contains(targetHandlesStr, ",") {
		rawHandles = strings.Split(targetHandlesStr, ",")
	} else {
		rawHandles = []string{targetHandlesStr}
	}

	var recipientHandles []string
	seen := make(map[string]bool)
	for _, h := range rawHandles {
		trimmed := strings.TrimSpace(h)
		if trimmed != "" && !seen[strings.ToUpper(trimmed)] {
			seen[strings.ToUpper(trimmed)] = true
			recipientHandles = append(recipientHandles, trimmed)
		}
	}

	if len(recipientHandles) == 0 {
		ui.Error("No valid recipient handles specified.")
		return 1
	}

	isGroup := len(recipientHandles) > 1

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

	// Remove local user from recipient handles if included in group list
	filteredHandles := make([]string, 0, len(recipientHandles))
	for _, h := range recipientHandles {
		if !strings.EqualFold(h, localIdFile.Handle) {
			filteredHandles = append(filteredHandles, h)
		}
	}
	if len(filteredHandles) == 0 {
		ui.Error("Cannot start a chat session targeting only yourself.")
		return 1
	}
	recipientHandles = filteredHandles
	isGroup = len(recipientHandles) > 1

	// 3. Resolve All Recipient Keys & Fingerprints
	var members []groupMember
	for _, handle := range recipientHandles {
		ui.Info("Resolving recipient '%s' from relay...", handle)
		info, err := apiClient.GetKey(handle)
		if err != nil {
			ui.Error("Failed to resolve recipient '%s': %v", handle, err)
			return 1
		}
		fp := info.Fingerprint
		if fp == "" {
			fp = crypto.ComputeFingerprint(info.PublicKey)
		}
		members = append(members, groupMember{
			handle:      handle,
			publicKey:   info.PublicKey,
			fingerprint: fp,
		})
	}

	// 4. Mandatory Fingerprint Verification Hard-Stop Upfront
	if isGroup {
		fmt.Fprintf(ui.Out, "\n%s================ LIVE GROUP CHAT SECURITY VERIFICATION ================%s\n", ColorBold, ColorReset)
		fmt.Fprintf(ui.Out, "  Your Handle:   %s%s%s (Fingerprint: %s)\n", ColorCyan, localIdFile.Handle, ColorReset, localIdFile.Fingerprint)
		fmt.Fprintf(ui.Out, "  Group Members (%d):\n", len(members))
		for _, m := range members {
			fmt.Fprintf(ui.Out, "    • %s%s%s (Fingerprint: %s%s%s)\n", ColorCyan, m.handle, ColorReset, ColorYellow, m.fingerprint, ColorReset)
		}
		fmt.Fprintf(ui.Out, "%s========================================================================%s\n", ColorBold, ColorReset)
		fmt.Fprintf(ui.Out, "%s[SECURITY CHECK]%s Verify that device fingerprints for ALL group members match.\n\n", ColorYellow, ColorReset)

		promptText := fmt.Sprintf("Establish encrypted group chat session with %d member(s)?", len(members))
		if !ui.PromptConfirm(promptText) {
			ui.Error("Verification aborted by user. Group chat session terminated.")
			return 1
		}
	} else {
		m := members[0]
		fmt.Fprintf(ui.Out, "\n%s================ LIVE CHAT SECURITY VERIFICATION ===============%s\n", ColorBold, ColorReset)
		fmt.Fprintf(ui.Out, "  Your Handle:           %s%s%s (Fingerprint: %s)\n", ColorCyan, localIdFile.Handle, ColorReset, localIdFile.Fingerprint)
		fmt.Fprintf(ui.Out, "  Recipient Handle:      %s%s%s\n", ColorCyan, m.handle, ColorReset)
		fmt.Fprintf(ui.Out, "  Recipient Fingerprint: %s%s%s\n", ColorYellow, m.fingerprint, ColorReset)
		fmt.Fprintf(ui.Out, "  Target Public Key:     %s%s%s\n", ColorDim, m.publicKey, ColorReset)
		fmt.Fprintf(ui.Out, "%s=================================================================%s\n", ColorBold, ColorReset)
		fmt.Fprintf(ui.Out, "%s[SECURITY CHECK]%s Verify that the recipient's device fingerprint matches.\n\n", ColorYellow, ColorReset)

		promptText := fmt.Sprintf("Establish encrypted live session with %s (%s)?", m.handle, m.fingerprint)
		if !ui.PromptConfirm(promptText) {
			ui.Error("Verification aborted by user. Chat session terminated.")
			return 1
		}
	}

	// 5. Render Live Chat Header
	fmt.Fprintf(ui.Out, "\n%s%s================================================================================%s\n", ColorBold, ColorCyan, ColorReset)
	if isGroup {
		handleListStr := strings.Join(recipientHandles, ", ")
		fmt.Fprintf(ui.Out, "%s%s  🔒 PANDORA LIVE GROUP RELAY | Encrypted Group Session (%d Members)%s\n", ColorBold, ColorGreen, len(members)+1, ColorReset)
		fmt.Fprintf(ui.Out, "%s%s  Group Members: [%s] | Zero Knowledge Relay Active%s\n", ColorDim, ColorWhite, handleListStr, ColorReset)
	} else {
		m := members[0]
		fmt.Fprintf(ui.Out, "%s%s  🔒 PANDORA LIVE RELAY | End-to-End Encrypted Session with %s%s\n", ColorBold, ColorGreen, m.handle, ColorReset)
		fmt.Fprintf(ui.Out, "%s%s  Device Fingerprint: [%s] | Zero Knowledge Relay Active%s\n", ColorDim, ColorWhite, m.fingerprint, ColorReset)
	}
	fmt.Fprintf(ui.Out, "%s%s  Type your message or %s/f%s%s to attach a file. Press [Ctrl+C] to exit.%s\n", ColorDim, ColorCyan, ColorBold+ColorYellow, ColorReset, ColorDim+ColorCyan, ColorReset)
	fmt.Fprintf(ui.Out, "%s%s================================================================================%s\n\n", ColorBold, ColorCyan, ColorReset)

	// 5b. Flush & Render Pending Offline Queued Messages
	inboxMsgs, err := apiClient.FetchInbox(localIdFile.Handle, recipientHandles[0])
	if err == nil && len(inboxMsgs) > 0 {
		for _, msg := range inboxMsgs {
			plaintext, err := crypto.Decrypt([]byte(msg.Ciphertext), devIdentity)
			if err != nil {
				continue
			}
			filename, fileData, isFile := crypto.DecodeFilePayload(plaintext)
			timestamp := time.Now().Format("15:04:05")
			senderName := msg.Sender
			if senderName == "" {
				senderName = recipientHandles[0]
			}
			if isFile {
				_ = os.MkdirAll("./downloads", 0755)
				savePath := filepath.Join("./downloads", filename)
				if err := os.WriteFile(savePath, fileData, 0600); err == nil {
					fmt.Fprintf(ui.Out, "%s[%s]%s %s[%s] (Offline Queue) ❯%s 📁 [FILE RECEIVED] %s (%d bytes) -> Saved to %s\n",
						ColorDim, timestamp, ColorReset,
						ColorBold+ColorMagenta, senderName, ColorReset,
						ColorYellow+filename+ColorReset, len(fileData), ColorCyan+savePath+ColorReset)
				}
			} else {
				fmt.Fprintf(ui.Out, "%s[%s]%s %s[%s] (Offline Queue) ❯%s %s\n",
					ColorDim, timestamp, ColorReset,
					ColorBold+ColorMagenta, senderName, ColorReset,
					string(plaintext))
			}
		}
	}

	stopCh := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	const chatWidth = 72

	// 6. Background Listener Goroutine (SSE Stream - Left Aligned)
	go func() {
		_ = apiClient.ListenStream(localIdFile.Handle, func(msg client.StreamEvent) {
			// Session isolation check: filter out messages from anyone outside active recipient list
			isMember := false
			for _, h := range recipientHandles {
				if strings.EqualFold(msg.Sender, h) {
					isMember = true
					break
				}
			}
			if msg.Sender != "" && !isMember {
				return // Drop non-session messages
			}

			plaintext, err := crypto.Decrypt([]byte(msg.Ciphertext), devIdentity)
			if err != nil {
				// Silently skip unaddressed/corrupted messages
				return
			}
			filename, fileData, isFile := crypto.DecodeFilePayload(plaintext)
			timestamp := time.Now().Format("15:04:05")
			senderName := msg.Sender
			if senderName == "" {
				senderName = recipientHandles[0]
			}

			var incomingMsg string
			if isFile {
				_ = os.MkdirAll("./downloads", 0755)
				savePath := filepath.Join("./downloads", filename)
				if err := os.WriteFile(savePath, fileData, 0600); err == nil {
					incomingMsg = fmt.Sprintf("%s[%s]%s %s[%s] ❯%s 📁 [FILE RECEIVED] %s (%d bytes) -> Saved to %s",
						ColorDim, timestamp, ColorReset,
						ColorBold+ColorMagenta, senderName, ColorReset,
						ColorYellow+filename+ColorReset, len(fileData), ColorCyan+savePath+ColorReset)
				} else {
					incomingMsg = fmt.Sprintf("%s[%s]%s %s[%s] ❯%s 📁 [FILE RECEIVED] %s (%d bytes)",
						ColorDim, timestamp, ColorReset,
						ColorBold+ColorMagenta, senderName, ColorReset,
						filename, len(fileData))
				}
			} else {
				// Left-aligned incoming bubble
				incomingMsg = fmt.Sprintf("%s[%s]%s %s[%s] ❯%s %s",
					ColorDim, timestamp, ColorReset,
					ColorBold+ColorMagenta, senderName, ColorReset,
					string(plaintext),
				)
			}

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

		isFileCmd := false
		var filePath string

		if text == "/f" || text == "/file" || text == "/attach" || text == "/sendfile" {
			isFileCmd = true
			filePath = openNativeFileDialog(ui)
			if filePath == "" {
				ui.Info("File attachment cancelled.")
				promptPrompt()
				continue
			}
		} else if strings.HasPrefix(text, "/f ") || strings.HasPrefix(text, "/file ") || strings.HasPrefix(text, "/attach ") || strings.HasPrefix(text, "/sendfile ") {
			isFileCmd = true
			parts := strings.SplitN(text, " ", 2)
			if len(parts) > 1 {
				filePath = strings.TrimSpace(parts[1])
			}
		}

		// Encrypt message or file locally using age recipient-based encryption
		var ciphertext []byte
		var displayMsg string
		if isFileCmd {
			fileBytes, err := os.ReadFile(filePath)
			if err != nil {
				ui.Error("Failed to read file %s: %v", filePath, err)
				promptPrompt()
				continue
			}
			fn := filepath.Base(filePath)
			displayMsg = fmt.Sprintf("📁 [FILE SENT] %s (%d bytes)", fn, len(fileBytes))
			if isGroup {
				pubKeys := make([]string, len(members))
				for i, m := range members {
					pubKeys[i] = m.publicKey
				}
				ciphertext, err = crypto.EncryptFilePayloadMulti(fn, fileBytes, pubKeys...)
			} else {
				ciphertext, err = crypto.EncryptFilePayload(fn, fileBytes, members[0].publicKey)
			}
		} else {
			displayMsg = text
			if isGroup {
				pubKeys := make([]string, len(members))
				for i, m := range members {
					pubKeys[i] = m.publicKey
				}
				ciphertext, err = crypto.EncryptMulti([]byte(text), pubKeys...)
			} else {
				ciphertext, err = crypto.Encrypt([]byte(text), members[0].publicKey)
			}
		}
		if err != nil {
			ui.Error("Encryption failed: %v", err)
			promptPrompt()
			continue
		}

		// Send live message envelope(s) to relay
		if isGroup {
			_, err = apiClient.PostGroupChatMessage(recipientHandles, localIdFile.Handle, string(ciphertext))
		} else {
			_, err = apiClient.PostChatMessage(recipientHandles[0], localIdFile.Handle, string(ciphertext))
		}
		if err != nil {
			ui.Error("Delivery failed: %v", err)
			promptPrompt()
			continue
		}

		timestamp := time.Now().Format("15:04:05")

		// Right-aligned outgoing bubble (WhatsApp style)
		visibleLen := len(displayMsg) + len(timestamp) + 12
		pad := chatWidth - visibleLen
		if pad < 2 {
			pad = 2
		}
		spaces := strings.Repeat(" ", pad)

		// Move cursor up 1 line, clear it, and print right-aligned sent message
		fmt.Fprintf(ui.Out, "\033[1A\r\033[K%s%s%s%s %s[YOU]%s %s[%s]%s\n",
			spaces,
			ColorBold+ColorGreen, displayMsg, ColorReset,
			ColorBold+ColorMagenta, ColorReset,
			ColorDim, timestamp, ColorReset,
		)

		promptPrompt()
	}

	close(stopCh)
	return 0
}

// openNativeFileDialog launches native Windows File Explorer GUI file picker dialog
func openNativeFileDialog(ui *UI) string {
	ui.Info("Opening File Explorer window... Select any image, document, or media file.")
	cmd := exec.Command("powershell", "-NoProfile", "-Command", `
		Add-Type -AssemblyName System.Windows.Forms
		$dialog = New-Object System.Windows.Forms.OpenFileDialog
		$dialog.Title = "Select File / Media to Encrypt & Send - Pandora's Veil"
		$dialog.Filter = "All Files (*.*)|*.*|Images (*.png;*.jpg;*.jpeg;*.gif)|*.png;*.jpg;*.jpeg;*.gif|Documents (*.pdf;*.docx;*.txt)|*.pdf;*.docx;*.txt|Media (*.mp4;*.mp3;*.zip)|*.mp4;*.mp3;*.zip"
		if ($dialog.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {
			Write-Output $dialog.FileName
		}
	`)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

