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
	"sync"
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

func runChat(args []string, ui *UI, apiClient client.RelayClient) int {
	chatCmd := flag.NewFlagSet("chat", flag.ContinueOnError)
	chatCmd.SetOutput(ui.Out)

	withFlag := chatCmd.String("with", "", "Recipient handle or comma-separated handles for group chat")
	groupFlag := chatCmd.String("group", "", "Alias for group members (comma-separated handles)")
	relayFlag := chatCmd.String("relay", client.DefaultRelayURL, "Relay server base URL")
	configFlag := chatCmd.String("config", "", "Path to identity file")

	if err := chatCmd.Parse(normalizeArgs(args)); err != nil {
		return 1
	}

	targetHandles := *withFlag
	if targetHandles == "" && *groupFlag != "" {
		targetHandles = *groupFlag
	}

	// 1. Verify Local Identity
	idPath := *configFlag
	if idPath == "" {
		var err error
		idPath, err = storage.DefaultIdentityPath()
		if err != nil {
			ui.Error("Failed to get default identity path: %v", err)
			return 1
		}
	}

	localIdFile, err := storage.LoadIdentity(idPath)
	if err != nil || localIdFile.PrivateKey == "" {
		ui.Error("Failed to load local device identity: %v", err)
		ui.Warn("Run 'pv init' first to generate your device keypair.")
		return 1
	}

	devIdentity, err := crypto.ParseIdentity(localIdFile.PrivateKey)
	if err != nil {
		ui.Error("Corrupt local private key: %v", err)
		return 1
	}

	// If using real HTTP client, update base URL from flag
	if httpCl, ok := apiClient.(*client.HTTPClient); ok && *relayFlag != "" {
		httpCl.BaseURL = *relayFlag
	}

	// 2. Validate Peer / Group Handles
	rawHandles := strings.Split(targetHandles, ",")
	var recipientHandles []string
	for _, h := range rawHandles {
		trimmed := strings.TrimSpace(h)
		if trimmed != "" && !strings.EqualFold(trimmed, localIdFile.Handle) {
			recipientHandles = append(recipientHandles, trimmed)
		}
	}

	if len(recipientHandles) == 0 {
		ui.Error("Missing target recipient. Usage: pv chat --with <HANDLE> or pv chat --group <H1,H2>")
		return 1
	}

	isGroup := len(recipientHandles) > 1

	// 3. Health & Public Keys
	if err := apiClient.Health(); err != nil {
		ui.Warn("Relay server health check warning: %v", err)
	}

	var members []groupMember
	for _, handle := range recipientHandles {
		keyInfo, err := apiClient.GetKey(handle)
		if err != nil {
			ui.Error("Failed to fetch public key for '%s': %v", handle, err)
			return 1
		}
		members = append(members, groupMember{
			handle:      keyInfo.Handle,
			publicKey:   keyInfo.PublicKey,
			fingerprint: keyInfo.Fingerprint,
		})
	}

	// 4. Print Simple Clean Terminal Header
	var targetTitle string
	var fpDetails string
	if isGroup {
		targetTitle = fmt.Sprintf("GROUP SESSION [%s]", strings.Join(recipientHandles, ", "))
		var fpList []string
		for _, m := range members {
			fpList = append(fpList, fmt.Sprintf("%s:%s", m.handle, m.fingerprint))
		}
		fpDetails = strings.Join(fpList, " | ")
	} else {
		targetTitle = fmt.Sprintf("End-to-End Encrypted Session with %s", members[0].handle)
		fpDetails = fmt.Sprintf("Device Fingerprint: [%s]", members[0].fingerprint)
	}

	fmt.Fprintf(ui.Out, "%s================================================================================%s\n", ColorCyan+ColorBold, ColorReset)
	fmt.Fprintf(ui.Out, "  %sPANDORA LIVE RELAY%s | %s%s%s\n", ColorGreen+ColorBold, ColorReset, ColorBold, targetTitle, ColorReset)
	fmt.Fprintf(ui.Out, "  %s%s%s | Zero Knowledge Relay Active\n", ColorYellow, fpDetails, ColorReset)
	fmt.Fprintf(ui.Out, "  Type your message or /f to attach a file. Press [Enter] to send. Press [Ctrl+C] or /quit to exit.\n")
	fmt.Fprintf(ui.Out, "%s================================================================================%s\n\n", ColorCyan+ColorBold, ColorReset)

	var printMu sync.Mutex
	prompt := fmt.Sprintf("%s[%s]%s > ", ColorCyan+ColorBold, localIdFile.Handle, ColorReset)

	printLine := func(line string) {
		printMu.Lock()
		defer printMu.Unlock()
		fmt.Fprintf(ui.Out, "\r\033[K%s\n%s", line, prompt)
	}

	// 5. Fetch Offline Inbox Messages
	for _, target := range recipientHandles {
		inboxEvents, err := apiClient.FetchInbox(localIdFile.Handle, target)
		if err == nil && len(inboxEvents) > 0 {
			for _, evt := range inboxEvents {
				plaintext, err := crypto.Decrypt([]byte(evt.Ciphertext), devIdentity)
				if err != nil {
					continue
				}
				senderName := evt.Sender
				if senderName == "" {
					senderName = target
				}
				filename, fileData, isFile := crypto.DecodeFilePayload(plaintext)
				if isFile {
					_ = os.MkdirAll("./downloads", 0755)
					savePath := filepath.Join("./downloads", filename)
					_ = os.WriteFile(savePath, fileData, 0600)
					printLine(fmt.Sprintf("%s[%s]%s %s[%s]%s > [FILE RECEIVED] %s (%d bytes) -> %s", ColorDim, time.Now().Format("15:04:05"), ColorReset, ColorYellow+ColorBold, senderName, ColorReset, filename, len(fileData), savePath))
				} else {
					printLine(fmt.Sprintf("%s[%s]%s %s[%s]%s > %s", ColorDim, time.Now().Format("15:04:05"), ColorReset, ColorYellow+ColorBold, senderName, ColorReset, string(plaintext)))
				}
			}
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

	// 6. Background Real-Time SSE Stream Listener
	stopCh := make(chan struct{})
	var closeOnce sync.Once
	safeClose := func() {
		closeOnce.Do(func() {
			close(stopCh)
		})
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		_ = apiClient.ListenStream(localIdFile.Handle, func(msg client.StreamEvent) {
			plaintext, err := crypto.Decrypt([]byte(msg.Ciphertext), devIdentity)
			if err != nil {
				return
			}
			timestamp := time.Now().Format("15:04:05")
			senderName := msg.Sender
			if senderName == "" {
				senderName = recipientHandles[0]
			}

			filename, fileData, isFile := crypto.DecodeFilePayload(plaintext)
			if isFile {
				_ = os.MkdirAll("./downloads", 0755)
				savePath := filepath.Join("./downloads", filename)
				_ = os.WriteFile(savePath, fileData, 0600)
				printLine(fmt.Sprintf("%s[%s]%s %s[%s]%s > [FILE RECEIVED] %s (%d bytes) -> %s", ColorDim, timestamp, ColorReset, ColorYellow+ColorBold, senderName, ColorReset, filename, len(fileData), savePath))
			} else {
				printLine(fmt.Sprintf("%s[%s]%s %s[%s]%s > %s", ColorDim, timestamp, ColorReset, ColorYellow+ColorBold, senderName, ColorReset, string(plaintext)))
			}
		}, stopCh)
	}()

	go func() {
		<-sigCh
		safeClose()
		fmt.Fprintf(ui.Out, "\n%s[i] Session closed.%s\n", ColorDim, ColorReset)
		os.Exit(0)
	}()

	// 7. Input Loop
	fmt.Fprint(ui.Out, prompt)
	scanner := bufio.NewScanner(ui.In)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			fmt.Fprint(ui.Out, prompt)
			continue
		}

		if text == "/quit" || text == "/exit" || text == ":q" {
			safeClose()
			fmt.Fprintf(ui.Out, "%s[i] Chat session terminated.%s\n", ColorDim, ColorReset)
			return 0
		}

		if text == "/help" {
			printLine(fmt.Sprintf("%s[i] Available commands: /f (attach file) | /clear | /quit%s", ColorDim, ColorReset))
			continue
		}

		if text == "/clear" {
			printMu.Lock()
			fmt.Fprint(ui.Out, "\033[2J\033[H")
			fmt.Fprintf(ui.Out, "%s================================================================================%s\n", ColorCyan+ColorBold, ColorReset)
			fmt.Fprintf(ui.Out, "  %sPANDORA LIVE RELAY%s | %s%s%s\n", ColorGreen+ColorBold, ColorReset, ColorBold, targetTitle, ColorReset)
			fmt.Fprintf(ui.Out, "%s================================================================================%s\n\n", ColorCyan+ColorBold, ColorReset)
			fmt.Fprint(ui.Out, prompt)
			printMu.Unlock()
			continue
		}

		// Handle file attachments (/f or /file or /sendfile)
		isFileCmd := false
		var filePath string

		if text == "/f" || text == "/file" || text == "/attach" || text == "/sendfile" {
			isFileCmd = true
			filePath = openNativeFileDialog(ui)
			if filePath == "" {
				printLine("[i] File attachment cancelled.")
				continue
			}
		} else if strings.HasPrefix(text, "/f ") || strings.HasPrefix(text, "/file ") || strings.HasPrefix(text, "/sendfile ") {
			isFileCmd = true
			parts := strings.SplitN(text, " ", 2)
			if len(parts) > 1 {
				filePath = strings.TrimSpace(parts[1])
			}
		}

		var ciphertext []byte
		var displayMsg string

		if isFileCmd {
			fileBytes, err := os.ReadFile(filePath)
			if err != nil {
				printLine(fmt.Sprintf("%s[!] Failed to read file %s: %v%s", ColorRed, filePath, err, ColorReset))
				continue
			}
			fn := filepath.Base(filePath)
			displayMsg = fmt.Sprintf("[FILE SENT] %s (%d bytes)", fn, len(fileBytes))
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
			printLine(fmt.Sprintf("%s[!] Encryption failed: %v%s", ColorRed, err, ColorReset))
			continue
		}

		// Post to relay
		if isGroup {
			_, err = apiClient.PostGroupChatMessage(recipientHandles, localIdFile.Handle, string(ciphertext))
		} else {
			_, err = apiClient.PostChatMessage(recipientHandles[0], localIdFile.Handle, string(ciphertext))
		}

		if err != nil {
			printLine(fmt.Sprintf("%s[!] Delivery failed: %v%s", ColorRed, err, ColorReset))
			continue
		}

		// Print outgoing message cleanly
		timestamp := time.Now().Format("15:04:05")
		printLine(fmt.Sprintf("%s[%s]%s %s[YOU]%s > %s%s%s", ColorDim, timestamp, ColorReset, ColorGreen+ColorBold, ColorReset, ColorGreen, displayMsg, ColorReset))
	}

	safeClose()
	return 0
}

func openNativeFileDialog(ui *UI) string {
	ui.Info("Opening file dialog...")
	cmd := exec.Command("powershell", "-NoProfile", "-Command", `
		Add-Type -AssemblyName System.Windows.Forms
		$dialog = New-Object System.Windows.Forms.OpenFileDialog
		$dialog.Title = "Select File to Encrypt & Send - Pandora's Veil"
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
