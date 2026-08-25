package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/Mob-max30/pandoras-veil/internal/client"
	"github.com/Mob-max30/pandoras-veil/internal/crypto"
	"github.com/Mob-max30/pandoras-veil/internal/storage"
	"golang.org/x/term"
)

type groupMember struct {
	handle      string
	publicKey   string
	fingerprint string
}

type ChatMessage struct {
	Timestamp  string
	Sender     string
	Text       string
	IsOutgoing bool
}

type TUIState struct {
	mu          sync.Mutex
	userHandle  string
	userFP      string
	userKey     string
	targetLabel string
	targetFP    string
	isGroup     bool
	members     []groupMember
	ttlSetting  string
	burnSetting string
	relayURL    string
	messages    []ChatMessage
	inputBuffer string
	out         io.Writer
}

func (s *TUIState) AddMessage(msg ChatMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msg)
	s.render()
}

func (s *TUIState) SetInput(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inputBuffer = text
	s.render()
}

func visualWidth(str string) int {
	return utf8.RuneCountInString(stripANSI(str))
}

func padRight(s string, w int) string {
	vis := visualWidth(s)
	if vis >= w {
		return s
	}
	return s + strings.Repeat(" ", w-vis)
}

func (s *TUIState) render() {
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width < 60 || height < 15 {
		width = 90
		height = 25
	}

	leftW := width * 24 / 100
	if leftW < 22 {
		leftW = 22
	}
	if leftW > 32 {
		leftW = 32
	}
	rightW := width - leftW - 3
	if rightW < 30 {
		rightW = 30
	}

	totalInnerH := height - 5
	if totalInnerH < 6 {
		totalInnerH = 6
	}

	leftH1 := totalInnerH / 2
	leftH2 := totalInnerH - leftH1 - 1 // 1 row for middle divider

	var b strings.Builder
	// Move cursor to top-left, hide cursor during frame draw
	b.WriteString("\033[H\033[?25l")

	// 1. Top Header Row
	topHeader := fmt.Sprintf("┌%s┬%s┐\n", strings.Repeat("─", leftW), strings.Repeat("─", rightW))
	b.WriteString(ColorDim + topHeader + ColorReset)

	titleLeft := padRight(fmt.Sprintf(" %s🔒 PANDORA RELAY%s", ColorBold+ColorCyan, ColorReset), leftW)
	headerTarget := fmt.Sprintf(" %s💬 SESSION: %s%s  |  FP: [%s%s%s]  |  %s🔒 E2E ENCRYPTED%s",
		ColorBold+ColorGreen, s.targetLabel, ColorReset,
		ColorYellow, s.targetFP, ColorReset,
		ColorGreen, ColorReset,
	)
	if s.isGroup {
		headerTarget = fmt.Sprintf(" %s🔒 GROUP (%d Members)%s  |  %s🔒 E2E MULTI-KEY ENCRYPTED%s",
			ColorBold+ColorGreen, len(s.members)+1, ColorReset,
			ColorGreen, ColorReset,
		)
	}
	titleRight := padRight(headerTarget, rightW)

	b.WriteString(fmt.Sprintf("%s│%s%s%s│%s%s%s│%s\n",
		ColorDim, ColorReset, titleLeft, ColorDim, ColorReset, titleRight, ColorDim, ColorReset,
	))

	// 2. Header Divider
	b.WriteString(fmt.Sprintf("%s├%s┼%s┤%s\n",
		ColorDim, strings.Repeat("─", leftW), strings.Repeat("─", rightW), ColorReset,
	))

	// 3. Prepare Left Sidebar Lines
	leftBox1 := make([]string, leftH1)
	leftBox1[0] = ColorBold + ColorWhite + " [YOUR IDENTITY]" + ColorReset
	leftBox1[1] = ColorCyan + " Handle: " + ColorReset + s.userHandle
	leftBox1[2] = ColorYellow + " FP: " + ColorReset + s.userFP
	keySnip := s.userKey
	if len(keySnip) > 10 {
		keySnip = keySnip[:5] + ".." + keySnip[len(keySnip)-4:]
	}
	if leftH1 > 3 {
		leftBox1[3] = ColorDim + " Key: " + keySnip + ColorReset
	}
	if leftH1 > 4 {
		leftBox1[4] = ColorGreen + " Status: Online" + ColorReset
	}

	leftBox2 := make([]string, leftH2)
	leftBox2[0] = ColorBold + ColorWhite + " [SETTINGS & POLICY]" + ColorReset
	leftBox2[1] = ColorCyan + " Disappear: " + ColorReset + s.ttlSetting
	leftBox2[2] = ColorCyan + " Burn Read: " + ColorReset + s.burnSetting
	if s.isGroup && leftH2 > 3 {
		leftBox2[3] = ColorDim + fmt.Sprintf(" Group: %d Peers", len(s.members)) + ColorReset
	} else if leftH2 > 3 {
		leftBox2[3] = ColorDim + " Cipher: age/X25519" + ColorReset
	}
	if leftH2 > 4 {
		leftBox2[4] = ColorDim + " Exit: /quit" + ColorReset
	}

	// 4. Prepare Right Chat Lines
	visibleMsgs := s.messages
	if len(visibleMsgs) > totalInnerH {
		visibleMsgs = visibleMsgs[len(visibleMsgs)-totalInnerH:]
	}

	chatLines := make([]string, totalInnerH)
	startIdx := totalInnerH - len(visibleMsgs)
	for i, msg := range visibleMsgs {
		idx := startIdx + i
		if idx >= 0 && idx < totalInnerH {
			if msg.IsOutgoing {
				// Outgoing right-aligned bubble
				formatted := fmt.Sprintf("%s%s%s %s[YOU]%s %s[%s]%s",
					ColorBold+ColorGreen, msg.Text, ColorReset,
					ColorBold+ColorMagenta, ColorReset,
					ColorDim, msg.Timestamp, ColorReset,
				)
				visLen := visualWidth(formatted)
				pad := rightW - visLen - 2
				if pad < 1 {
					pad = 1
				}
				chatLines[idx] = strings.Repeat(" ", pad) + formatted
			} else {
				// Incoming left-aligned bubble
				formatted := fmt.Sprintf(" %s[%s]%s %s[%s] ❯%s %s",
					ColorDim, msg.Timestamp, ColorReset,
					ColorBold+ColorMagenta, msg.Sender, ColorReset,
					msg.Text,
				)
				chatLines[idx] = formatted
			}
		}
	}

	// 5. Render Main Content (Row by Row)
	for row := 0; row < totalInnerH; row++ {
		var leftContent string
		if row < leftH1 {
			leftContent = leftBox1[row]
		} else if row == leftH1 {
			// Middle horizontal divider on left box
			leftContent = ColorDim + strings.Repeat("─", leftW) + ColorReset
		} else {
			box2Idx := row - leftH1 - 1
			if box2Idx < len(leftBox2) {
				leftContent = leftBox2[box2Idx]
			}
		}

		leftPadded := padRight(leftContent, leftW)
		rightPadded := padRight(chatLines[row], rightW)

		b.WriteString(fmt.Sprintf("%s│%s%s%s│%s%s%s│%s\n",
			ColorDim, ColorReset, leftPadded, ColorDim, ColorReset, rightPadded, ColorDim, ColorReset,
		))
	}

	// 6. Bottom Separator
	b.WriteString(fmt.Sprintf("%s├%s┴%s┤%s\n",
		ColorDim, strings.Repeat("─", leftW), strings.Repeat("─", rightW), ColorReset,
	))

	// 7. Input Line Row
	inputPrompt := fmt.Sprintf(" [%s] ❯ ", s.userHandle)
	inputDisplay := s.inputBuffer
	maxInputW := width - len(inputPrompt) - 4
	if maxInputW > 0 && len(inputDisplay) > maxInputW {
		inputDisplay = inputDisplay[len(inputDisplay)-maxInputW:]
	}

	inputLine := padRight(fmt.Sprintf("%s%s%s%s", ColorBold+ColorCyan, inputPrompt, ColorReset, inputDisplay), width-2)
	b.WriteString(fmt.Sprintf("%s│%s%s%s│%s\n", ColorDim, ColorReset, inputLine, ColorDim, ColorReset))

	// 8. Bottom Frame Border
	b.WriteString(fmt.Sprintf("%s└%s┘%s\033[?25h", ColorDim, strings.Repeat("─", width-2), ColorReset))

	fmt.Fprint(s.out, b.String())
}

func stripANSI(str string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range str {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
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

	// 5. Enter Alternate Screen Buffer (Dedicated Full Screen Layout)
	fmt.Fprint(ui.Out, "\033[?1049h\033[2J\033[H")
	defer fmt.Fprint(ui.Out, "\033[?1049l\033[?25h")

	targetLabel := recipientHandles[0]
	targetFP := members[0].fingerprint
	if isGroup {
		targetLabel = strings.Join(recipientHandles, ", ")
		targetFP = fmt.Sprintf("%d Members", len(members))
	}

	state := &TUIState{
		userHandle:  localIdFile.Handle,
		userFP:      localIdFile.Fingerprint,
		userKey:     localIdFile.PublicKey,
		targetLabel: targetLabel,
		targetFP:    targetFP,
		isGroup:     isGroup,
		members:     members,
		ttlSetting:  "24 Hours",
		burnSetting: "Disabled",
		relayURL:    *relayFlag,
		messages:    make([]ChatMessage, 0),
		out:         ui.Out,
	}

	state.render()

	stopCh := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// 6. Background Listener Goroutine (SSE Stream - Session Isolation Protected)
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
				return // Drop messages from outside the active session
			}

			plaintext, err := crypto.Decrypt([]byte(msg.Ciphertext), devIdentity)
			if err != nil {
				return
			}
			timestamp := time.Now().Format("15:04:05")
			senderName := msg.Sender
			if senderName == "" {
				senderName = recipientHandles[0]
			}

			state.AddMessage(ChatMessage{
				Timestamp:  timestamp,
				Sender:     senderName,
				Text:       string(plaintext),
				IsOutgoing: false,
			})
		}, stopCh)
	}()

	go func() {
		<-sigCh
		close(stopCh)
		fmt.Fprint(ui.Out, "\033[?1049l\033[?25h")
		os.Exit(0)
	}()

	// 7. Foreground Input Handling Loop
	scanner := bufio.NewScanner(ui.In)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			state.SetInput("")
			continue
		}

		if text == "/quit" || text == "/exit" {
			close(stopCh)
			return 0
		}

		// Encrypt message locally using age recipient-based encryption
		var ciphertext []byte
		if isGroup {
			pubKeys := make([]string, len(members))
			for i, m := range members {
				pubKeys[i] = m.publicKey
			}
			ciphertext, err = crypto.EncryptMulti([]byte(text), pubKeys...)
		} else {
			ciphertext, err = crypto.Encrypt([]byte(text), members[0].publicKey)
		}
		if err != nil {
			state.AddMessage(ChatMessage{
				Timestamp:  time.Now().Format("15:04:05"),
				Sender:     "SYSTEM",
				Text:       fmt.Sprintf("Encryption error: %v", err),
				IsOutgoing: false,
			})
			state.SetInput("")
			continue
		}

		// Send live message envelope(s) to relay
		if isGroup {
			_, err = apiClient.PostGroupChatMessage(recipientHandles, localIdFile.Handle, string(ciphertext))
		} else {
			_, err = apiClient.PostChatMessage(recipientHandles[0], localIdFile.Handle, string(ciphertext))
		}
		if err != nil {
			state.AddMessage(ChatMessage{
				Timestamp:  time.Now().Format("15:04:05"),
				Sender:     "SYSTEM",
				Text:       fmt.Sprintf("Delivery failed: %v", err),
				IsOutgoing: false,
			})
			state.SetInput("")
			continue
		}

		timestamp := time.Now().Format("15:04:05")
		state.AddMessage(ChatMessage{
			Timestamp:  timestamp,
			Sender:     localIdFile.Handle,
			Text:       text,
			IsOutgoing: true,
		})
		state.SetInput("")
	}

	close(stopCh)
	return 0
}
