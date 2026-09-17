package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Global variables for storage paths and session configurations
var uploadDir string

const sessionCookieName = "filedrop_session"
const maxDevices = 2
const sessionTimeout = 30 * time.Second

// Keep track of connected browser sessions
var (
	sessionMu      sync.Mutex
	sessions       = make(map[string]*Session)
	desktopSession *Session
)

//go:embed template/index.html static
var embeddedFiles embed.FS

type Session struct {
	ID         string
	DeviceName string
	LastSeen   time.Time
}

type FileEntry struct {
	Name string `json:"name"`
}

type HomeData struct {
	Files    []FileEntry
	HasPeer  bool
	ThisName string
	PeerName string
}

type DownloadRecord struct {
	FileName     string    `json:"file_name"`
	DownloadedAt time.Time `json:"downloaded_at"`
}

type FilesResponse struct {
	Files   []FileEntry `json:"files"`
	HasPeer bool        `json:"has_peer"`
}

type DeviceResponse struct {
	ThisDevice  string `json:"this_device"`
	OtherDevice string `json:"other_device,omitempty"`
}

type contextKey string

const desktopContextKey contextKey = "filedrop-desktop"

type App struct {
	v3App *application.App
}

func main() {
	// 1. Resolve storage directory dynamically depending on execution environment (Android vs Desktop)
	if runtime.GOOS == "android" {
		filesDir := os.Getenv("FILES_DIR")
		if filesDir == "" {
			filesDir = "/data/data/com.wails.app/files"
		}
		uploadDir = filepath.Join(filesDir, "shared_files")
	} else {
		uploadDir = "./shared_files"
	}

	// Create storage folder with appropriate permissions
	if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
		fmt.Printf("Failed to create storage directory: %v\n", err)
	}

	// Clean sessions
	go cleanupSessions()

	staticFiles, err := fs.Sub(embeddedFiles, "static")
	if err != nil {
		fmt.Printf("Failed to load static files: %v\n", err)
		return
	}

	mux := http.NewServeMux()

	mux.Handle("/", http.HandlerFunc(handleHome))
	mux.Handle("/upload", http.HandlerFunc(handleUpload))
	mux.Handle("/transfer", http.HandlerFunc(handleTransfer))
	mux.Handle("/download/", http.HandlerFunc(handleDownload))
	mux.Handle("/devices", http.HandlerFunc(handleDevices))
	mux.Handle("/device", http.HandlerFunc(handleDevice))
	mux.Handle("/files", http.HandlerFunc(handleFiles))

	staticHandler := http.StripPrefix(
		"/static",
		http.FileServer(http.FS(staticFiles)),
	)

	mux.Handle("/static/", staticHandler)

	localIP := getLocalIP()
	port := ":8080"
	targetURL := fmt.Sprintf("http://%s%s", localIP, port)

	fmt.Printf("File Server started successfully!\n")
	fmt.Printf("Local Host: http://localhost%s\n", port)
	fmt.Printf("Target URL: %s\n", targetURL)
	fmt.Println("Scan this QR code with your phone to connect instantly:")
	fmt.Println("")

	// Generate and print the terminal QR code
	qr, err := qrcode.New(targetURL, qrcode.Medium)
	if err == nil {
		fmt.Print(qr.ToSmallString(false))
	} else {
		fmt.Printf("Could not generate QR code: %v\n", err)
	}

	go func() {
		if err := http.ListenAndServe("0.0.0.0:8080", mux); err != nil {
			fmt.Printf("Error starting LAN server: %v\n", err)
		}
	}()

	wailsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), desktopContextKey, true)
		request := r.WithContext(ctx)
		if request.URL.Path == "/index.html" {
			request.URL.Path = "/"
		}
		mux.ServeHTTP(w, request)
	})

	// Instantiate Wails v3 Application
	appStruct := &App{}
	wailsApp := application.New(application.Options{
		Name:        "File Drop",
		Description: "LAN File Sharing App",
		Assets: application.AssetOptions{
			Handler: wailsHandler,
		},
		Services: []application.Service{
			application.NewService(appStruct),
		},
	})

	// Store app reference inside App struct if needed for dialogs
	appStruct.v3App = wailsApp

	// Create Window in v3
	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "File Drop",
		Width:  1024,
		Height: 768,
		URL:    "http://127.0.0.1:8080/",
	})

	err = wailsApp.Run()
	if err != nil {
		fmt.Printf("Error starting File Drop: %v\n", err)
	}
}

// DownloadFile uses the Wails v3 dialog system instead of v2 runtime context
func (a *App) DownloadFile(filename string) error {
	session := desktopSession

	if session == nil {
		return fmt.Errorf("desktop session not available")
	}

	source := filepath.Join(getSessionDir(session), filepath.Base(filename))

	if _, err := os.Stat(source); err != nil {
		return err
	}

	// Wails v3 Save File Dialog API
	if a.v3App == nil {
		return fmt.Errorf("application not available")
	}

	dialog := a.v3App.Dialog.SaveFile()
	dialog.SetFilename(filepath.Base(filename))

	destination, err := dialog.PromptForSingleSelection()
	if err != nil {
		return err
	}

	if destination == "" {
		return nil
	}

	return copyFile(source, destination)
}

// Generate secure session IDs
func generateSessionID() (string, error) {
	bytes := make([]byte, 32)

	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	return hex.EncodeToString(bytes), nil
}

// User's session
func getSession(w http.ResponseWriter, r *http.Request) (*Session, error) {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	isDesktop := false

	if value, ok := r.Context().Value(desktopContextKey).(bool); ok {
		isDesktop = value
	}

	if isDesktop {
		if desktopSession == nil {
			sessionID, err := generateSessionID()
			if err != nil {
				return nil, err
			}

			desktopSession = &Session{
				ID:         sessionID,
				DeviceName: detectDeviceName(r.UserAgent()),
				LastSeen:   time.Now(),
			}

			sessions[sessionID] = desktopSession
		}

		desktopSession.LastSeen = time.Now()

		return desktopSession, nil
	}

	cookie, err := r.Cookie(sessionCookieName)

	if err == nil && isValidSessionID(cookie.Value) {
		if session, exists := sessions[cookie.Value]; exists {
			session.LastSeen = time.Now()
			return session, nil
		}
	}

	if len(sessions) >= maxDevices {
		return nil, fmt.Errorf(
			"two devices are already connected",
		)
	}

	sessionID, err := generateSessionID()
	if err != nil {
		return nil, err
	}

	session := &Session{
		ID:         sessionID,
		DeviceName: detectDeviceName(r.UserAgent()),
		LastSeen:   time.Now(),
	}

	sessions[sessionID] = session

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 365 * 10,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	return session, nil
}

// Validates session
func isValidSessionID(id string) bool {
	if len(id) != 64 {
		return false
	}

	for _, char := range id {
		if !((char >= '0' && char <= '9') ||
			(char >= 'a' && char <= 'f') ||
			(char >= 'A' && char <= 'F')) {
			return false
		}
	}

	return true
}

func cleanupSessions() {
	for {
		time.Sleep(10 * time.Second)

		sessionMu.Lock()

		for id, session := range sessions {
			if desktopSession != nil && id == desktopSession.ID {
				continue
			}

			if time.Since(session.LastSeen) > sessionTimeout {
				delete(sessions, id)
			}
		}

		sessionMu.Unlock()
	}
}

// Detects the device type from the browser user agent
func detectDeviceName(userAgent string) string {
	ua := strings.ToLower(userAgent)

	if strings.Contains(ua, "smart-tv") ||
		strings.Contains(ua, "smarttv") ||
		strings.Contains(ua, "googletv") ||
		strings.Contains(ua, "netcast") ||
		strings.Contains(ua, "tizen") ||
		strings.Contains(ua, "webos") ||
		strings.Contains(ua, "hbbtv") {
		return "TV"
	}

	if strings.Contains(ua, "iphone") ||
		strings.Contains(ua, "ipad") ||
		strings.Contains(ua, "android") {
		return "Phone / Tablet"
	}

	if strings.Contains(ua, "windows") ||
		strings.Contains(ua, "macintosh") ||
		strings.Contains(ua, "linux") {
		return "Laptop / Computer"
	}

	return "Unknown Device"
}

// Get the directory belonging to a device
func getSessionDir(session *Session) string {
	return filepath.Join(uploadDir, session.ID)
}

func ensureSessionDir(session *Session) error {
	return os.MkdirAll(getSessionDir(session), os.ModePerm)
}

// Find the other connected device
func getOtherSession(session *Session) *Session {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	for id, other := range sessions {
		if id != session.ID {
			return other
		}
	}

	return nil
}

// Get files in the device's session automatically
func getSessionFiles(session *Session) ([]FileEntry, error) {
	files, err := os.ReadDir(getSessionDir(session))
	if err != nil {
		return nil, err
	}

	var fileNames []FileEntry

	for _, file := range files {
		if !file.IsDir() &&
			!strings.HasPrefix(file.Name(), ".") &&
			file.Name() != "history.json" {
			fileNames = append(fileNames, FileEntry{
				Name: file.Name(),
			})
		}
	}

	return fileNames, nil
}

// 2. Safely parse embedded template for root "/" route across desktop and Android WebViews
func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	session, err := getSession(w, r)
	if err != nil {
		http.Error(w, "Two devices are already connected.", http.StatusConflict)
		return
	}

	if err := ensureSessionDir(session); err != nil {
		http.Error(w, "Unable to create session directory", http.StatusInternalServerError)
		return
	}

	files, err := os.ReadDir(getSessionDir(session))
	if err != nil {
		http.Error(w, "Unable to read files", http.StatusInternalServerError)
		return
	}

	var fileNames []FileEntry

	for _, file := range files {
		if !file.IsDir() &&
			!strings.HasPrefix(file.Name(), ".") &&
			file.Name() != "history.json" {
			fileNames = append(fileNames, FileEntry{
				Name: file.Name(),
			})
		}
	}

	other := getOtherSession(session)

	data := HomeData{
		Files:    fileNames,
		HasPeer:  other != nil,
		ThisName: session.DeviceName,
	}

	if other != nil {
		data.PeerName = other.DeviceName
	}

	// Use template.ParseFS to resolve templates reliably from embedded FS in Wails
	tmpl, err := template.ParseFS(embeddedFiles, "template/index.html")
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to parse page template: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "Unable to render page", http.StatusInternalServerError)
		return
	}
}

// Returns information about the two connected devices
func handleDevices(w http.ResponseWriter, r *http.Request) {
	session, err := getSession(w, r)
	if err != nil {
		http.Error(w, "Two devices are already connected.", http.StatusConflict)
		return
	}

	other := getOtherSession(session)

	response := DeviceResponse{
		ThisDevice: session.DeviceName,
	}

	if other != nil {
		response.OtherDevice = other.DeviceName
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Unable to encode device information", http.StatusInternalServerError)
	}
}

// Detects specific device
func handleDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session, err := getSession(w, r)
	if err != nil {
		http.Error(w, "Two devices are already connected.", http.StatusConflict)
		return
	}

	deviceName := r.FormValue("name")

	if deviceName != "" {
		sessionMu.Lock()
		session.DeviceName = deviceName
		sessionMu.Unlock()
	}

	w.WriteHeader(http.StatusOK)
}

// Processes multipart form data file uploads
func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session, err := getSession(w, r)
	if err != nil {
		http.Error(w, "Two devices are already connected.", http.StatusConflict)
		return
	}

	if err := ensureSessionDir(session); err != nil {
		http.Error(w, "Unable to create session directory", http.StatusInternalServerError)
		return
	}

	// Parse up to 200MB files into memory; anything larger streams to temp files automatically
	err = r.ParseMultipartForm(200 << 20)
	if err != nil {
		http.Error(w, "File too large", http.StatusBadRequest)
		return
	}

	files := r.MultipartForm.File["files"]

	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		filename := filepath.Base(fileHeader.Filename)

		dstPath := filepath.Join(getSessionDir(session), filename)

		dst, err := os.Create(dstPath)
		if err != nil {
			file.Close()

			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		_, copyErr := io.Copy(dst, file)

		dst.Close()
		file.Close()

		if copyErr != nil {
			http.Error(w, copyErr.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "Upload Complete")
}

// Sends a file from this device to the other connected device
func handleTransfer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session, err := getSession(w, r)
	if err != nil {
		http.Error(w, "Unable to create session", http.StatusInternalServerError)
		return
	}

	other := getOtherSession(session)

	if other == nil {
		http.Error(w, "No other device is connected.", http.StatusConflict)
		return
	}

	filename := filepath.Base(r.FormValue("file"))
	action := r.FormValue("action")

	if filename == "" || filename == "." {
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}

	if action != "copy" && action != "move" {
		http.Error(w, "Invalid transfer action", http.StatusBadRequest)
		return
	}

	sourceDir := getSessionDir(session)
	destinationDir := getSessionDir(other)

	source := filepath.Join(sourceDir, filename)

	if _, err := os.Stat(source); err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	if err := ensureSessionDir(other); err != nil {
		http.Error(w, "Unable to create destination directory", http.StatusInternalServerError)
		return
	}

	destination := getUniqueFilename(destinationDir, filename)

	if err := copyFile(source, destination); err != nil {
		http.Error(w, "Unable to transfer file", http.StatusInternalServerError)
		return
	}

	if action == "move" {
		if err := os.Remove(source); err != nil {
			// Remove the destination again so a failed move
			// does not leave an unexpected duplicate.
			_ = os.Remove(destination)

			http.Error(w, "Unable to complete move", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)

	if action == "move" {
		fmt.Fprint(w, "File moved successfully")
	} else {
		fmt.Fprint(w, "File copied successfully")
	}
}

// Copies a file from one device directory to another
func copyFile(source, destination string) error {
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(destination)
	if err != nil {
		return err
	}

	_, err = io.Copy(dst, src)

	if closeErr := dst.Close(); err == nil {
		err = closeErr
	}

	return err
}

// Generates a unique filename if the destination already contains the file
func getUniqueFilename(dir, filename string) string {
	extension := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, extension)

	candidate := filepath.Join(dir, filename)

	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate
	}

	for i := 1; ; i++ {
		candidateName := fmt.Sprintf("%s_copy_%d%s", base, i, extension)

		candidate = filepath.Join(dir, candidateName)

		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

// Downloads a file belonging only to the current device
func handleDownload(w http.ResponseWriter, r *http.Request) {
	session, err := getSession(w, r)
	if err != nil {
		http.Error(w, "Unable to create session", http.StatusInternalServerError)
		return
	}

	filename := strings.TrimPrefix(r.URL.Path, "/download/")

	filename = filepath.Base(filename)

	if filename == "" || filename == "." {
		http.NotFound(w, r)
		return
	}

	filePath := filepath.Join(getSessionDir(session), filename)

	info, err := os.Stat(filePath)

	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	recordDownload(session, filename)

	http.ServeFile(w, r, filePath)
}

// Records download history for the current device
func recordDownload(session *Session, filename string) {
	historyPath := filepath.Join(getSessionDir(session), "history.json")

	var history []DownloadRecord

	data, err := os.ReadFile(historyPath)
	if err == nil {
		_ = json.Unmarshal(data, &history)
	}

	history = append(history, DownloadRecord{
		FileName:     filename,
		DownloadedAt: time.Now(),
	})

	data, err = json.MarshalIndent(history, "", "    ")

	if err != nil {
		return
	}

	_ = os.WriteFile(historyPath, data, 0644)
}

func handleFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session, err := getSession(w, r)
	if err != nil {
		http.Error(w, "Two devices are already connected.", http.StatusConflict)
		return
	}

	files, err := os.ReadDir(getSessionDir(session))
	if err != nil {
		http.Error(w, "Unable to read files", http.StatusInternalServerError)
		return
	}

	var fileNames []FileEntry

	for _, file := range files {
		if !file.IsDir() &&
			!strings.HasPrefix(file.Name(), ".") &&
			file.Name() != "history.json" {
			fileNames = append(fileNames, FileEntry{
				Name: file.Name(),
			})
		}
	}

	response := FilesResponse{
		Files:   fileNames,
		HasPeer: getOtherSession(session) != nil,
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Unable to encode file information", http.StatusInternalServerError)
	}
}

// getLocalIP determines the local IP address that should be reachable
// by other devices on the same network.
// getLocalIP returns the IPv4 address of the active LAN interface.
func getLocalIP() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}

	for _, iface := range interfaces {
		// Ignore interfaces that are down or loopback interfaces.
		if iface.Flags&net.FlagUp == 0 ||
			iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP

			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.To4() == nil {
				continue
			}

			// Ignore WSL's internal 10.x address.
			if ip.IsLoopback() ||
				ip.IsLinkLocalUnicast() ||
				ip.To4()[0] == 10 {
				continue
			}

			return ip.To4().String()
		}
	}

	return "127.0.0.1"
}
