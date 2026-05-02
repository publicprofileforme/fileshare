package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed templates/*
var templateFS embed.FS

// ─── Helpers ──────────────────────────────────────────────────────────────────

func localIPs() []string {
	var ips []string
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && !ip.IsLoopback() && ip.To4() != nil {
				ips = append(ips, ip.String())
			}
		}
	}
	return ips
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func safeTimestamp() string {
	return time.Now().Format("2006-01-02_15-04-05")
}

func defaultSaveDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "uploads"
	}
	return filepath.Join(home, "Downloads", "Uploads")
}

// ─── Send state ───────────────────────────────────────────────────────────────

type sendMode int

const (
	modeIdle sendMode = iota
	modeFile
	modeText
)

type sendState struct {
	mu       sync.RWMutex
	mode     sendMode
	// file
	filePath string
	fileName string
	fileSize int64
	// text
	text string
}

func (s *sendState) setFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("path is a directory")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode     = modeFile
	s.filePath = path
	s.fileName = filepath.Base(path)
	s.fileSize = info.Size()
	s.text     = ""
	return nil
}

func (s *sendState) setText(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode     = modeText
	s.text     = text
	s.filePath = ""
	s.fileName = ""
	s.fileSize = 0
}

func (s *sendState) snapshot() (mode sendMode, filePath, fileName, text string, fileSize int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mode, s.filePath, s.fileName, s.text, s.fileSize
}

// ─── Received text log ────────────────────────────────────────────────────────

type textLog struct {
	mu      sync.RWMutex
	entries []textEntry
}

type textEntry struct {
	Time string `json:"time"`
	Text string `json:"text"`
}

func (l *textLog) add(text string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, textEntry{
		Time: time.Now().Format("15:04:05"),
		Text: text,
	})
}

func (l *textLog) all() []textEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	cp := make([]textEntry, len(l.entries))
	copy(cp, l.entries)
	return cp
}

// ─── Send server ──────────────────────────────────────────────────────────────

type sendServer struct {
	state      *sendState
	adminPort  int
	clientPort int
	tmpl       *template.Template
	headless   bool
	tmpDir     string
}

func (srv *sendServer) registerAdminRoutes(mux *http.ServeMux) {
	// Admin UI
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		mode, _, fileName, text, fileSize := srv.state.snapshot()
		data := map[string]interface{}{
			"ClientPort": srv.clientPort,
			"IPs":        localIPs(),
			"Headless":   srv.headless,
			"Mode":       mode,
			"ModeFile":   modeFile,
			"ModeText":   modeText,
			"FileName":   fileName,
			"FileSize":   humanSize(fileSize),
			"Text":       text,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		srv.tmpl.ExecuteTemplate(w, "send_admin.html", data)
	})

	// Upload file via browser
	mux.HandleFunc("/api/upload-file", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		if err := r.ParseMultipartForm(8 << 30); err != nil {
			http.Error(w, "parse error: "+err.Error(), 400)
			return
		}
		fhs := r.MultipartForm.File["file"]
		if len(fhs) == 0 {
			http.Error(w, "no file", 400)
			return
		}
		fh := fhs[0]
		src, err := fh.Open()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer src.Close()
		safeName := filepath.Base(fh.Filename)
		dst := filepath.Join(srv.tmpDir, safeName)
		out, err := os.Create(dst)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		io.Copy(out, src)
		out.Close()
		if err := srv.state.setFile(dst); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		_, _, fileName, _, fileSize := srv.state.snapshot()
		fmt.Printf("[send]  file: %s (%s)\n", fileName, humanSize(fileSize))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"name": fileName,
			"size": humanSize(fileSize),
		})
	})

	// Set file by path (headless)
	mux.HandleFunc("/api/select", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		var body struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
			http.Error(w, "bad request", 400)
			return
		}
		if err := srv.state.setFile(body.Path); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		_, _, fileName, _, fileSize := srv.state.snapshot()
		fmt.Printf("[send]  file: %s (%s)\n", fileName, humanSize(fileSize))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"name": fileName,
			"size": humanSize(fileSize),
		})
	})

	// Set text
	mux.HandleFunc("/api/set-text", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		var body struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		if strings.TrimSpace(body.Text) == "" {
			http.Error(w, "empty text", 400)
			return
		}
		srv.state.setText(body.Text)
		fmt.Printf("[send]  text set (%d chars)\n", len(body.Text))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})

	// Status
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		mode, _, fileName, text, fileSize := srv.state.snapshot()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"mode":     mode,
			"fileName": fileName,
			"fileSize": humanSize(fileSize),
			"text":     text,
		})
	})
}

func (srv *sendServer) registerClientRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		mode, _, fileName, text, fileSize := srv.state.snapshot()
		data := map[string]interface{}{
			"Mode":     mode,
			"ModeFile": modeFile,
			"ModeText": modeText,
			"FileName": fileName,
			"FileSize": humanSize(fileSize),
			"Text":     text,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		srv.tmpl.ExecuteTemplate(w, "send_client.html", data)
	})

	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		mode, filePath, fileName, _, fileSize := srv.state.snapshot()
		if mode != modeFile {
			http.Error(w, "no file available", 503)
			return
		}
		f, err := os.Open(filePath)
		if err != nil {
			http.Error(w, "file not found", 404)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Disposition", `attachment; filename="`+fileName+`"`)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))
		io.Copy(w, f)
	})

	// Poll for client (auto-refresh when idle)
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		mode, _, fileName, text, fileSize := srv.state.snapshot()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"mode":     mode,
			"fileName": fileName,
			"fileSize": humanSize(fileSize),
			"text":     text,
		})
	})
}

// ─── Receive server ───────────────────────────────────────────────────────────

type receiveServer struct {
	saveDir string
	port    int
	tmpl    *template.Template
	log     *textLog
}

func (srv *receiveServer) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data := map[string]interface{}{
			"Port":    srv.port,
			"SaveDir": srv.saveDir,
			"IPs":     localIPs(),
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		srv.tmpl.ExecuteTemplate(w, "receive.html", data)
	})

	// File upload
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		if err := r.ParseMultipartForm(4 << 30); err != nil {
			http.Error(w, "parse error: "+err.Error(), 400)
			return
		}
		files := r.MultipartForm.File["files"]
		if len(files) == 0 {
			http.Error(w, "no files", 400)
			return
		}
		var saved []string
		for _, fh := range files {
			src, err := fh.Open()
			if err != nil {
				continue
			}
			safeName := filepath.Base(fh.Filename)
			dst := filepath.Join(srv.saveDir, safeName)
			if _, err := os.Stat(dst); err == nil {
				ext := filepath.Ext(safeName)
				base := strings.TrimSuffix(safeName, ext)
				dst = filepath.Join(srv.saveDir, base+"_"+safeTimestamp()+ext)
			}
			out, err := os.Create(dst)
			if err != nil {
				src.Close()
				continue
			}
			io.Copy(out, src)
			out.Close()
			src.Close()
			fmt.Printf("[recv]  file saved: %s\n", dst)
			saved = append(saved, filepath.Base(dst))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"saved": saved})
	})

	// Text receive
	mux.HandleFunc("/send-text", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		var body struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		text := strings.TrimSpace(body.Text)
		if text == "" {
			http.Error(w, "empty text", 400)
			return
		}
		srv.log.add(text)
		fmt.Printf("[recv]  text (%d chars): %s\n", len(text), truncate(text, 80))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})

	// Text log poll
	mux.HandleFunc("/api/texts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(srv.log.all())
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	sendPort    := flag.Int("send-port",    8080, "client download port (all interfaces)")
	adminPort   := flag.Int("admin-port",   8081, "admin UI port (localhost only)")
	receivePort := flag.Int("receive-port", 8082, "receive/upload port (all interfaces)")
	receiveDir  := flag.String("dir",       defaultSaveDir(), "directory for received files")
	filePath    := flag.String("file",      "",   "file to share — enables headless send mode")
	noSend      := flag.Bool("no-send",     false, "disable send server")
	noReceive   := flag.Bool("no-receive",  false, "disable receive server")
	flag.Parse()

	tmpl, err := template.New("").Funcs(template.FuncMap{
		"humanSize": humanSize,
	}).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		fmt.Fprintf(os.Stderr, "template error: %v\n", err)
		os.Exit(1)
	}

	headless := *filePath != ""
	var servers []*http.Server
	var wg sync.WaitGroup

	// ── Send ─────────────────────────────────────────
	if !*noSend {
		tmpDir, err := os.MkdirTemp("", "fileshare-send-*")
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot create tmp dir: %v\n", err)
			os.Exit(1)
		}
		state := &sendState{}
		if headless {
			if err := state.setFile(*filePath); err != nil {
				fmt.Fprintf(os.Stderr, "cannot load file: %v\n", err)
				os.Exit(1)
			}
		}
		ss := &sendServer{
			state:      state,
			adminPort:  *adminPort,
			clientPort: *sendPort,
			tmpl:       tmpl,
			headless:   headless,
			tmpDir:     tmpDir,
		}
		adminMux := http.NewServeMux()
		ss.registerAdminRoutes(adminMux)
		adminSrv := &http.Server{Addr: fmt.Sprintf("127.0.0.1:%d", *adminPort), Handler: adminMux}

		clientMux := http.NewServeMux()
		ss.registerClientRoutes(clientMux)
		clientSrv := &http.Server{Addr: fmt.Sprintf("0.0.0.0:%d", *sendPort), Handler: clientMux}

		servers = append(servers, adminSrv, clientSrv)
		wg.Add(2)
		go func() { defer wg.Done(); adminSrv.ListenAndServe() }()
		go func() { defer wg.Done(); clientSrv.ListenAndServe() }()
	}

	// ── Receive ───────────────────────────────────────
	if !*noReceive {
		if err := os.MkdirAll(*receiveDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "cannot create dir: %v\n", err)
			os.Exit(1)
		}
		rs := &receiveServer{
			saveDir: *receiveDir,
			port:    *receivePort,
			tmpl:    tmpl,
			log:     &textLog{},
		}
		recvMux := http.NewServeMux()
		rs.registerRoutes(recvMux)
		recvSrv := &http.Server{Addr: fmt.Sprintf("0.0.0.0:%d", *receivePort), Handler: recvMux}
		servers = append(servers, recvSrv)
		wg.Add(1)
		go func() { defer wg.Done(); recvSrv.ListenAndServe() }()
	}

	// ── Banner ────────────────────────────────────────
	ips := localIPs()
	vpnIP := ""
	if len(ips) > 0 {
		vpnIP = ips[0]
	}
	fmt.Println()
	fmt.Println("  fileshare started!")
	fmt.Println("  ──────────────────────────────────────────────")
	if !*noSend {
		mode := "GUI"
		if headless {
			mode = "HEADLESS (--file)"
		}
		fmt.Printf("  [SEND]  Mode         : %s\n", mode)
		fmt.Printf("          Admin (you)  : http://localhost:%d\n", *adminPort)
		if vpnIP != "" {
			fmt.Printf("          Client       : http://%s:%d\n", vpnIP, *sendPort)
		}
	}
	if !*noReceive {
		fmt.Println("  ──────────────────────────────────────────────")
		if vpnIP != "" {
			fmt.Printf("  [RECV]  Upload URL   : http://%s:%d\n", vpnIP, *receivePort)
		}
		fmt.Printf("          Localhost    : http://localhost:%d\n", *receivePort)
		fmt.Printf("          Save dir     : %s\n", *receiveDir)
	}
	fmt.Println("  ──────────────────────────────────────────────")
	fmt.Println("  Stop: Ctrl+C")
	fmt.Println()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	fmt.Println("\nShutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, s := range servers {
		s.Shutdown(ctx)
	}
}
