package main

import (
	"archive/zip"
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

// sanitizeRelPath strips leading slashes/dots and cleans the relative path.
// Used to preserve folder structure on upload without path traversal.
func sanitizeRelPath(rel string) string {
	// Normalize separators
	rel = filepath.FromSlash(rel)
	rel = filepath.Clean(rel)
	// Strip any leading ".." components
	parts := strings.Split(rel, string(filepath.Separator))
	var safe []string
	for _, p := range parts {
		if p == ".." || p == "." || p == "" {
			continue
		}
		safe = append(safe, p)
	}
	if len(safe) == 0 {
		return "file"
	}
	return filepath.Join(safe...)
}

// ─── Send state ───────────────────────────────────────────────────────────────

type sendMode int

const (
	modeIdle sendMode = iota
	modeFile
	modeText
)

// sendFileEntry holds one file in the send list.
type sendFileEntry struct {
	Path string
	Name string // display name
	Size int64
}

type sendState struct {
	mu      sync.RWMutex
	mode    sendMode
	files   []sendFileEntry // 1 or more files
	text    string
	zipName string // precomputed zip filename
}

func (s *sendState) setFiles(entries []sendFileEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode = modeFile
	s.files = entries
	s.text = ""
	if len(entries) == 1 {
		s.zipName = ""
	} else {
		s.zipName = "files_" + time.Now().Format("20060102_150405") + ".zip"
	}
}

func (s *sendState) setText(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode    = modeText
	s.text    = text
	s.files   = nil
	s.zipName = ""
}

func (s *sendState) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode    = modeIdle
	s.files   = nil
	s.text    = ""
	s.zipName = ""
}

type sendSnapshot struct {
	Mode    sendMode
	Files   []sendFileEntry
	Text    string
	ZipName string
	// Computed helpers
	TotalSize int64
	FileCount int
}

func (s *sendState) snapshot() sendSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make([]sendFileEntry, len(s.files))
	copy(cp, s.files)
	var total int64
	for _, f := range cp {
		total += f.Size
	}
	return sendSnapshot{
		Mode:      s.mode,
		Files:     cp,
		Text:      s.text,
		ZipName:   s.zipName,
		TotalSize: total,
		FileCount: len(cp),
	}
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
		snap := srv.state.snapshot()
		var firstName, firstSize string
		if len(snap.Files) > 0 {
			firstName = snap.Files[0].Name
			firstSize = humanSize(snap.Files[0].Size)
		}
		data := map[string]interface{}{
			"ClientPort": srv.clientPort,
			"IPs":        localIPs(),
			"Headless":   srv.headless,
			"Mode":       snap.Mode,
			"ModeFile":   modeFile,
			"ModeText":   modeText,
			"Files":      snap.Files,
			"FileCount":  snap.FileCount,
			"TotalSize":  humanSize(snap.TotalSize),
			"ZipName":    snap.ZipName,
			"FileName":   firstName,
			"FileSize":   firstSize,
			"Text":       snap.Text,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		srv.tmpl.ExecuteTemplate(w, "send_admin.html", data)
	})

	// Upload files via browser (GUI mode)
	mux.HandleFunc("/api/upload-files", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		if err := r.ParseMultipartForm(8 << 30); err != nil {
			http.Error(w, "parse error: "+err.Error(), 400)
			return
		}
		fhs := r.MultipartForm.File["files"]
		if len(fhs) == 0 {
			http.Error(w, "no files", 400)
			return
		}
		var entries []sendFileEntry
		for _, fh := range fhs {
			src, err := fh.Open()
			if err != nil {
				continue
			}
			safeName := filepath.Base(fh.Filename)
			dst := filepath.Join(srv.tmpDir, safeTimestamp()+"_"+safeName)
			out, err := os.Create(dst)
			if err != nil {
				src.Close()
				continue
			}
			io.Copy(out, src)
			out.Close()
			src.Close()
			info, _ := os.Stat(dst)
			entries = append(entries, sendFileEntry{
				Path: dst,
				Name: safeName,
				Size: info.Size(),
			})
		}
		if len(entries) == 0 {
			http.Error(w, "failed to save files", 500)
			return
		}
		srv.state.setFiles(entries)
		snap := srv.state.snapshot()
		fmt.Printf("[send]  %d file(s) ready (%s)\n", snap.FileCount, humanSize(snap.TotalSize))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"count":   snap.FileCount,
			"total":   humanSize(snap.TotalSize),
			"zipName": snap.ZipName,
			"files":   snap.Files,
		})
	})

	// Set file by path (headless / path input)
	mux.HandleFunc("/api/select", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		var body struct {
			Paths []string `json:"paths"`
			// legacy single-path support
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		if body.Path != "" && len(body.Paths) == 0 {
			body.Paths = []string{body.Path}
		}
		if len(body.Paths) == 0 {
			http.Error(w, "no paths", 400)
			return
		}
		var entries []sendFileEntry
		for _, p := range body.Paths {
			info, err := os.Stat(p)
			if err != nil || info.IsDir() {
				continue
			}
			entries = append(entries, sendFileEntry{
				Path: p,
				Name: filepath.Base(p),
				Size: info.Size(),
			})
		}
		if len(entries) == 0 {
			http.Error(w, "no valid files", 400)
			return
		}
		srv.state.setFiles(entries)
		snap := srv.state.snapshot()
		fmt.Printf("[send]  %d file(s) set (%s)\n", snap.FileCount, humanSize(snap.TotalSize))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"count":   snap.FileCount,
			"total":   humanSize(snap.TotalSize),
			"zipName": snap.ZipName,
			"files":   snap.Files,
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

	// Clear
	mux.HandleFunc("/api/clear", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		srv.state.clear()
		w.WriteHeader(204)
	})

	// Status
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		snap := srv.state.snapshot()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"mode":      snap.Mode,
			"fileCount": snap.FileCount,
			"totalSize": humanSize(snap.TotalSize),
			"zipName":   snap.ZipName,
			"files":     snap.Files,
			"text":      snap.Text,
		})
	})
}

func (srv *sendServer) registerClientRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		snap := srv.state.snapshot()
		data := map[string]interface{}{
			"Mode":      snap.Mode,
			"ModeFile":  modeFile,
			"ModeText":  modeText,
			"Files":     snap.Files,
			"FileCount": snap.FileCount,
			"TotalSize": humanSize(snap.TotalSize),
			"ZipName":   snap.ZipName,
			"Text":      snap.Text,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		srv.tmpl.ExecuteTemplate(w, "send_client.html", data)
	})

	// Single file download
	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		snap := srv.state.snapshot()
		if snap.Mode != modeFile {
			http.Error(w, "no file available", 503)
			return
		}
		// /download/0, /download/1, ...
		idxStr := strings.TrimPrefix(r.URL.Path, "/download/")
		idx, err := strconv.Atoi(idxStr)
		if err != nil || idx < 0 || idx >= len(snap.Files) {
			http.Error(w, "not found", 404)
			return
		}
		entry := snap.Files[idx]
		f, err := os.Open(entry.Path)
		if err != nil {
			http.Error(w, "file not found", 404)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Disposition", `attachment; filename="`+entry.Name+`"`)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.FormatInt(entry.Size, 10))
		io.Copy(w, f)
		fmt.Printf("[send]  downloaded: %s\n", entry.Name)
	})

	// ZIP download — streams all files into a zip without temp file
	mux.HandleFunc("/download-zip", func(w http.ResponseWriter, r *http.Request) {
		snap := srv.state.snapshot()
		if snap.Mode != modeFile || len(snap.Files) == 0 {
			http.Error(w, "no files available", 503)
			return
		}
		zipName := snap.ZipName
		if zipName == "" {
			zipName = snap.Files[0].Name + ".zip"
		}
		w.Header().Set("Content-Disposition", `attachment; filename="`+zipName+`"`)
		w.Header().Set("Content-Type", "application/zip")
		// Cannot set Content-Length because we stream; that's fine.
		zw := zip.NewWriter(w)
		defer zw.Close()
		for _, entry := range snap.Files {
			fw, err := zw.Create(entry.Name)
			if err != nil {
				continue
			}
			f, err := os.Open(entry.Path)
			if err != nil {
				continue
			}
			io.Copy(fw, f)
			f.Close()
		}
		fmt.Printf("[send]  zip downloaded (%d files)\n", len(snap.Files))
	})

	// Poll status
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		snap := srv.state.snapshot()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"mode":      snap.Mode,
			"fileCount": snap.FileCount,
			"totalSize": humanSize(snap.TotalSize),
			"zipName":   snap.ZipName,
			"files":     snap.Files,
			"text":      snap.Text,
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

	// File upload — supports relative paths for folder uploads
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		if err := r.ParseMultipartForm(8 << 30); err != nil {
			http.Error(w, "parse error: "+err.Error(), 400)
			return
		}
		// "files" field carries flat files; "relpaths" field carries matching relative paths
		fhs := r.MultipartForm.File["files"]
		relPaths := r.MultipartForm.Value["relpaths"]
		if len(fhs) == 0 {
			http.Error(w, "no files", 400)
			return
		}
		var saved []string
		for i, fh := range fhs {
			src, err := fh.Open()
			if err != nil {
				continue
			}
			var relPath string
			if i < len(relPaths) && relPaths[i] != "" {
				relPath = sanitizeRelPath(relPaths[i])
			} else {
				relPath = filepath.Base(fh.Filename)
			}
			dst := filepath.Join(srv.saveDir, relPath)
			// Ensure parent dirs exist (for folder uploads)
			if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
				src.Close()
				continue
			}
			// Deduplicate
			if _, err := os.Stat(dst); err == nil {
				ext := filepath.Ext(relPath)
				base := strings.TrimSuffix(dst, ext)
				dst = base + "_" + safeTimestamp() + ext
			}
			out, err := os.Create(dst)
			if err != nil {
				src.Close()
				continue
			}
			io.Copy(out, src)
			out.Close()
			src.Close()
			fmt.Printf("[recv]  saved: %s\n", dst)
			saved = append(saved, relPath)
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
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Text) == "" {
			http.Error(w, "bad request", 400)
			return
		}
		srv.log.add(body.Text)
		fmt.Printf("[recv]  text (%d chars): %s\n", len(body.Text), truncate(body.Text, 60))
		w.WriteHeader(204)
	})

	// Text log
	mux.HandleFunc("/api/texts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(srv.log.all())
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	sendPort    := flag.Int("send-port",    8080, "client download port (all interfaces)")
	adminPort   := flag.Int("admin-port",   8081, "admin UI port (localhost only)")
	receivePort := flag.Int("receive-port", 8082, "receive/upload port (all interfaces)")
	receiveDir  := flag.String("dir",       defaultSaveDir(), "directory for received files")
	filePaths   := flag.String("file",      "", "comma-separated file paths to share (headless)")
	noSend      := flag.Bool("no-send",     false, "disable send server")
	noReceive   := flag.Bool("no-receive",  false, "disable receive server")
	flag.Parse()

	tmpl, err := template.New("").Funcs(template.FuncMap{
		"humanSize": humanSize,
		"add":       func(a, b int) int { return a + b },
	}).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		fmt.Fprintf(os.Stderr, "template error: %v\n", err)
		os.Exit(1)
	}

	headless := *filePaths != ""
	var servers []*http.Server
	var wg sync.WaitGroup

	// ── Send ─────────────────────────────────────────
	if !*noSend {
		tmpDir, _ := os.MkdirTemp("", "fileshare-send-*")
		state := &sendState{}

		if headless {
			var entries []sendFileEntry
			for _, p := range strings.Split(*filePaths, ",") {
				p = strings.TrimSpace(p)
				info, err := os.Stat(p)
				if err != nil || info.IsDir() {
					fmt.Fprintf(os.Stderr, "skipping: %s\n", p)
					continue
				}
				entries = append(entries, sendFileEntry{
					Path: p,
					Name: filepath.Base(p),
					Size: info.Size(),
				})
			}
			if len(entries) == 0 {
				fmt.Fprintf(os.Stderr, "no valid files to share\n")
				os.Exit(1)
			}
			state.setFiles(entries)
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
		tlog := &textLog{}
		rs := &receiveServer{saveDir: *receiveDir, port: *receivePort, tmpl: tmpl, log: tlog}
		mux := http.NewServeMux()
		rs.registerRoutes(mux)
		recvSrv := &http.Server{Addr: fmt.Sprintf("0.0.0.0:%d", *receivePort), Handler: mux}
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
			mode = "HEADLESS"
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

	// ── Graceful shutdown ─────────────────────────────
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
