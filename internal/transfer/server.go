package transfer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ekk-clsrspk/OpenDrop/internal/notify"
)

// ClipboardMessage is POSTed peer-to-peer for auto-sync.
type ClipboardMessage struct {
	Text     string `json:"text"`
	OriginID string `json:"origin_id"`
	SentAt   int64  `json:"sent_at_unix"`
	Hash     string `json:"hash"`
}

func HashText(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// Server bundles daemon state for handlers.
type Server struct {
	DeviceID    string
	DeviceName  string
	DownloadDir string
	OnClipboard func(msg ClipboardMessage, r *http.Request)
}

func (s *Server) routes() *http.ServeMux {
	m := http.NewServeMux()
	m.HandleFunc("/api/v1/health", s.health)
	m.HandleFunc("/api/v1/files", s.handleFiles)
	m.HandleFunc("/api/v1/clipboard", s.handleClipboard)
	return m
}

func (s *Server) Handler() http.Handler { return s.routes() }

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"ok":          true,
		"device_id":   s.DeviceID,
		"device_name": s.DeviceName,
		"time":        time.Now().UTC().Format(time.RFC3339),
	})
}

// handleFiles accepts multipart `file` (+ optional `sender`). Streams to disk,
// verifies sha256 if client sent X-Sha256, then notifies.
func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if err := os.MkdirAll(s.DownloadDir, 0o755); err != nil {
		http.Error(w, "cannot create download dir", http.StatusInternalServerError)
		return
	}
	// 4 GiB cap per request; streaming so RAM stays flat.
	r.Body = http.MaxBytesReader(w, r.Body, 4<<30)
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, "parse form: "+err.Error(), http.StatusBadRequest)
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "field 'file' required", http.StatusBadRequest)
		return
	}
	defer f.Close()
	sender := r.FormValue("sender")
	if sender == "" {
		sender = r.RemoteAddr
	}

	dest := uniquePath(s.DownloadDir, filepath.Base(hdr.Filename))
	out, err := os.Create(dest)
	if err != nil {
		http.Error(w, "create dest: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), f)
	_ = out.Close()
	if err != nil {
		_ = os.Remove(dest)
		http.Error(w, "save: "+err.Error(), http.StatusInternalServerError)
		return
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if want := r.Header.Get("X-Sha256"); want != "" && !strings.EqualFold(want, sum) {
		_ = os.Remove(dest)
		http.Error(w, "sha256 mismatch", http.StatusBadRequest)
		return
	}
	msg := fmt.Sprintf("%s (%s) from %s", filepath.Base(dest), humanBytes(n), sender)
	notify.Notify("OpenDrop — file received", msg)
	writeJSON(w, map[string]any{"ok": true, "path": dest, "bytes": n, "sha256": sum})
}

func (s *Server) handleClipboard(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var msg ClipboardMessage
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal(body, &msg); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if msg.Hash == "" {
			msg.Hash = HashText(msg.Text)
		}
		if s.OnClipboard != nil {
			s.OnClipboard(msg, r)
		}
		writeJSON(w, map[string]any{"ok": true})
	case http.MethodGet:
		// lightweight poll fallback; daemon pushes via POST, this is just for debugging
		writeJSON(w, map[string]any{"ok": true, "hint": "POST text here"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func uniquePath(dir, base string) string {
	base = filepath.Base(base)
	if base == "" || base == "." {
		base = fmt.Sprintf("drop-%d", time.Now().Unix())
	}
	p := filepath.Join(dir, base)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return p
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for i := 1; ; i++ {
		cand := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		if _, err := os.Stat(cand); os.IsNotExist(err) {
			return cand
		}
	}
}

func humanBytes(n int64) string {
	const u = 1024
	if n < u {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(u), 0
	for n/div >= u && exp < 4 {
		div *= u
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	return fmt.Sprintf("%.1f %s", float64(n)/float64(div), units[exp])
}
