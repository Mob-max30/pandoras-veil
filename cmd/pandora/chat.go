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

// ANSI Color Theme matching the reference screenshot
const (
	ThemeCyan    = "\033[38;2;0;210;255m"
	ThemeGreen   = "\033[38;2;50;255;120m"
	ThemeMagenta = "\033[38;2;255;85;210m"
	ThemeDim     = "\033[38;2;110;120;140m"
	ThemeWhite   = "\033[38;2;235;240;250m"
	ThemeYellow  = "\033[38;2;255;215;0m"
	ThemeDarkBg  = "\033[48;2;16;18;27m"
	ThemeSelBg   = "\033[48;2;50;255;120m\033[38;2;16;18;27m\033[1m"
)

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

func padCenter(s string, w int) string {
	vis := visualWidth(s)
	if vis >= w {
		return s
	}
	left := (w - vis) / 2
	right := w - vis - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

func padTrunc(s string, w int) string {
	vis := visualWidth(s)
	if vis > w {
		if w > 3 {
			return s[:w-3] + ".."
		}
		return s[:w]
	}
	return s + strings.Repeat(" ", w-vis)
}

func (s *TUIState) render() {
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width < 80 || height < 20 {
		width = 110
		height = 30
	}

	// Dynamic column widths: Left (24%), Center (52%), Right (24%)
	leftW := width * 23 / 100
	if leftW < 22 {
		leftW = 22
	}
	rightW := width * 25 / 100
	if rightW < 24 {
		rightW = 24
	}
	centerW := width - leftW - rightW - 4
	if centerW < 36 {
		centerW = 36
	}

	// Height breakdown
	mainH := height - 7 // Top title (1) + gaps + bottom input box (4) + footer (1)
	if mainH < 12 {
		mainH = 12
	}

	var b strings.Builder
	// Move cursor to top-left, hide cursor during draw
	b.WriteString("\033[H\033[?25l")

	// -------------------------------------------------------------
	// 1. TOP TITLE BAR
	// -------------------------------------------------------------
	titleText := fmt.Sprintf("🔴 🟡 🟢   %sPANDORA'S VEIL%s  |  %s[ v1.2.5 ]%s Secure Channel - %sMain%s",
		ThemeCyan+ColorBold, ColorReset,
		ThemeGreen, ColorReset,
		ThemeWhite+ColorBold, ColorReset,
	)
	b.WriteString(padCenter(titleText, width) + "\n\n")

	// -------------------------------------------------------------
	// 2. PREPARE LEFT COLUMN (DEVICE IDENTITY + CHANNELS)
	// -------------------------------------------------------------
	leftLines := make([]string, mainH)
	box1H := 6
	box2H := mainH - box1H - 1

	// Box 1: Device Identity
	leftLines[0] = fmt.Sprintf("%s╭─ DEVICE IDENTITY %s╮%s", ThemeCyan, strings.Repeat("─", leftW-20), ColorReset)
	leftLines[1] = fmt.Sprintf("%s│%s %s%s│%s", ThemeCyan, ColorReset, padRight("HOST: "+ThemeWhite+ColorBold+padTrunc(s.userHandle, leftW-9)+ColorReset, leftW-3), ThemeCyan, ColorReset)
	leftLines[2] = fmt.Sprintf("%s│%s %s%s│%s", ThemeCyan, ColorReset, padRight("FINGERPRINT:", leftW-3), ThemeCyan, ColorReset)
	leftLines[3] = fmt.Sprintf("%s│%s %s%s│%s", ThemeCyan, ColorReset, padRight(ThemeYellow+padTrunc(s.userFP, leftW-3)+ColorReset, leftW-3), ThemeCyan, ColorReset)
	statusText := fmt.Sprintf("STATUS: %sONLINE%s %s(AES-256)%s", ThemeGreen+ColorBold, ColorReset, ThemeDim, ColorReset)
	leftLines[4] = fmt.Sprintf("%s│%s %s%s│%s", ThemeCyan, ColorReset, padRight(statusText, leftW-3), ThemeCyan, ColorReset)
	leftLines[5] = fmt.Sprintf("%s╰%s╯%s", ThemeCyan, strings.Repeat("─", leftW-2), ColorReset)

	// Box 2: Channels / Contacts
	if box2H > 4 {
		leftLines[6] = fmt.Sprintf("%s╭─ CHANNELS %s╮%s", ThemeCyan, strings.Repeat("─", leftW-13), ColorReset)
		leftLines[7] = fmt.Sprintf("%s│%s %s%s│%s", ThemeCyan, ColorReset, padRight(ThemeDim+"Active Messages"+ColorReset, leftW-3), ThemeCyan, ColorReset)
		activeMsgLine := fmt.Sprintf("  %s●%s %s", ThemeGreen, ColorReset, padTrunc(s.targetLabel, leftW-7))
		leftLines[8] = fmt.Sprintf("%s│%s %s%s│%s", ThemeCyan, ColorReset, padRight(activeMsgLine, leftW-3), ThemeCyan, ColorReset)
		leftLines[9] = fmt.Sprintf("%s│%s %s%s│%s", ThemeCyan, ColorReset, padRight(ThemeDim+"Group Chats"+ColorReset, leftW-3), ThemeCyan, ColorReset)

		activeChanName := "#Development"
		if s.isGroup {
			activeChanName = "#" + s.targetLabel
		}
		chanHighlight := fmt.Sprintf(" %s ● ", padTrunc(activeChanName, leftW-8))
		leftLines[10] = fmt.Sprintf("%s│%s%s%s│%s", ThemeCyan, ColorReset, ThemeSelBg+padRight(chanHighlight, leftW-2)+ColorReset, ThemeCyan, ColorReset)
		
		groupList := []string{"#Alpha_Team", "#Ops_Center", "#General"}
		for i, gName := range groupList {
			rowIdx := 11 + i
			if rowIdx < mainH-1 {
				gLine := fmt.Sprintf("  %s%s%s", ThemeDim, padTrunc(gName, leftW-5), ColorReset)
				leftLines[rowIdx] = fmt.Sprintf("%s│%s %s%s│%s", ThemeCyan, ColorReset, padRight(gLine, leftW-3), ThemeCyan, ColorReset)
			}
		}
		leftLines[mainH-1] = fmt.Sprintf("%s╰%s╯%s", ThemeCyan, strings.Repeat("─", leftW-2), ColorReset)
	}

	// -------------------------------------------------------------
	// 3. PREPARE RIGHT COLUMN (POLICIES & METADATA)
	// -------------------------------------------------------------
	rightLines := make([]string, mainH)
	
	// Box 1: Policy Inspector
	rightLines[0] = fmt.Sprintf("%s╭─ SECRET DEPOSIT & POLICY %s╮%s", ThemeMagenta, strings.Repeat("─", rightW-28), ColorReset)
	rightLines[1] = fmt.Sprintf("%s│%s %s%s│%s", ThemeMagenta, ColorReset, padRight("POLICY: "+ThemeGreen+"BURST_MODE_ALPHA"+ColorReset, rightW-3), ThemeMagenta, ColorReset)
	rightLines[2] = fmt.Sprintf("%s╰%s╯%s", ThemeMagenta, strings.Repeat("─", rightW-2), ColorReset)

	// Box 2: Deposit Object & TTL
	rightLines[3] = fmt.Sprintf("%s╭─ DEPOSIT OBJECT %s╮%s", ThemeMagenta, strings.Repeat("─", rightW-19), ColorReset)
	rightLines[4] = fmt.Sprintf("%s│%s %s%s│%s", ThemeMagenta, ColorReset, padRight("  "+ThemeWhite+ColorBold+"Auth_Key_74"+ColorReset, rightW-3), ThemeMagenta, ColorReset)
	rightLines[5] = fmt.Sprintf("%s│%s %s%s│%s", ThemeMagenta, ColorReset, padRight("  TTL EXPIRATION", rightW-3), ThemeMagenta, ColorReset)
	ttlLine := fmt.Sprintf("  [ 60s | %s*%s*%s | 1h | 24h ]", ThemeGreen+ColorBold, s.ttlSetting, ColorReset)
	rightLines[6] = fmt.Sprintf("%s│%s %s%s│%s", ThemeMagenta, ColorReset, padRight(ttlLine, rightW-3), ThemeMagenta, ColorReset)
	rightLines[7] = fmt.Sprintf("%s╰%s╯%s", ThemeMagenta, strings.Repeat("─", rightW-2), ColorReset)

	// Box 3: Burn After Reading
	rightLines[8] = fmt.Sprintf("%s╭─ BURN-AFTER-READING %s╮%s", ThemeMagenta, strings.Repeat("─", rightW-23), ColorReset)
	burnToggle := fmt.Sprintf("  Redis GETDEL   %s[===●]%s", ThemeGreen, ColorReset)
	rightLines[9] = fmt.Sprintf("%s│%s %s%s│%s", ThemeMagenta, ColorReset, padRight(burnToggle, rightW-3), ThemeMagenta, ColorReset)
	burnState := fmt.Sprintf("  [ %s*%s*%s | OFF ]", ThemeGreen+ColorBold, s.burnSetting, ColorReset)
	rightLines[10] = fmt.Sprintf("%s│%s %s%s│%s", ThemeMagenta, ColorReset, padRight(burnState, rightW-3), ThemeMagenta, ColorReset)
	rightLines[11] = fmt.Sprintf("%s╰%s╯%s", ThemeMagenta, strings.Repeat("─", rightW-2), ColorReset)

	// Box 4: Key Metadata
	if mainH > 16 {
		rightLines[12] = fmt.Sprintf("%s╭─ KEY METADATA %s╮%s", ThemeMagenta, strings.Repeat("─", rightW-17), ColorReset)
		rightLines[13] = fmt.Sprintf("%s│%s %s%s│%s", ThemeMagenta, ColorReset, padRight("  Created:   "+padTrunc(s.userHandle, rightW-14), rightW-3), ThemeMagenta, ColorReset)
		rightLines[14] = fmt.Sprintf("%s│%s %s%s│%s", ThemeMagenta, ColorReset, padRight("  Expires:   24 Hours", rightW-3), ThemeMagenta, ColorReset)
		rightLines[15] = fmt.Sprintf("%s│%s %s%s│%s", ThemeMagenta, ColorReset, padRight("  Recipient: "+padTrunc(s.targetLabel, rightW-14), rightW-3), ThemeMagenta, ColorReset)
		rightLines[16] = fmt.Sprintf("%s│%s %s%s│%s", ThemeMagenta, ColorReset, padRight("  Cipher:    AES-256", rightW-3), ThemeMagenta, ColorReset)
		rightLines[17] = fmt.Sprintf("%s╰%s╯%s", ThemeMagenta, strings.Repeat("─", rightW-2), ColorReset)
	}

	// -------------------------------------------------------------
	// 4. PREPARE CENTER MAIN PANE (SPEECH BUBBLE MESSAGES)
	// -------------------------------------------------------------
	centerLines := make([]string, mainH)
	centerLines[0] = fmt.Sprintf("%s╭─ CENTER MAIN PANE (Width: 55%%) %s╮%s", ThemeGreen, strings.Repeat("─", centerW-32), ColorReset)

	// Render speech bubble messages inside the center pane
	chatRowsAvailable := mainH - 2
	var renderedBubbleLines []string

	for _, msg := range s.messages {
		if msg.IsOutgoing {
			// Outgoing Bubble (Right Aligned, Green Border)
			bubbleW := len(msg.Text) + 6
			if bubbleW < 24 {
				bubbleW = 24
			}
			if bubbleW > centerW-4 {
				bubbleW = centerW - 4
			}
			headerText := fmt.Sprintf("[%s] [YOU]", msg.Timestamp)
			topBorder := fmt.Sprintf("╭%s%s%s%s╮",
				strings.Repeat("─", bubbleW-len(headerText)-3),
				ThemeDim, headerText, ThemeGreen,
			)
			bodyText := padRight(" "+msg.Text, bubbleW-2)
			botBorder := fmt.Sprintf("╰%s╯", strings.Repeat("─", bubbleW-2))

			padLeftCount := centerW - bubbleW - 3
			if padLeftCount < 0 {
				padLeftCount = 0
			}
			indent := strings.Repeat(" ", padLeftCount)

			renderedBubbleLines = append(renderedBubbleLines, indent+ThemeGreen+topBorder+ColorReset)
			renderedBubbleLines = append(renderedBubbleLines, indent+ThemeGreen+"│"+ColorReset+ThemeWhite+bodyText+ThemeGreen+"│"+ColorReset)
			renderedBubbleLines = append(renderedBubbleLines, indent+ThemeGreen+botBorder+ColorReset)
		} else {
			// Incoming Bubble (Left Aligned, Cyan/Dim Border)
			bubbleW := len(msg.Text) + len(msg.Sender) + 12
			if bubbleW < 26 {
				bubbleW = 26
			}
			if bubbleW > centerW-4 {
				bubbleW = centerW - 4
			}
			headerText := fmt.Sprintf("[%s] %s", msg.Timestamp, msg.Sender)
			topBorder := fmt.Sprintf("╭%s%s%s %s%s╮",
				ThemeDim, headerText, ThemeCyan,
				strings.Repeat("─", bubbleW-len(headerText)-4),
				ThemeCyan,
			)
			bodyText := padRight(" "+msg.Text, bubbleW-2)
			botBorder := fmt.Sprintf("╰%s╯", strings.Repeat("─", bubbleW-2))

			renderedBubbleLines = append(renderedBubbleLines, " "+ThemeCyan+topBorder+ColorReset)
			renderedBubbleLines = append(renderedBubbleLines, " "+ThemeCyan+"│"+ColorReset+ThemeWhite+bodyText+ThemeCyan+"│"+ColorReset)
			renderedBubbleLines = append(renderedBubbleLines, " "+ThemeCyan+botBorder+ColorReset)
		}
	}

	// Slice visible bubble lines to fit center pane height
	visibleBubbles := renderedBubbleLines
	if len(visibleBubbles) > chatRowsAvailable {
		visibleBubbles = visibleBubbles[len(visibleBubbles)-chatRowsAvailable:]
	}

	startRow := chatRowsAvailable - len(visibleBubbles)
	for i, line := range visibleBubbles {
		row := 1 + startRow + i
		if row < mainH-1 {
			centerLines[row] = fmt.Sprintf("%s│%s%s%s│%s", ThemeGreen, ColorReset, padRight(line, centerW-2), ThemeGreen, ColorReset)
		}
	}

	// Fill empty center lines
	for r := 1; r < mainH-1; r++ {
		if centerLines[r] == "" {
			centerLines[r] = fmt.Sprintf("%s│%s%s%s│%s", ThemeGreen, ColorReset, strings.Repeat(" ", centerW-2), ThemeGreen, ColorReset)
		}
	}
	centerLines[mainH-1] = fmt.Sprintf("%s╰%s╯%s", ThemeGreen, strings.Repeat("─", centerW-2), ColorReset)

	// -------------------------------------------------------------
	// 5. ASSEMBLE 3 COLUMNS ROW BY ROW
	// -------------------------------------------------------------
	for row := 0; row < mainH; row++ {
		l := padRight(leftLines[row], leftW)
		c := padRight(centerLines[row], centerW)
		r := padRight(rightLines[row], rightW)
		b.WriteString(fmt.Sprintf(" %s  %s  %s\n", l, c, r))
	}

	// -------------------------------------------------------------
	// 6. BOTTOM INPUT BOX & COMMAND PROMPT
	// -------------------------------------------------------------
	chanLabel := "#Development"
	if s.isGroup {
		chanLabel = "#" + s.targetLabel
	}
	inputBoxTop := fmt.Sprintf("%s╭─ [ %s%s%s ] %s╮%s",
		ThemeCyan, ThemeWhite+ColorBold, chanLabel, ThemeCyan,
		strings.Repeat("─", width-len(chanLabel)-13), ColorReset,
	)
	b.WriteString("\n " + inputBoxTop + "\n")

	promptStr := fmt.Sprintf("pveil > ")
	inputText := s.inputBuffer
	maxInput := width - 14
	if len(inputText) > maxInput {
		inputText = inputText[len(inputText)-maxInput:]
	}
	inputLineContent := padRight(fmt.Sprintf(" %s%s%s%s", ThemeGreen+ColorBold, promptStr, ColorReset+ThemeWhite, inputText), width-4)
	b.WriteString(fmt.Sprintf(" %s│%s%s%s│%s\n", ThemeCyan, ColorReset, inputLineContent, ThemeCyan, ColorReset))
	b.WriteString(" " + fmt.Sprintf("%s╰%s╯%s\n", ThemeCyan, strings.Repeat("─", width-4), ColorReset))

	// -------------------------------------------------------------
	// 7. FOOTER SHORTCUT BAR
	// -------------------------------------------------------------
	shortcutBar := fmt.Sprintf(" %s[Tab]%s Switch Pane    %s[Ctrl+N]%s New Group    %s[Ctrl+S]%s Search    %s[Ctrl+K]%s SecDeposit    %s[Ctrl+Q]%s Exit",
		ThemeCyan, ThemeDim,
		ThemeCyan, ThemeDim,
		ThemeCyan, ThemeDim,
		ThemeCyan, ThemeDim,
		ThemeCyan, ThemeDim,
	)
	b.WriteString(shortcutBar + "\033[?25h")

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

	// 5. Enter Alternate Screen Buffer (Cyberpunk Dashboard Theme)
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
		ttlSetting:  "300s",
		burnSetting: "ON",
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
			timestamp := time.Now().Format("15:04")
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
				Timestamp:  time.Now().Format("15:04"),
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
				Timestamp:  time.Now().Format("15:04"),
				Sender:     "SYSTEM",
				Text:       fmt.Sprintf("Delivery failed: %v", err),
				IsOutgoing: false,
			})
			state.SetInput("")
			continue
		}

		timestamp := time.Now().Format("15:04")
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
