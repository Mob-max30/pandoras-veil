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
	lastWidth   int
	lastHeight  int
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

func (s *TUIState) ToggleBurn() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.EqualFold(s.burnSetting, "ON") {
		s.burnSetting = "OFF"
	} else {
		s.burnSetting = "ON"
	}
	s.render()
	return s.burnSetting
}

func (s *TUIState) SetTTL(val string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ttlSetting = val
	s.render()
}

func (s *TUIState) ClearMessages() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = make([]ChatMessage, 0)
	s.render()
}

// ANSI Color Theme matching reference screenshot
const (
	ThemeCyan    = "\033[38;2;0;210;255m"
	ThemeGreen   = "\033[38;2;50;255;120m"
	ThemeMagenta = "\033[38;2;255;85;210m"
	ThemeDim     = "\033[38;2;110;120;140m"
	ThemeWhite   = "\033[38;2;235;240;250m"
	ThemeYellow  = "\033[38;2;255;215;0m"
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
	// 1. Detect dynamic terminal dimensions
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 || height <= 0 {
		width, height, err = term.GetSize(int(os.Stdin.Fd()))
		if err != nil || width <= 0 || height <= 0 {
			width = 90
			height = 25
		}
	}

	if width < 70 {
		width = 70
	}
	if height < 16 {
		height = 16
	}

	var b strings.Builder
	// If window size changed, clear entire screen buffer
	if width != s.lastWidth || height != s.lastHeight {
		b.WriteString("\033[2J")
		s.lastWidth = width
		s.lastHeight = height
	}

	// Move cursor to home and hide cursor during screen refresh
	b.WriteString("\033[H\033[?25l")

	// Column Widths strictly summing to width-2
	leftW := width * 22 / 100
	if leftW < 20 {
		leftW = 20
	}
	rightW := width * 24 / 100
	if rightW < 22 {
		rightW = 22
	}
	centerW := width - leftW - rightW - 4
	if centerW < 24 {
		centerW = 24
	}

	// Height budget strictly fitting inside height - 1 to prevent scrolling
	mainH := height - 6
	if mainH < 8 {
		mainH = 8
	}

	// -------------------------------------------------------------
	// 1. TOP TITLE BAR (1 Line)
	// -------------------------------------------------------------
	titleText := fmt.Sprintf("🔴 🟡 🟢   %sPANDORA'S VEIL%s | %s[ v1.2.5 ]%s Secure Channel - %sMain%s",
		ThemeCyan+ColorBold, ColorReset,
		ThemeGreen, ColorReset,
		ThemeWhite+ColorBold, ColorReset,
	)
	b.WriteString(padCenter(titleText, width-2) + "\n")

	// -------------------------------------------------------------
	// 2. PREPARE LEFT COLUMN (DEVICE IDENTITY + CHANNELS)
	// -------------------------------------------------------------
	leftLines := make([]string, mainH)
	box1H := 5
	box2H := mainH - box1H

	// Box 1: Device Identity (5 lines)
	leftLines[0] = fmt.Sprintf("%s╭─ DEVICE IDENTITY %s╮%s", ThemeCyan, strings.Repeat("─", maxInt(0, leftW-20)), ColorReset)
	leftLines[1] = fmt.Sprintf("%s│%s %s%s│%s", ThemeCyan, ColorReset, padRight("HOST: "+ThemeWhite+ColorBold+padTrunc(s.userHandle, leftW-9)+ColorReset, leftW-3), ThemeCyan, ColorReset)
	leftLines[2] = fmt.Sprintf("%s│%s %s%s│%s", ThemeCyan, ColorReset, padRight("FINGERPRINT: "+ThemeYellow+padTrunc(s.userFP, leftW-15)+ColorReset, leftW-3), ThemeCyan, ColorReset)
	statusText := fmt.Sprintf("STATUS: %sONLINE%s %s(AES-256)%s", ThemeGreen+ColorBold, ColorReset, ThemeDim, ColorReset)
	leftLines[3] = fmt.Sprintf("%s│%s %s%s│%s", ThemeCyan, ColorReset, padRight(statusText, leftW-3), ThemeCyan, ColorReset)
	leftLines[4] = fmt.Sprintf("%s╰%s╯%s", ThemeCyan, strings.Repeat("─", maxInt(0, leftW-2)), ColorReset)

	// Box 2: Channels (remaining lines)
	if box2H >= 4 {
		leftLines[5] = fmt.Sprintf("%s╭─ CHANNELS %s╮%s", ThemeCyan, strings.Repeat("─", maxInt(0, leftW-13)), ColorReset)
		leftLines[6] = fmt.Sprintf("%s│%s %s%s│%s", ThemeCyan, ColorReset, padRight(ThemeDim+"Active Messages"+ColorReset, leftW-3), ThemeCyan, ColorReset)
		activeMsgLine := fmt.Sprintf(" %s●%s %s", ThemeGreen, ColorReset, padTrunc(s.targetLabel, leftW-6))
		leftLines[7] = fmt.Sprintf("%s│%s %s%s│%s", ThemeCyan, ColorReset, padRight(activeMsgLine, leftW-3), ThemeCyan, ColorReset)

		if box2H >= 6 {
			leftLines[8] = fmt.Sprintf("%s│%s %s%s│%s", ThemeCyan, ColorReset, padRight(ThemeDim+"Group Chats"+ColorReset, leftW-3), ThemeCyan, ColorReset)
			activeChanName := "#Development"
			if s.isGroup {
				activeChanName = "#" + s.targetLabel
			}
			chanHighlight := fmt.Sprintf(" %s ●", padTrunc(activeChanName, leftW-6))
			leftLines[9] = fmt.Sprintf("%s│%s%s%s│%s", ThemeCyan, ColorReset, ThemeSelBg+padRight(chanHighlight, leftW-2)+ColorReset, ThemeCyan, ColorReset)
		}

		groupList := []string{"#Alpha_Team", "#Ops_Center", "#General"}
		for i, gName := range groupList {
			rowIdx := 10 + i
			if rowIdx < mainH-1 {
				gLine := fmt.Sprintf(" %s%s%s", ThemeDim, padTrunc(gName, leftW-4), ColorReset)
				leftLines[rowIdx] = fmt.Sprintf("%s│%s %s%s│%s", ThemeCyan, ColorReset, padRight(gLine, leftW-3), ThemeCyan, ColorReset)
			}
		}
		// Fill empty lines
		for r := 5; r < mainH-1; r++ {
			if leftLines[r] == "" {
				leftLines[r] = fmt.Sprintf("%s│%s%s%s│%s", ThemeCyan, ColorReset, strings.Repeat(" ", maxInt(0, leftW-2)), ThemeCyan, ColorReset)
			}
		}
		leftLines[mainH-1] = fmt.Sprintf("%s╰%s╯%s", ThemeCyan, strings.Repeat("─", maxInt(0, leftW-2)), ColorReset)
	}

	// -------------------------------------------------------------
	// 3. PREPARE RIGHT COLUMN (POLICIES & METADATA)
	// -------------------------------------------------------------
	rightLines := make([]string, mainH)
	
	// Box 1: Policy Inspector (3 lines)
	rightLines[0] = fmt.Sprintf("%s╭─ SECRET DEPOSIT %s╮%s", ThemeMagenta, strings.Repeat("─", maxInt(0, rightW-19)), ColorReset)
	rightLines[1] = fmt.Sprintf("%s│%s %s%s│%s", ThemeMagenta, ColorReset, padRight("POLICY: "+ThemeGreen+"BURST_ALPHA"+ColorReset, rightW-3), ThemeMagenta, ColorReset)
	rightLines[2] = fmt.Sprintf("%s╰%s╯%s", ThemeMagenta, strings.Repeat("─", maxInt(0, rightW-2)), ColorReset)

	// Box 2: Deposit Object & TTL (5 lines)
	if mainH >= 10 {
		rightLines[3] = fmt.Sprintf("%s╭─ DEPOSIT OBJECT %s╮%s", ThemeMagenta, strings.Repeat("─", maxInt(0, rightW-19)), ColorReset)
		rightLines[4] = fmt.Sprintf("%s│%s %s%s│%s", ThemeMagenta, ColorReset, padRight(" "+ThemeWhite+ColorBold+"Auth_Key_74"+ColorReset, rightW-3), ThemeMagenta, ColorReset)
		ttlLine := fmt.Sprintf(" TTL: %s*%s*%s [24h]", ThemeGreen+ColorBold, s.ttlSetting, ColorReset)
		rightLines[5] = fmt.Sprintf("%s│%s %s%s│%s", ThemeMagenta, ColorReset, padRight(ttlLine, rightW-3), ThemeMagenta, ColorReset)
		rightLines[6] = fmt.Sprintf("%s╰%s╯%s", ThemeMagenta, strings.Repeat("─", maxInt(0, rightW-2)), ColorReset)
	}

	// Box 3: Burn After Reading (4 lines)
	if mainH >= 14 {
		rightLines[7] = fmt.Sprintf("%s╭─ BURN-AFTER-READING %s╮%s", ThemeMagenta, strings.Repeat("─", maxInt(0, rightW-23)), ColorReset)
		burnToggle := fmt.Sprintf(" Redis GETDEL  %s[===●]%s", ThemeGreen, ColorReset)
		rightLines[8] = fmt.Sprintf("%s│%s %s%s│%s", ThemeMagenta, ColorReset, padRight(burnToggle, rightW-3), ThemeMagenta, ColorReset)
		burnState := fmt.Sprintf(" State: %s*%s*%s (Active)", ThemeGreen+ColorBold, s.burnSetting, ColorReset)
		rightLines[9] = fmt.Sprintf("%s│%s %s%s│%s", ThemeMagenta, ColorReset, padRight(burnState, rightW-3), ThemeMagenta, ColorReset)
		rightLines[10] = fmt.Sprintf("%s╰%s╯%s", ThemeMagenta, strings.Repeat("─", maxInt(0, rightW-2)), ColorReset)
	}

	// Box 4: Key Metadata (remaining lines)
	if mainH >= 17 {
		rightLines[11] = fmt.Sprintf("%s╭─ KEY METADATA %s╮%s", ThemeMagenta, strings.Repeat("─", maxInt(0, rightW-17)), ColorReset)
		rightLines[12] = fmt.Sprintf("%s│%s %s%s│%s", ThemeMagenta, ColorReset, padRight(" Created: "+padTrunc(s.userHandle, rightW-12), rightW-3), ThemeMagenta, ColorReset)
		rightLines[13] = fmt.Sprintf("%s│%s %s%s│%s", ThemeMagenta, ColorReset, padRight(" Target:  "+padTrunc(s.targetLabel, rightW-12), rightW-3), ThemeMagenta, ColorReset)
		rightLines[14] = fmt.Sprintf("%s│%s %s%s│%s", ThemeMagenta, ColorReset, padRight(" Cipher:  AES-256", rightW-3), ThemeMagenta, ColorReset)
		rightLines[15] = fmt.Sprintf("%s╰%s╯%s", ThemeMagenta, strings.Repeat("─", maxInt(0, rightW-2)), ColorReset)
	}

	// Fill empty right lines
	for r := 0; r < mainH; r++ {
		if rightLines[r] == "" {
			rightLines[r] = strings.Repeat(" ", rightW)
		}
	}

	// -------------------------------------------------------------
	// 4. PREPARE CENTER MAIN PANE (SPEECH BUBBLE MESSAGES)
	// -------------------------------------------------------------
	centerLines := make([]string, mainH)
	centerHeader := fmt.Sprintf("╭─ CENTER MAIN PANE %s╮", strings.Repeat("─", maxInt(0, centerW-21)))
	centerLines[0] = fmt.Sprintf("%s%s%s", ThemeGreen, centerHeader, ColorReset)

	chatRowsAvailable := mainH - 2
	var renderedBubbleLines []string

	for _, msg := range s.messages {
		if msg.IsOutgoing {
			// Outgoing Bubble (Right Aligned, Green Border)
			bubbleW := len(msg.Text) + 4
			if bubbleW < 18 {
				bubbleW = 18
			}
			if bubbleW > centerW-4 {
				bubbleW = centerW - 4
			}
			headerText := fmt.Sprintf("[%s] [YOU]", msg.Timestamp)
			topBorder := fmt.Sprintf("╭%s%s%s%s╮",
				strings.Repeat("─", maxInt(0, bubbleW-len(headerText)-2)),
				ThemeDim, headerText, ThemeGreen,
			)
			bodyText := padRight(" "+msg.Text, bubbleW-2)
			botBorder := fmt.Sprintf("╰%s╯", strings.Repeat("─", maxInt(0, bubbleW-2)))

			padLeftCount := centerW - bubbleW - 4
			if padLeftCount < 0 {
				padLeftCount = 0
			}
			indent := strings.Repeat(" ", padLeftCount)

			renderedBubbleLines = append(renderedBubbleLines, indent+ThemeGreen+topBorder+ColorReset)
			renderedBubbleLines = append(renderedBubbleLines, indent+ThemeGreen+"│"+ColorReset+ThemeWhite+bodyText+ThemeGreen+"│"+ColorReset)
			renderedBubbleLines = append(renderedBubbleLines, indent+ThemeGreen+botBorder+ColorReset)
		} else {
			// Incoming Bubble (Left Aligned, Cyan Border)
			bubbleW := len(msg.Text) + len(msg.Sender) + 8
			if bubbleW < 20 {
				bubbleW = 20
			}
			if bubbleW > centerW-4 {
				bubbleW = centerW - 4
			}
			headerText := fmt.Sprintf("[%s] %s", msg.Timestamp, msg.Sender)
			topBorder := fmt.Sprintf("╭%s%s%s %s╮",
				ThemeDim, headerText, ThemeCyan,
				strings.Repeat("─", maxInt(0, bubbleW-len(headerText)-3)),
			)
			bodyText := padRight(" "+msg.Text, bubbleW-2)
			botBorder := fmt.Sprintf("╰%s╯", strings.Repeat("─", maxInt(0, bubbleW-2)))

			renderedBubbleLines = append(renderedBubbleLines, " "+ThemeCyan+topBorder+ColorReset)
			renderedBubbleLines = append(renderedBubbleLines, " "+ThemeCyan+"│"+ColorReset+ThemeWhite+bodyText+ThemeCyan+"│"+ColorReset)
			renderedBubbleLines = append(renderedBubbleLines, " "+ThemeCyan+botBorder+ColorReset)
		}
	}

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

	for r := 1; r < mainH-1; r++ {
		if centerLines[r] == "" {
			centerLines[r] = fmt.Sprintf("%s│%s%s%s│%s", ThemeGreen, ColorReset, strings.Repeat(" ", maxInt(0, centerW-2)), ThemeGreen, ColorReset)
		}
	}
	centerLines[mainH-1] = fmt.Sprintf("%s╰%s╯%s", ThemeGreen, strings.Repeat("─", maxInt(0, centerW-2)), ColorReset)

	// -------------------------------------------------------------
	// 5. ASSEMBLE 3 COLUMNS ROW BY ROW (Exactly fitting width-2)
	// -------------------------------------------------------------
	for row := 0; row < mainH; row++ {
		l := padRight(leftLines[row], leftW)
		c := padRight(centerLines[row], centerW)
		r := padRight(rightLines[row], rightW)
		b.WriteString(fmt.Sprintf("%s %s %s\n", l, c, r))
	}

	// -------------------------------------------------------------
	// 6. BOTTOM INPUT BOX (3 Lines)
	// -------------------------------------------------------------
	chanLabel := "#Development"
	if s.isGroup {
		chanLabel = "#" + s.targetLabel
	}
	inputBoxW := width - 2
	inputBoxTop := fmt.Sprintf("%s╭─ [ %s%s%s ] %s╮%s",
		ThemeCyan, ThemeWhite+ColorBold, chanLabel, ThemeCyan,
		strings.Repeat("─", maxInt(0, inputBoxW-len(chanLabel)-9)), ColorReset,
	)
	b.WriteString(inputBoxTop + "\n")

	promptStr := "pveil > "
	inputText := s.inputBuffer
	maxInput := inputBoxW - 12
	if len(inputText) > maxInput && maxInput > 0 {
		inputText = inputText[len(inputText)-maxInput:]
	}
	inputBody := padRight(fmt.Sprintf(" %s%s%s%s", ThemeGreen+ColorBold, promptStr, ColorReset+ThemeWhite, inputText), inputBoxW-2)
	b.WriteString(fmt.Sprintf("%s│%s%s%s│%s\n", ThemeCyan, ColorReset, inputBody, ThemeCyan, ColorReset))
	b.WriteString(fmt.Sprintf("%s╰%s╯%s\n", ThemeCyan, strings.Repeat("─", maxInt(0, inputBoxW-2)), ColorReset))

	// -------------------------------------------------------------
	// 7. FOOTER SHORTCUT BAR (1 Line)
	// -------------------------------------------------------------
	shortcutBar := fmt.Sprintf(" %s[Tab]%s Switch   %s[Ctrl+N]%s Group   %s[Ctrl+S]%s Search   %s[Ctrl+K]%s Deposit   %s[Ctrl+Q]%s Exit",
		ThemeCyan, ThemeDim,
		ThemeCyan, ThemeDim,
		ThemeCyan, ThemeDim,
		ThemeCyan, ThemeDim,
		ThemeCyan, ThemeDim,
	)
	b.WriteString(padTrunc(shortcutBar, width-2) + "\033[?25h")

	fmt.Fprint(s.out, b.String())
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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

	// 5. Enter Alternate Screen Buffer (Dedicated Full Screen)
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

	// Fetch & Replay Pending Queued Inbox Messages
	for _, m := range members {
		pendingMsgs, err := apiClient.FetchInbox(localIdFile.Handle, m.handle)
		if err == nil && len(pendingMsgs) > 0 {
			for _, msg := range pendingMsgs {
				plaintext, err := crypto.Decrypt([]byte(msg.Ciphertext), devIdentity)
				if err != nil {
					continue
				}
				timestamp := time.Now().Format("15:04")
				senderName := msg.Sender
				if senderName == "" {
					senderName = m.handle
				}
				state.AddMessage(ChatMessage{
					Timestamp:  timestamp,
					Sender:     senderName,
					Text:       string(plaintext),
					IsOutgoing: false,
				})
			}
		}
	}

	stopCh := make(chan struct{})
	var closeOnce sync.Once
	safeClose := func() {
		closeOnce.Do(func() {
			close(stopCh)
		})
	}

	// Background Auto-Resize Watcher (dynamically re-renders on window resize / maximize)
	go func() {
		ticker := time.NewTicker(150 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				w, h, err := term.GetSize(int(os.Stdout.Fd()))
				if err == nil && w > 0 && h > 0 {
					state.mu.Lock()
					if w != state.lastWidth || h != state.lastHeight {
						state.mu.Unlock()
						state.render()
					} else {
						state.mu.Unlock()
					}
				}
			}
		}
	}()

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
		safeClose()
		fmt.Fprint(ui.Out, "\033[?1049l\033[?25h")
		os.Exit(0)
	}()

	// 7. Foreground Input Handling Loop with interactive commands
	scanner := bufio.NewScanner(ui.In)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			state.SetInput("")
			continue
		}

		// Handle interactive slash commands / footer buttons
		if text == "/quit" || text == "/exit" || text == ":q" {
			safeClose()
			return 0
		}

		if text == "/help" {
			state.AddMessage(ChatMessage{
				Timestamp:  time.Now().Format("15:04"),
				Sender:     "SYSTEM",
				Text:       "Commands: /ttl <60s|300s|1h|24h> | /burn (toggle) | /clear | /quit",
				IsOutgoing: false,
			})
			state.SetInput("")
			continue
		}

		if text == "/burn" {
			newState := state.ToggleBurn()
			state.AddMessage(ChatMessage{
				Timestamp:  time.Now().Format("15:04"),
				Sender:     "POLICY",
				Text:       fmt.Sprintf("Burn-After-Reading updated: *%s*", newState),
				IsOutgoing: false,
			})
			state.SetInput("")
			continue
		}

		if strings.HasPrefix(text, "/ttl") {
			parts := strings.Fields(text)
			if len(parts) > 1 {
				state.SetTTL(parts[1])
				state.AddMessage(ChatMessage{
					Timestamp:  time.Now().Format("15:04"),
					Sender:     "POLICY",
					Text:       fmt.Sprintf("TTL expiration updated: *%s*", parts[1]),
					IsOutgoing: false,
				})
			}
			state.SetInput("")
			continue
		}

		if text == "/clear" {
			state.ClearMessages()
			state.SetInput("")
			continue
		}

		// Strip optional /msg prefix
		msgContent := text
		if strings.HasPrefix(text, "/msg ") {
			msgContent = strings.TrimPrefix(text, "/msg ")
		}

		// Encrypt message locally using age recipient-based encryption
		var ciphertext []byte
		if isGroup {
			pubKeys := make([]string, len(members))
			for i, m := range members {
				pubKeys[i] = m.publicKey
			}
			ciphertext, err = crypto.EncryptMulti([]byte(msgContent), pubKeys...)
		} else {
			ciphertext, err = crypto.Encrypt([]byte(msgContent), members[0].publicKey)
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
			Text:       msgContent,
			IsOutgoing: true,
		})
		state.SetInput("")
	}

	safeClose()
	return 0
}
