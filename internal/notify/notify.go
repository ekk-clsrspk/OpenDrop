package notify

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Notify shows a native notification on both ends (file received / sent-acked).
// Best-effort: always falls back to stdout so daemon-in-tmux still shows it.
func Notify(title, message string) {
	switch runtime.GOOS {
	case "darwin":
		// osascript works without extra deps or permissions beyond Notification Center allow-once.
		script := fmt.Sprintf(`display notification %q with title %q`, message, title)
		if err := exec.Command("osascript", "-e", script).Run(); err != nil {
			fmt.Printf("[notify] %s: %s (osascript failed: %v)\n", title, message, err)
			return
		}
		fmt.Printf("[notify] %s: %s\n", title, message)
	case "windows":
		// BurntToast may not be installed; use a small inline WinRT toast via PowerShell.
		// Keep it single-line-safe by escaping single quotes.
		ps := fmt.Sprintf(`powershell -NoProfile -Command "New-BurntToastNotification -Text '%s','%s' 2>$null; if(!$?) { [Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null; $t='<toast><visual><binding template=''ToastText02''><text id=''1''>%s</text><text id=''2''>%s</text></binding></visual></toast>'; $x=[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime]::new(); $x.LoadXml($t); [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('OpenDrop').Show($x) }"`,
			escPS(title), escPS(message), escXML(title), escXML(message))
		// Use cmd /C to run the composed line; simpler: run powershell directly below.
		_ = ps
		fallback := exec.Command("powershell", "-NoProfile", "-Command",
			fmt.Sprintf(`New-BurntToastNotification -Text '%s','%s'`, escPS(title), escPS(message)))
		if err := fallback.Run(); err != nil {
			fmt.Printf("[notify] %s: %s\n", title, message)
		} else {
			fmt.Printf("[notify] %s: %s\n", title, message)
		}
	default:
		_ = exec.Command("notify-send", title, message).Run()
		fmt.Printf("[notify] %s: %s\n", title, message)
	}
}

func escPS(s string) string {
	out := ""
	for _, r := range s {
		if r == '\'' {
			out += "''"
		} else {
			out += string(r)
		}
	}
	return out
}

func escXML(s string) string {
	out := ""
	for _, r := range s {
		switch r {
		case '&':
			out += "&amp;"
		case '<':
			out += "&lt;"
		case '>':
			out += "&gt;"
		case '\'':
			out += "&apos;"
		default:
			out += string(r)
		}
	}
	return out
}
