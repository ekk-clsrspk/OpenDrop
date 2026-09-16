package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ekk-clsrspk/OpenDrop/internal/auth"
	"github.com/ekk-clsrspk/OpenDrop/internal/clipboard"
	"github.com/ekk-clsrspk/OpenDrop/internal/config"
	"github.com/ekk-clsrspk/OpenDrop/internal/discovery"
	"github.com/ekk-clsrspk/OpenDrop/internal/notify"
	"github.com/ekk-clsrspk/OpenDrop/internal/transfer"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "daemon":
		cmdDaemon(os.Args[2:])
	case "send":
		cmdSend(os.Args[2:])
	case "peers":
		cmdPeers(os.Args[2:])
	case "status":
		cmdStatus(os.Args[2:])
	case "clip":
		cmdClip(os.Args[2:])
	case "pair":
		cmdPair(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Printf("unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	_, _ = os.Stdout.WriteString(`OpenDrop — LAN file + clipboard sharing (Mac <-> Windows)

Usage:
  opendrop daemon [--port 53317]            run the background service (tmux / Task Scheduler runs this)
  opendrop send <file> --to <peer>          send a file (peer = name, id-prefix, or host:port)
  opendrop peers [--timeout 3s]             list devices on the LAN
  opendrop status                           local daemon health + config
  opendrop clip push [--text "..."]         push clipboard text to all peers
  opendrop clip get                         print last-sent note (daemon holds no history; reads local clipboard)
  opendrop clip pause | resume              stop/start auto-sync (kill-switch for passwords)
  opendrop pair --token <PSK>               set the shared secret (must match on both devices)

Config: ~/.opendrop/config.yaml   Downloads: ~/Downloads/OpenDrop (or %USERPROFILE%\Downloads\OpenDrop)`)
}

// ---------- daemon ----------

func cmdDaemon(args []string) {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	port := fs.Int("port", 0, "override port")
	_ = fs.Parse(args)

	cfg, err := config.Load()
	if err != nil {
		fmt.Println("config error:", err)
		os.Exit(1)
	}
	if *port != 0 {
		cfg.Port = *port
	}

	srv := &transfer.Server{
		DeviceID:    cfg.DeviceID,
		DeviceName:  cfg.DeviceName,
		DownloadDir: cfg.DownloadDir,
	}

	// Clipboard receive path with loop prevention.
	// lastSent tracks hashes WE pushed so an echo doesn't re-apply.
	lastSent := map[string]time.Time{}
	lastApplied := map[string]time.Time{}

	srv.OnClipboard = func(msg transfer.ClipboardMessage, r *http.Request) {
		if cfg.ClipboardPaused || !cfg.ClipboardEnabled {
			return
		}
		if msg.OriginID == cfg.DeviceID {
			return // our own broadcast came back
		}
		if t, ok := lastSent[msg.Hash]; ok && time.Since(t) < 3*time.Second {
			return
		}
		if t, ok := lastApplied[msg.Hash]; ok && time.Since(t) < 3*time.Second {
			return
		}
		if len(msg.Text) > cfg.ClipboardMaxBytes {
			fmt.Printf("[clip] rejected oversize %d bytes from %s\n", len(msg.Text), msg.OriginID)
			return
		}
		if err := clipboard.Write(msg.Text); err != nil {
			fmt.Printf("[clip] apply failed: %v\n", err)
			return
		}
		lastApplied[msg.Hash] = time.Now()
		fmt.Printf("[clip] applied %d chars from %s\n", len(msg.Text), msg.OriginID)
	}

	mux := srv.Handler()
	handler := auth.Middleware(cfg.PSKToken, mux)

	httpSrv := &http.Server{
		Addr:         fmt.Sprintf("0.0.0.0:%d", cfg.Port),
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 10 * time.Minute,
	}

	// mDNS advertise
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if cfg.MDNSEnabled {
		go func() {
			if err := discovery.Advertise(ctx, cfg.DeviceName, cfg.DeviceID, cfg.Port); err != nil {
				fmt.Printf("[discovery] mdns advertise failed: %v (manual peers still work)\n", err)
			}
		}()
	}

	// Clipboard watcher: poll local clipboard, push diffs to peers.
	go func() {
		if !cfg.ClipboardEnabled {
			fmt.Println("[clip] disabled in config")
			return
		}
		ticker := time.NewTicker(700 * time.Millisecond)
		defer ticker.Stop()
		prev := ""
		if cur, err := clipboard.Read(); err == nil {
			prev = cur
		}
		prevHash := transfer.HashText(prev)
		for range ticker.C {
			if cfg.ClipboardPaused {
				continue
			}
			cur, err := clipboard.Read()
			if err != nil {
				continue
			}
			if cur == prev {
				continue
			}
			prev = cur
			h := transfer.HashText(cur)
			if h == prevHash {
				continue
			}
			prevHash = h
			if cur == "" || len(cur) > cfg.ClipboardMaxBytes {
				continue
			}
			// Don't rebroadcast something we just applied.
			if t, ok := lastApplied[h]; ok && time.Since(t) < 3*time.Second {
				continue
			}
			lastSent[h] = time.Now()
			msg := transfer.ClipboardMessage{
				Text: cur, OriginID: cfg.DeviceID,
				SentAt: time.Now().Unix(), Hash: h,
			}
			for _, addr := range resolvePeerAddrs(cfg) {
				a := addr // capture
				go func() {
					_ = transfer.PushClipboard(a, cfg.PSKToken, msg)
				}()
			}
			fmt.Printf("[clip] pushed %d chars to peers\n", len(cur))
		}
	}()

	fmt.Printf("OpenDrop daemon: %s (%s) on :%d\nDownloads: %s\nPSK: %s… (keep secret)\n",
		cfg.DeviceName, cfg.DeviceID, cfg.Port, cfg.DownloadDir, cfg.PSKToken[:8])
	fmt.Println("Peers (manual):", strings.Join(cfg.ManualPeers, ", "))
	fmt.Println("Press Ctrl-C to stop. Run in tmux (Mac) or Task Scheduler (Windows).")

	ln, err := net.Listen("tcp", httpSrv.Addr)
	if err != nil {
		fmt.Println("listen error:", err)
		os.Exit(1)
	}
	if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
		fmt.Println("server error:", err)
		os.Exit(1)
	}
}

// resolvePeerAddrs merges mDNS browse (short timeout) + manual peers.
// Called by sender paths; daemon watcher calls it per clipboard change (cached 10s).
func resolvePeerAddrs(cfg *config.Config) []string {
	addrs := discovery.AddrsFromManual(cfg.ManualPeers, cfg.Port)
	if cfg.MDNSEnabled {
		peers, err := discovery.Browse(1500 * time.Millisecond)
		if err == nil {
			for _, p := range peers {
				if p.ID == cfg.DeviceID {
					continue
				}
				dup := false
				for _, a := range addrs {
					if a == p.Addr {
						dup = true
						break
					}
				}
				if !dup {
					addrs = append(addrs, p.Addr)
				}
			}
		}
	}
	return addrs
}

// resolveOne maps --to (name | id-prefix | host:port | ip) to a single addr.
func resolveOne(cfg *config.Config, to string) (string, error) {
	to = strings.TrimSpace(to)
	if to == "" {
		return "", fmt.Errorf("--to required (name, id, or host:port). Run `opendrop peers` first")
	}
	// direct host:port or bare IP/hostname
	if _, _, err := net.SplitHostPort(to); err == nil {
		return to, nil
	}
	if ip := net.ParseIP(to); ip != nil {
		return fmt.Sprintf("%s:%d", to, cfg.Port), nil
	}
	if !strings.Contains(to, " ") && !strings.Contains(to, ".") && strings.Contains(to, ":") {
		return to, nil // likely host:port without split? keep
	}
	// try manual peers by substring
	for _, a := range discovery.AddrsFromManual(cfg.ManualPeers, cfg.Port) {
		if strings.Contains(strings.ToLower(a), strings.ToLower(to)) {
			return a, nil
		}
	}
	// mDNS lookup by name/id
	peers, err := discovery.Browse(3 * time.Second)
	if err != nil {
		return "", fmt.Errorf("discovery failed: %w", err)
	}
	lower := strings.ToLower(to)
	for _, p := range peers {
		if p.ID == cfg.DeviceID {
			continue
		}
		if strings.EqualFold(p.Name, to) || strings.Contains(strings.ToLower(p.Name), lower) ||
			(p.ID != "" && strings.HasPrefix(strings.ToLower(p.ID), lower)) ||
			strings.EqualFold(p.Addr, to) || strings.EqualFold(p.Host, to) {
			return p.Addr, nil
		}
	}
	// last resort: treat as hostname with default port
	if !strings.Contains(to, ":") {
		return fmt.Sprintf("%s:%d", to, cfg.Port), nil
	}
	return "", fmt.Errorf("no peer matching %q (see `opendrop peers`)", to)
}

// ---------- send ----------

func cmdSend(args []string) {
	// Accept flags in any order: `send <file> --to <peer>` and `send --to <peer> <file>`
	toVal := ""
	filtered := []string{}
	for i := 0; i < len(args); i++ {
		if args[i] == "--to" && i+1 < len(args) {
			toVal = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(args[i], "--to=") {
			toVal = strings.TrimPrefix(args[i], "--to=")
			continue
		}
		filtered = append(filtered, args[i])
	}
	fs := flag.NewFlagSet("send", flag.ExitOnError)
	to := fs.String("to", toVal, "peer name, id-prefix, or host:port")
	_ = fs.Parse(filtered)
	if *to == "" {
		// flag may have been passed inside filtered
	}
	rest := fs.Args()
	if len(rest) < 1 {
		fmt.Println("usage: opendrop send <file> --to <peer>")
		os.Exit(2)
	}
	filePath := rest[0]
	cfg, err := config.Load()
	if err != nil {
		fmt.Println("config error:", err)
		os.Exit(1)
	}
	addr, err := resolveOne(cfg, *to)
	if err != nil {
		fmt.Println("resolve error:", err)
		os.Exit(1)
	}
	fmt.Printf("Sending %s → %s …\n", filePath, addr)
	remote, n, err := transfer.SendFile(addr, cfg.PSKToken, cfg.DeviceName, filePath)
	if err != nil {
		fmt.Println("send failed:", err)
		// hint firewall/auth
		if strings.Contains(err.Error(), "401") {
			fmt.Println("hint: PSK mismatch — run `opendrop pair --token <same-on-both>` on both devices")
		}
		os.Exit(1)
	}
	notify.Notify("OpenDrop — file sent", fmt.Sprintf("%s (%d B) → %s", filePath, n, addr))
	fmt.Printf("OK: %d bytes, stored at %s (on receiver)\n", n, remote)
}

// ---------- peers ----------

func cmdPeers(args []string) {
	fs := flag.NewFlagSet("peers", flag.ExitOnError)
	timeout := fs.Duration("timeout", 3*time.Second, "mdns browse time")
	_ = fs.Parse(args)
	cfg, err := config.Load()
	if err != nil {
		fmt.Println("config error:", err)
		os.Exit(1)
	}
	fmt.Printf("Self: %s (%s) :%d\n", cfg.DeviceName, cfg.DeviceID, cfg.Port)
	fmt.Println("--- mDNS ---")
	peers, err := discovery.Browse(*timeout)
	if err != nil {
		fmt.Println("mdns error (manual peers still usable):", err)
	} else if len(peers) == 0 {
		fmt.Println("(none found — same WiFi? firewall open on 5353/UDP + 53317/TCP?)")
	} else {
		for _, p := range peers {
			self := ""
			if p.ID == cfg.DeviceID {
				self = " [self]"
			}
			name, id, ok := transfer.ProbeHealth(p.Addr, 2*time.Second)
			health := "?"
			if ok {
				health = fmt.Sprintf("ok name=%s id=%s", name, id)
			}
			fmt.Printf("  %-28s %-21s %s%s\n", p.Name, p.Addr, health, self)
		}
	}
	fmt.Println("--- manual ---")
	for _, a := range discovery.AddrsFromManual(cfg.ManualPeers, cfg.Port) {
		name, _, ok := transfer.ProbeHealth(a, 2*time.Second)
		status := "unreachable"
		if ok {
			status = "ok name=" + name
		}
		fmt.Printf("  %-21s %s\n", a, status)
	}
}

// ---------- status ----------

func cmdStatus(args []string) {
	_ = args
	cfg, err := config.Load()
	if err != nil {
		fmt.Println("config error:", err)
		os.Exit(1)
	}
	fmt.Printf("Device: %s (%s)\nPort: %d\nConfig: %s\nDownloads: %s\nClipboard: enabled=%v paused=%v\nManual peers: %v\n",
		cfg.DeviceName, cfg.DeviceID, cfg.Port, config.Path(), cfg.DownloadDir,
		cfg.ClipboardEnabled, cfg.ClipboardPaused, cfg.ManualPeers)
	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	if name, id, ok := transfer.ProbeHealth(addr, 2*time.Second); ok {
		fmt.Printf("Daemon: RUNNING (name=%s id=%s)\n", name, id)
	} else {
		fmt.Println("Daemon: NOT RUNNING (start with `opendrop daemon` / tmux / Task Scheduler)")
	}
}

// ---------- clip ----------

func cmdClip(args []string) {
	if len(args) == 0 {
		fmt.Println("usage: opendrop clip push [--text ...] | get | pause | resume")
		os.Exit(2)
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Println("config error:", err)
		os.Exit(1)
	}
	switch args[0] {
	case "push":
		fs := flag.NewFlagSet("clip push", flag.ExitOnError)
		text := fs.String("text", "", "text to send (default: read local clipboard)")
		_ = fs.Parse(args[1:])
		t := *text
		if t == "" {
			cur, err := clipboard.Read()
			if err != nil {
				fmt.Println("read clipboard:", err)
				os.Exit(1)
			}
			t = cur
		}
		if t == "" {
			fmt.Println("clipboard empty, nothing to send")
			return
		}
		msg := transfer.ClipboardMessage{
			Text: t, OriginID: cfg.DeviceID,
			SentAt: time.Now().Unix(), Hash: transfer.HashText(t),
		}
		addrs := resolvePeerAddrs(cfg)
		if len(addrs) == 0 {
			fmt.Println("no peers (run `opendrop peers`, or add `peers:` to config)")
			os.Exit(1)
		}
		ok := 0
		for _, a := range addrs {
			if err := transfer.PushClipboard(a, cfg.PSKToken, msg); err != nil {
				fmt.Printf("  ✗ %s: %v\n", a, err)
			} else {
				fmt.Printf("  ✓ %s\n", a)
				ok++
			}
		}
		fmt.Printf("pushed %d chars to %d/%d peers\n", len(t), ok, len(addrs))
	case "get":
		cur, err := clipboard.Read()
		if err != nil {
			fmt.Println("read clipboard:", err)
			os.Exit(1)
		}
		fmt.Print(cur)
	case "pause":
		cfg.ClipboardPaused = true
		_ = cfg.Save()
		fmt.Println("clipboard auto-sync PAUSED (password-safe). `opendrop clip resume` to re-enable.")
	case "resume":
		cfg.ClipboardPaused = false
		_ = cfg.Save()
		fmt.Println("clipboard auto-sync RESUMED.")
	default:
		fmt.Println("usage: opendrop clip push [--text ...] | get | pause | resume")
		os.Exit(2)
	}
}

// ---------- pair ----------

func cmdPair(args []string) {
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	token := fs.String("token", "", "shared secret (must match on both devices)")
	_ = fs.Parse(args)
	if *token == "" {
		fmt.Println("usage: opendrop pair --token <PSK>")
		os.Exit(2)
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Println("config error:", err)
		os.Exit(1)
	}
	cfg.PSKToken = strings.TrimSpace(*token)
	if err := cfg.Save(); err != nil {
		fmt.Println("save error:", err)
		os.Exit(1)
	}
	fmt.Println("paired. Restart daemon on both devices.")
}
