package tui

import (
	"fmt"
	"strings"
)

// ANSI Color Constants
const (
	ColorReset   = "\033[0m"
	ColorBold    = "\033[1m"
	ColorDim     = "\033[2m"
	ColorCyan    = "\033[36m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorMagenta = "\033[35m"
	ColorRed     = "\033[31m"
	ColorWhite   = "\033[37m"
	ColorBgDark  = "\033[48;5;234m"
)

// ContactItem represents a live registered contact or channel in the Left Directory
type ContactItem struct {
	Handle      string
	Fingerprint string
	IsGroup     bool
	IsActive    bool
	UnreadCount int
}

// ChatMessageItem represents a message bubble in the Center Stream
type ChatMessageItem struct {
	Sender    string
	Text      string
	Timestamp string
	IsYou     bool
	IsFile    bool
	Filename  string
	FileSize  int
	SavedPath string
}

// SecurityState represents state for the Right Security Inspector Drawer
type SecurityState struct {
	RecipientHandle string
	Fingerprint     string
	PublicKey       string
	TTLExpiration   int // seconds
	BurnAfterRead   bool
	Verified        bool
}

// RenderFullTUI renders a complete multi-pane TUI terminal dashboard matching the LazyGit aesthetic
func RenderFullTUI(
	hostHandle string,
	hostFingerprint string,
	contacts []ContactItem,
	activeChannel string,
	messages []ChatMessageItem,
	sec SecurityState,
	inputText string,
	termWidth int,
) string {
	if termWidth < 80 {
		termWidth = 90
	}

	var sb strings.Builder

	// Title Header
	headerTitle := fmt.Sprintf(" PANDORA'S VEIL | [ v1.2.5 ] Secure Channel - %s ", activeChannel)
	headerPadding := (termWidth - len(headerTitle) - 6) / 2
	if headerPadding < 2 {
		headerPadding = 2
	}
	sb.WriteString(fmt.Sprintf("%s%s🔴 🟡 🟢%s%s%s%s%s\n",
		ColorBgDark, ColorBold,
		strings.Repeat(" ", headerPadding),
		ColorCyan, headerTitle, ColorReset, ColorBgDark))

	// Pane Width calculations
	leftWidth := 26
	rightWidth := 30
	centerWidth := termWidth - leftWidth - rightWidth - 6
	if centerWidth < 30 {
		centerWidth = 30
	}

	// 1. Draw Top Frame Borders
	topBorderLeft := "┌─ DEVICE IDENTITY " + strings.Repeat("─", leftWidth-19) + "┐"
	topBorderCenter := "┌─ CENTER MAIN PANE (" + fmt.Sprintf("%s", activeChannel) + ") " + strings.Repeat("─", max(2, centerWidth-len(activeChannel)-24)) + "┐"
	topBorderRight := "┌─ SECRET DEPOSIT & POLICY " + strings.Repeat("─", max(2, rightWidth-26)) + "┐"
	sb.WriteString(fmt.Sprintf("%s%s %s %s%s\n", ColorCyan, topBorderLeft, topBorderCenter, topBorderRight, ColorReset))

	// Prepare Left Pane Lines
	var leftLines []string
	leftLines = append(leftLines, fmt.Sprintf(" HOST: %s%s%s", ColorBold+ColorCyan, truncateStr(hostHandle, leftWidth-8), ColorReset))
	leftLines = append(leftLines, fmt.Sprintf(" FP:   %s%s%s", ColorYellow, truncateStr(hostFingerprint, leftWidth-8), ColorReset))
	leftLines = append(leftLines, fmt.Sprintf(" STATUS: %sONLINE (age)%s", ColorGreen, ColorReset))
	leftLines = append(leftLines, "├─ CHANNELS " + strings.Repeat("─", leftWidth-13) + "┤")
	leftLines = append(leftLines, fmt.Sprintf(" %sRegistered Contacts:%s", ColorDim, ColorReset))

	if len(contacts) == 0 {
		leftLines = append(leftLines, fmt.Sprintf("  %s(No other keys)%s", ColorDim, ColorReset))
	} else {
		for _, c := range contacts {
			prefix := "○ "
			color := ColorWhite
			if c.IsActive {
				prefix = "● "
				color = ColorBold + ColorCyan
			}
			unreadStr := ""
			if c.UnreadCount > 0 {
				unreadStr = fmt.Sprintf(" %s(%d)%s", ColorYellow, c.UnreadCount, ColorReset)
			}
			line := fmt.Sprintf(" %s%s%s%s", color, prefix, truncateStr(c.Handle, leftWidth-7), unreadStr)
			leftLines = append(leftLines, line)
		}
	}

	// Prepare Center Stream Lines
	var centerLines []string
	if len(messages) == 0 {
		centerLines = append(centerLines, fmt.Sprintf("  %s🔒 Live encrypted stream active. Type message or /f to attach file.%s", ColorDim, ColorReset))
	} else {
		for _, msg := range messages {
			if msg.IsYou {
				if msg.IsFile {
					centerLines = append(centerLines, fmt.Sprintf("%s[%s] [YOU]%s", strings.Repeat(" ", max(0, centerWidth-len(msg.Filename)-24)), msg.Timestamp, ColorReset))
					centerLines = append(centerLines, fmt.Sprintf("%s%s📁 [FILE SENT] %s (%d KB)%s", strings.Repeat(" ", max(0, centerWidth-len(msg.Filename)-24)), ColorGreen, msg.Filename, msg.FileSize/1024, ColorReset))
				} else {
					centerLines = append(centerLines, fmt.Sprintf("%s[%s] [YOU]%s", strings.Repeat(" ", max(0, centerWidth-len(msg.Text)-16)), msg.Timestamp, ColorReset))
					centerLines = append(centerLines, fmt.Sprintf("%s%s%s%s", strings.Repeat(" ", max(0, centerWidth-len(msg.Text)-16)), ColorGreen, msg.Text, ColorReset))
				}
			} else {
				if msg.IsFile {
					centerLines = append(centerLines, fmt.Sprintf("[%s] %s%s%s", msg.Timestamp, ColorMagenta, msg.Sender, ColorReset))
					centerLines = append(centerLines, fmt.Sprintf(" 📁 [FILE] %s%s%s (%d KB)", ColorYellow, msg.Filename, ColorReset, msg.FileSize/1024))
					if msg.SavedPath != "" {
						centerLines = append(centerLines, fmt.Sprintf("    -> Saved to %s%s%s", ColorCyan, msg.SavedPath, ColorReset))
					}
				} else {
					centerLines = append(centerLines, fmt.Sprintf("[%s] %s%s%s ❯ %s", msg.Timestamp, ColorMagenta, msg.Sender, ColorReset, msg.Text))
				}
			}
		}
	}

	// Prepare Right Security Inspector Lines
	var rightLines []string
	rightLines = append(rightLines, fmt.Sprintf(" RECIP: %s%s%s", ColorCyan, truncateStr(sec.RecipientHandle, rightWidth-9), ColorReset))
	rightLines = append(rightLines, fmt.Sprintf(" FP:    %s%s%s", ColorYellow, truncateStr(sec.Fingerprint, rightWidth-9), ColorReset))
	if sec.Verified {
		rightLines = append(rightLines, fmt.Sprintf(" VERIFIED: %s[✓ MATCH]%s", ColorGreen, ColorReset))
	} else {
		rightLines = append(rightLines, fmt.Sprintf(" VERIFIED: %s[?] UNVERIFIED%s", ColorRed, ColorReset))
	}
	rightLines = append(rightLines, "├─ TTL EXPIRATION " + strings.Repeat("─", max(2, rightWidth-19)) + "┤")
	ttl60 := "[ 60s ]"
	ttl300 := "[ 300s ]"
	ttl3600 := "[ 1h ]"
	ttl86400 := "[ 24h ]"
	switch sec.TTLExpiration {
	case 60:
		ttl60 = ColorBold + ColorGreen + "[*60s*]" + ColorReset
	case 300:
		ttl300 = ColorBold + ColorGreen + "[*300s*]" + ColorReset
	case 3600:
		ttl3600 = ColorBold + ColorGreen + "[*1h*]" + ColorReset
	default:
		ttl86400 = ColorBold + ColorGreen + "[*24h*]" + ColorReset
	}
	rightLines = append(rightLines, fmt.Sprintf(" %s %s %s %s", ttl60, ttl300, ttl3600, ttl86400))
	rightLines = append(rightLines, "├─ BURN-AFTER-READING " + strings.Repeat("─", max(2, rightWidth-23)) + "┤")
	if sec.BurnAfterRead {
		rightLines = append(rightLines, fmt.Sprintf(" Redis GETDEL: %s[*ON* | OFF]%s", ColorBold+ColorRed, ColorReset))
	} else {
		rightLines = append(rightLines, fmt.Sprintf(" Redis GETDEL: %s[ ON | *OFF* ]%s", ColorGreen, ColorReset))
	}
	rightLines = append(rightLines, "├─ CRYPTO METADATA " + strings.Repeat("─", max(2, rightWidth-20)) + "┤")
	rightLines = append(rightLines, fmt.Sprintf(" Cipher: %sage (X25519)%s", ColorDim, ColorReset))

	// Max body height
	maxHeight := 14
	for i := 0; i < maxHeight; i++ {
		lL := ""
		if i < len(leftLines) {
			lL = leftLines[i]
		}
		cL := ""
		if i < len(centerLines) {
			cL = centerLines[i]
		}
		rL := ""
		if i < len(rightLines) {
			rL = rightLines[i]
		}

		sb.WriteString(fmt.Sprintf("%s│%s %s %s│%s %s %s│%s %s %s│%s\n",
			ColorCyan, ColorReset, padLine(lL, leftWidth-2),
			ColorCyan, ColorReset, padLine(cL, centerWidth-2),
			ColorCyan, ColorReset, padLine(rL, rightWidth-2),
			ColorCyan, ColorReset))
	}

	// Bottom Box Frame
	botBorderLeft := "└" + strings.Repeat("─", leftWidth) + "┘"
	botBorderCenter := "└" + strings.Repeat("─", centerWidth) + "┘"
	botBorderRight := "└" + strings.Repeat("─", rightWidth) + "┘"
	sb.WriteString(fmt.Sprintf("%s%s %s %s%s\n", ColorCyan, botBorderLeft, botBorderCenter, botBorderRight, ColorReset))

	// Bottom Status & Command Bar
	sb.WriteString(fmt.Sprintf("%s┌─ COMMAND & STATUS BAR %s┐%s\n", ColorCyan, strings.Repeat("─", termWidth-26), ColorReset))
	sb.WriteString(fmt.Sprintf("%s│%s pv > %s%-s %s│%s\n",
		ColorCyan, ColorBold+ColorGreen, ColorReset, padLine(inputText, termWidth-10), ColorCyan, ColorReset))
	sb.WriteString(fmt.Sprintf("%s│%s %s[Tab]%s Switch Pane  %s[/f]%s Attach File  %s[Ctrl+N]%s New Group  %s[Ctrl+C]%s Exit %s%s│%s\n",
		ColorCyan, ColorReset, ColorBold+ColorYellow, ColorReset, ColorBold+ColorYellow, ColorReset, ColorBold+ColorYellow, ColorReset, ColorBold+ColorYellow, ColorReset, strings.Repeat(" ", max(0, termWidth-68)), ColorCyan, ColorReset))
	sb.WriteString(fmt.Sprintf("%s└%s┘%s\n", ColorCyan, strings.Repeat("─", termWidth-2), ColorReset))

	return sb.String()
}

func padLine(s string, targetLen int) string {
	plainLen := len(stripANSI(s))
	if plainLen >= targetLen {
		return s
	}
	return s + strings.Repeat(" ", targetLen-plainLen)
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func stripANSI(s string) string {
	var sb strings.Builder
	inEsc := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') {
				inEsc = false
			}
			continue
		}
		sb.WriteByte(s[i])
	}
	return sb.String()
}
