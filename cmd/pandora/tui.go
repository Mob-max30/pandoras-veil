package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Mob-max30/pandoras-veil/internal/client"
	"github.com/Mob-max30/pandoras-veil/internal/crypto"
	"github.com/Mob-max30/pandoras-veil/internal/storage"
	"github.com/Mob-max30/pandoras-veil/internal/tui"
)

// runTUI handles 'pandora tui' command to launch the full multi-pane dashboard TUI
func runTUI(args []string, ui *UI, apiClient client.RelayClient) int {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	fs.SetOutput(ui.Out)

	withFlag := fs.String("with", "PV-UJWAL", "Initial chat recipient handle")
	relayFlag := fs.String("relay", client.DefaultRelayURL, "Relay server URL")
	pathFlag := fs.String("config", "", "Custom path for local identity file")
	ttlFlag := fs.Int("ttl", 86400, "Time-to-live in seconds")
	burnFlag := fs.Bool("burn", false, "Burn after reading mode")

	if err := fs.Parse(normalizeArgs(args)); err != nil {
		return 1
	}

	if httpCl, ok := apiClient.(*client.HTTPClient); ok && *relayFlag != "" {
		httpCl.BaseURL = *relayFlag
	}

	// 1. Health Check
	if err := apiClient.Health(); err != nil {
		ui.Error("SERVER OFFLINE: Cannot reach relay server at %s", *relayFlag)
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

	// 3. Resolve Actual Registered Contacts (Zero fake data)
	targetHandle := strings.TrimSpace(*withFlag)
	if targetHandle == "" {
		targetHandle = "PV-UJWAL"
	}

	var contacts []tui.ContactItem
	var targetPubKey string
	var targetFingerprint string

	// Attempt to resolve target handle live
	info, err := apiClient.GetKey(targetHandle)
	if err == nil {
		targetPubKey = info.PublicKey
		targetFingerprint = info.Fingerprint
		if targetFingerprint == "" {
			targetFingerprint = crypto.ComputeFingerprint(targetPubKey)
		}
		contacts = append(contacts, tui.ContactItem{
			Handle:      targetHandle,
			Fingerprint: targetFingerprint,
			IsActive:    true,
		})
	} else {
		// Fallback to target handle placeholder
		contacts = append(contacts, tui.ContactItem{
			Handle:   targetHandle,
			IsActive: true,
		})
	}

	secState := tui.SecurityState{
		RecipientHandle: targetHandle,
		Fingerprint:     targetFingerprint,
		PublicKey:       targetPubKey,
		TTLExpiration:   *ttlFlag,
		BurnAfterRead:   *burnFlag,
		Verified:        true,
	}

	var chatMessages []tui.ChatMessageItem

	// 4. Fetch Offline Queued Messages
	inboxMsgs, err := apiClient.FetchInbox(localIdFile.Handle, targetHandle)
	if err == nil && len(inboxMsgs) > 0 {
		for _, m := range inboxMsgs {
			plaintext, err := crypto.Decrypt([]byte(m.Ciphertext), devIdentity)
			if err != nil {
				continue
			}
			fn, fData, isFile := crypto.DecodeFilePayload(plaintext)
			sender := m.Sender
			if sender == "" {
				sender = targetHandle
			}
			ts := time.Now().Format("15:04")
			if isFile {
				_ = os.MkdirAll("./downloads", 0755)
				savePath := filepath.Join("./downloads", fn)
				_ = os.WriteFile(savePath, fData, 0600)
				chatMessages = append(chatMessages, tui.ChatMessageItem{
					Sender:    sender,
					Timestamp: ts,
					IsYou:     false,
					IsFile:    true,
					Filename:  fn,
					FileSize:  len(fData),
					SavedPath: savePath,
				})
			} else {
				chatMessages = append(chatMessages, tui.ChatMessageItem{
					Sender:    sender,
					Text:      string(plaintext),
					Timestamp: ts,
					IsYou:     false,
				})
			}
		}
	}

	stopCh := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Clear Screen
	fmt.Fprintf(ui.Out, "\033[2J\033[H")

	redraw := func(input string) {
		fmt.Fprintf(ui.Out, "\033[H")
		frame := tui.RenderFullTUI(
			localIdFile.Handle,
			localIdFile.Fingerprint,
			contacts,
			targetHandle,
			chatMessages,
			secState,
			input,
			95,
		)
		fmt.Fprintf(ui.Out, "%s", frame)
	}

	redraw("")

	// 5. Stream Listener
	go func() {
		_ = apiClient.ListenStream(localIdFile.Handle, func(msg client.StreamEvent) {
			if msg.Sender != "" && !strings.EqualFold(msg.Sender, targetHandle) {
				return
			}
			plaintext, err := crypto.Decrypt([]byte(msg.Ciphertext), devIdentity)
			if err != nil {
				return
			}
			fn, fData, isFile := crypto.DecodeFilePayload(plaintext)
			sender := msg.Sender
			if sender == "" {
				sender = targetHandle
			}
			ts := time.Now().Format("15:04")
			if isFile {
				_ = os.MkdirAll("./downloads", 0755)
				savePath := filepath.Join("./downloads", fn)
				_ = os.WriteFile(savePath, fData, 0600)
				chatMessages = append(chatMessages, tui.ChatMessageItem{
					Sender:    sender,
					Timestamp: ts,
					IsYou:     false,
					IsFile:    true,
					Filename:  fn,
					FileSize:  len(fData),
					SavedPath: savePath,
				})
			} else {
				chatMessages = append(chatMessages, tui.ChatMessageItem{
					Sender:    sender,
					Text:      string(plaintext),
					Timestamp: ts,
					IsYou:     false,
				})
			}
			redraw("")
		}, stopCh)
	}()

	// 6. Interactive Input Loop
	scanner := bufio.NewScanner(ui.In)
	go func() {
		<-sigCh
		close(stopCh)
		fmt.Fprintf(ui.Out, "\n\n%s[i] TUI session closed.%s\n", ColorYellow, ColorReset)
		os.Exit(0)
	}()

	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			redraw("")
			continue
		}

		if text == "/quit" || text == "/exit" {
			close(stopCh)
			fmt.Fprintf(ui.Out, "\n%s[i] Pandora TUI session closed.%s\n", ColorYellow, ColorReset)
			return 0
		}

		isFileCmd := false
		var filePath string

		if text == "/f" || text == "/file" || text == "/attach" || text == "/sendfile" {
			isFileCmd = true
			filePath = openNativeFileDialog(ui)
			if filePath == "" {
				redraw("")
				continue
			}
		} else if strings.HasPrefix(text, "/f ") || strings.HasPrefix(text, "/file ") || strings.HasPrefix(text, "/attach ") || strings.HasPrefix(text, "/sendfile ") {
			isFileCmd = true
			parts := strings.SplitN(text, " ", 2)
			if len(parts) > 1 {
				filePath = strings.TrimSpace(parts[1])
			}
		}

		var ciphertext []byte
		var displayMsg string
		ts := time.Now().Format("15:04")

		if isFileCmd {
			fBytes, err := os.ReadFile(filePath)
			if err != nil {
				redraw("")
				continue
			}
			fn := filepath.Base(filePath)
			displayMsg = fmt.Sprintf("📁 [FILE SENT] %s", fn)
			ciphertext, err = crypto.EncryptFilePayload(fn, fBytes, targetPubKey)
			if err == nil {
				chatMessages = append(chatMessages, tui.ChatMessageItem{
					Sender:    "YOU",
					Timestamp: ts,
					IsYou:     true,
					IsFile:    true,
					Filename:  fn,
					FileSize:  len(fBytes),
				})
			}
		} else {
			displayMsg = text
			ciphertext, err = crypto.Encrypt([]byte(text), targetPubKey)
			if err == nil {
				chatMessages = append(chatMessages, tui.ChatMessageItem{
					Sender:    "YOU",
					Text:      displayMsg,
					Timestamp: ts,
					IsYou:     true,
				})
			}
		}

		if ciphertext != nil {
			_, _ = apiClient.PostChatMessage(targetHandle, localIdFile.Handle, string(ciphertext))
		}

		redraw("")
	}

	close(stopCh)
	return 0
}
