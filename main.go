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

type sendState struct {
	mu       sync.RWMutex
	filePath string
	fileName string
	fileSize int64
	ready    bool
}

func (s *sendState) set(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("path is a directory")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.filePath = path
	s.fileName = filepath.Base(path)
	s.fileSize = info.Size()
	s.ready = true
	return nil
}

func (s *sendState) get() (path, name string, size int64, ready bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.filePath, s.fileName, s.fileSize, s.ready
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
		_, name, size, ready := srv.state.get()
		data := map[string]interface{}{
			"ClientPort": srv.clientPort,
			"IPs":        localIPs(),
			"Headless":   srv.headless,
			"Ready":      ready,
			"FileName":   name,
			"FileSize":   humanSize(size),
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		srv.tmpl.ExecuteTemplate(w, "send_admin.html", data)
	})

	// Upload file via browser file picker → store in tmpDir, register in state
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

		if err := srv.state.set(dst); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		_, name, size, _ := srv.state.get()
		fmt.Printf("[send]  file set: %s (%s)\n", name, humanSize(size))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"name": name,
			"size": humanSize(size),
		})
	})

	// Set file by path (headless / manual)
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
		if err := srv.state.set(body.Path); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		_, name, size, _ := srv.state.get()
		fmt.Printf("[send]  file set: %s (%s)\n", name, humanSize(size))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"name": name,
			"size": humanSize(size),
		})
	})

	// Status
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		_, name, size, ready := srv.state.get()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ready": ready,
			"name":  name,
			"size":  humanSize(size),
		})
	})
}

func (srv *sendServer) registerClientRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		_, name, size, ready := srv.state.get()
		data := map[string]interface{}{
			"Ready":    ready,
			"FileName": name,
			"FileSize": humanSize(size),
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		srv.tmpl.ExecuteTemplate(w, "send_client.html", data)
	})

	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		path, name, size, ready := srv.state.get()
		if !ready {
			http.Error(w, "no file available", 503)
			return
		}
		f, err := os.Open(path)
		if err != nil {
			http.Error(w, "file not found", 404)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		io.Copy(w, f)
	})
}

// ─── Receive server ───────────────────────────────────────────────────────────

type receiveServer struct {
	saveDir string
	port    int
	tmpl    *template.Template
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
			fmt.Printf("[recv]  saved: %s\n", dst)
			saved = append(saved, filepath.Base(dst))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"saved": saved})
	})
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
		// Temp dir for files uploaded via browser picker
		tmpDir, err := os.MkdirTemp("", "fileshare-send-*")
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot create tmp dir: %v\n", err)
			os.Exit(1)
		}

		state := &sendState{}
		if headless {
			if err := state.set(*filePath); err != nil {
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
		rs := &receiveServer{saveDir: *receiveDir, port: *receivePort, tmpl: tmpl}
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
