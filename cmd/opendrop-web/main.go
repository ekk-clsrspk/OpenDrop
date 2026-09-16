package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ekk-clsrspk/OpenDrop/internal/config"
	"github.com/ekk-clsrspk/OpenDrop/internal/discovery"
	"github.com/ekk-clsrspk/OpenDrop/internal/notify"
	"github.com/ekk-clsrspk/OpenDrop/internal/transfer"
)

//go:embed web
var webFS embed.FS

var cfg *config.Config

func main() {
	bind := flag.String("bind", "127.0.0.1", "interface to listen on (keep loopback: this UI has no auth)")
	port := flag.Int("port", 8655, "port for the web UI")
	flag.Parse()

	var err error
	cfg, err = config.Load()
	if err != nil {
		fmt.Println("config error:", err)
		os.Exit(1)
	}

	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		fmt.Println("embed error:", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/info", handleInfo)
	mux.HandleFunc("/api/peers", handlePeers)
	mux.HandleFunc("/api/send", handleSend)
	mux.HandleFunc("/api/inbox", handleInbox)
	mux.HandleFunc("/files/", handleFileDownload)

	addr := fmt.Sprintf("%s:%d", *bind, *port)
	fmt.Printf("OpenDrop Web: http://%s\nForwarding as %s (%s) — daemon on :%d must be running.\n",
		addr, cfg.DeviceName, cfg.DeviceID, cfg.Port)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Println("server error:", err)
		os.Exit(1)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func handleInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"device_name":  cfg.DeviceName,
		"device_id":    cfg.DeviceID,
		"daemon_port":  cfg.Port,
		"download_dir": cfg.DownloadDir,
	})
}

type peerJSON struct {
	Name string `json:"name"`
	ID   string `json:"id"`
	Addr string `json:"addr"`
	OK   bool   `json:"ok"`
}

func handlePeers(w http.ResponseWriter, r *http.Request) {
	out := []peerJSON{}
	seen := map[string]bool{}

	// Manual peers first (reliable path).
	for _, a := range discovery.AddrsFromManual(cfg.ManualPeers, cfg.Port) {
		name, id, ok := transfer.ProbeHealth(a, 2*time.Second)
		if name == "" {
			name = a
		}
		out = append(out, peerJSON{Name: name, ID: id, Addr: a, OK: ok})
		seen[a] = true
	}

	// Broadcast beacons second (may miss some; interval > browse window).
	if peers, err := discovery.Browse(2500 * time.Millisecond); err == nil {
		for _, p := range peers {
			if p.ID == cfg.DeviceID || seen[p.Addr] {
				continue
			}
			name, id, ok := transfer.ProbeHealth(p.Addr, 2*time.Second)
			if name == "" {
				name = p.Name
			}
			if id == "" {
				id = p.ID
			}
			out = append(out, peerJSON{Name: name, ID: id, Addr: p.Addr, OK: ok})
			seen[p.Addr] = true
		}
	}
	writeJSON(w, map[string]any{"peers": out})
}

// resolveTo maps the UI's `to` value (addr from the peer list, bare IP, or name)
// to a host:port. Keeps the UI simple: it always sends the exact addr it listed.
func resolveTo(to string) (string, error) {
	to = strings.TrimSpace(to)
	if to == "" {
		return "", fmt.Errorf("pick a peer first")
	}
	if _, _, err := net.SplitHostPort(to); err == nil {
		return to, nil
	}
	if ip := net.ParseIP(to); ip != nil {
		return fmt.Sprintf("%s:%d", to, cfg.Port), nil
	}
	for _, a := range discovery.AddrsFromManual(cfg.ManualPeers, cfg.Port) {
		if strings.Contains(strings.ToLower(a), strings.ToLower(to)) {
			return a, nil
		}
	}
	if peers, err := discovery.Browse(3 * time.Second); err == nil {
		lower := strings.ToLower(to)
		for _, p := range peers {
			if strings.EqualFold(p.Name, to) || strings.Contains(strings.ToLower(p.Name), lower) {
				return p.Addr, nil
			}
		}
	}
	return "", fmt.Errorf("no peer matching %q", to)
}

type sendResult struct {
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes,omitempty"`
	Remote string `json:"remote,omitempty"`
	Error  string `json:"error,omitempty"`
}

// handleSend accepts one multipart request with 1..N files (field "files" or
// "file") and forwards each to the peer via the existing PSK file API.
func handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	addr, err := resolveTo(r.URL.Query().Get("to"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<30)
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, "parse form: "+err.Error(), http.StatusBadRequest)
		return
	}
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		files = r.MultipartForm.File["file"]
	}
	if len(files) == 0 {
		http.Error(w, `no files (form field "files")`, http.StatusBadRequest)
		return
	}

	tmpDir, err := os.MkdirTemp("", "opendrop-web-*")
	if err != nil {
		http.Error(w, "temp dir: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(tmpDir)

	results := make([]sendResult, 0, len(files))
	var okCount int64
	var okBytes int64
	for _, fh := range files {
		res := sendResult{Name: filepath.Base(fh.Filename)}
		src, err := fh.Open()
		if err != nil {
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		tmp, err := os.CreateTemp(tmpDir, "up-*")
		if err != nil {
			_ = src.Close()
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		n, err := io.Copy(tmp, src)
		_ = src.Close()
		_ = tmp.Close()
		if err != nil {
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		// Stage under the real filename so the receiver keeps it.
		staged := uniqueTmpName(tmpDir, res.Name)
		if err := os.Rename(tmp.Name(), staged); err != nil {
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		remote, sent, err := transfer.SendFile(addr, cfg.PSKToken, cfg.DeviceName, staged)
		_ = n
		if err != nil {
			res.Error = err.Error()
			if strings.Contains(err.Error(), "401") {
				res.Error += " (PSK mismatch — pair both devices with the same token)"
			}
		} else {
			res.Bytes = sent
			res.Remote = remote
			okCount++
			okBytes += sent
		}
		results = append(results, res)
	}

	go notify.Notify("OpenDrop Web — sent",
		fmt.Sprintf("%d/%d files (%s) → %s", okCount, len(files), humanBytes(okBytes), addr))
	writeJSON(w, map[string]any{"results": results})
}

type inboxEntry struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	MTime int64  `json:"mtime_unix"`
}

func handleInbox(w http.ResponseWriter, r *http.Request) {
	entries, _ := os.ReadDir(cfg.DownloadDir)
	out := []inboxEntry{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, inboxEntry{Name: e.Name(), Size: info.Size(), MTime: info.ModTime().Unix()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MTime > out[j].MTime })
	if out == nil {
		out = []inboxEntry{}
	}
	writeJSON(w, map[string]any{"dir": cfg.DownloadDir, "files": out})
}

// uniqueTmpName stages an upload under its real filename, deduplicating
// within the batch so two same-named files don't clobber each other.
func uniqueTmpName(dir, base string) string {
	base = filepath.Base(base)
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

func handleFileDownload(w http.ResponseWriter, r *http.Request) {	name := filepath.Base(strings.TrimPrefix(r.URL.Path, "/files/"))
	if name == "" || name == "." || strings.Contains(name, "\x00") {
		http.Error(w, "bad filename", http.StatusBadRequest)
		return
	}
	p := filepath.Join(cfg.DownloadDir, name)
	http.ServeFile(w, r, p)
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
