package clipboard

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Read returns current clipboard text. V1: text only, capped by caller.
func Read() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("pbpaste").Output()
		if err != nil {
			return "", err
		}
		return string(out), nil
	case "windows":
		// Get-Clipboard -Raw preserves newlines; -TextFormatType Text limits to text.
		out, err := exec.Command("powershell", "-NoProfile", "-Command", "Get-Clipboard -Raw").Output()
		if err != nil {
			return "", err
		}
		return string(out), nil
	default:
		// Linux fallback: xclip / wl-paste
		if out, err := exec.Command("wl-paste", "-n").Output(); err == nil {
			return string(out), nil
		}
		out, err := exec.Command("xclip", "-selection", "clipboard", "-o").Output()
		if err != nil {
			return "", fmt.Errorf("no clipboard tool (install wl-clipboard or xclip): %w", err)
		}
		return string(out), nil
	}
}

// Write replaces clipboard text.
func Write(text string) error {
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("pbcopy")
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	case "windows":
		cmd := exec.Command("powershell", "-NoProfile", "-Command", "Set-Clipboard -Value ([Console]::In.ReadToEnd())")
		cmd.Stdin = strings.NewReader(text)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("Set-Clipboard: %v: %s", err, stderr.String())
		}
		return nil
	default:
		if c := exec.Command("wl-copy"); c.Err == nil {
			_ = c.Err
		}
		cmd := exec.Command("wl-copy")
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
		cmd2 := exec.Command("xclip", "-selection", "clipboard", "-i")
		cmd2.Stdin = strings.NewReader(text)
		return cmd2.Run()
	}
}
