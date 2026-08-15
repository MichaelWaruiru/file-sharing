package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"
)

// The folder where shared files will be saved on your machine
const uploadDir = "./shared_files"
const sessionCookieName = "filedrop_session"
const maxDevices = 2

// Keep track of connected browser sessions
var (
	sessionMu sync.Mutex
	sessions  = make(map[string]*Session)
)

// HTML Template with embedded CSS and JavaScript for the drag-and-drop interface
const htmlTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>File Drop</title>

    <style>
        :root {
            --background: #121212;
            --surface: #1a1a1a;
            --surface-hover: #2d3748;
            --text: #e0e0e0;
            --heading: #ffffff;
            --muted: #718096;
            --file-text: #a0aec0;
            --border: #2d3748;
            --primary: #4299e1;
            --primary-hover: #63b3ed;
            --success: #48bb78;
            --danger: #f56565;
        }

        body.light-mode {
            --background: #f7fafc;
            --surface: #ffffff;
            --surface-hover: #edf2f7;
            --text: #2d3748;
            --heading: #1a202c;
            --muted: #718096;
            --file-text: #4a5568;
            --border: #e2e8f0;
            --primary: #3182ce;
            --primary-hover: #2b6cb0;
            --success: #38a169;
            --danger: #e53e3e;
        }

        * {
            box-sizing: border-box;
        }

        html {
            transition: background-color 0.2s ease;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            background-color: var(--background);
            color: var(--text);
            margin: 0;
            padding: 20px;
            min-height: 100vh;
            display: flex;
            flex-direction: column;
            align-items: center;
            transition: background-color 0.2s ease, color 0.2s ease;
        }

        h1 {
            color: var(--heading);
            text-align: center;
            margin: 10px 0;
        }

        h2 {
            color: var(--heading);
            text-align: center;
            margin: 25px 0 15px;
        }

        p {
            text-align: center;
        }

        .top-bar {
            width: 100%;
            max-width: 900px;
            display: flex;
            justify-content: flex-end;
            margin-bottom: 10px;
        }

        #theme-toggle {
            border: 1px solid var(--border);
            border-radius: 6px;
            padding: 8px 12px;
            cursor: pointer;
            background: var(--surface);
            color: var(--text);
            font-size: 14px;
        }

        #theme-toggle:hover,
        #theme-toggle:focus {
            background: var(--surface-hover);
            outline: 2px solid var(--primary);
            outline-offset: 2px;
        }

        #device-info {
            width: 100%;
            max-width: 900px;
            background: var(--surface);
            border-radius: 8px;
            padding: 15px;
            margin-bottom: 20px;
        }

        .device-row {
            display: flex;
            justify-content: space-between;
            gap: 20px;
            padding: 6px 0;
        }

        .label {
            color: var(--muted);
        }

        .value {
            color: var(--heading);
            font-weight: 600;
            text-align: right;
        }

        #connection-status {
            margin-top: 8px;
            color: var(--success);
        }

        .notice {
            width: 100%;
            max-width: 900px;
            padding: 12px;
            margin-bottom: 20px;
            border-radius: 6px;
            background: var(--surface-hover);
            display: none;
        }

        #drop-zone {
            border: 2px dashed var(--primary);
            border-radius: 8px;
            width: 100%;
            max-width: 850px;
            padding: 50px 20px;
            text-align: center;
            cursor: pointer;
            background: var(--surface);
            transition: background-color 0.2s ease, border-color 0.2s ease;
            margin-bottom: 30px;
        }

        #drop-zone.hover {
            background: var(--surface-hover);
            border-color: var(--primary-hover);
        }

        #drop-zone:focus {
            outline: 2px solid var(--primary);
            outline-offset: 3px;
        }

        #file-input {
            display: none;
        }

        .file-list {
            width: 100%;
            max-width: 900px;
            background: var(--surface);
            border-radius: 8px;
            padding: 10px;
        }

        .file-item {
            display: flex;
            justify-content: space-between;
            align-items: center;
            gap: 15px;
            padding: 12px;
            border-bottom: 1px solid var(--border);
        }

        .file-item:last-child {
            border-bottom: none;
        }

        .file-name {
            color: var(--file-text);
            text-decoration: none;
            font-weight: 500;
            word-break: break-word;
            min-width: 0;
        }

        .file-name:hover,
        .file-name:focus {
            color: var(--primary);
            outline: none;
            text-decoration: underline;
        }

        .file-actions {
            display: flex;
            gap: 6px;
            flex-shrink: 0;
        }

        button {
            border: none;
            border-radius: 5px;
            padding: 8px 12px;
            cursor: pointer;
            background: var(--surface-hover);
            color: var(--text);
            font-size: 14px;
        }

        button:hover {
            background: var(--primary);
            color: #ffffff;
        }

        button:focus {
            outline: 2px solid var(--primary);
            outline-offset: 2px;
        }

        button:disabled {
            opacity: 0.5;
            cursor: not-allowed;
        }

        .status {
            margin-top: 10px;
            color: var(--success);
            font-weight: bold;
        }

        @media (max-width: 600px) {
            body {
                padding: 12px;
            }

            h1 {
                font-size: 26px;
            }

            h2 {
                font-size: 21px;
            }

            #device-info,
            .file-list,
            .notice {
                border-radius: 6px;
            }

            #drop-zone {
                padding: 35px 15px;
            }

            .file-item {
                align-items: flex-start;
                flex-direction: column;
            }

            .file-name {
                width: 100%;
            }

            .file-actions {
                width: 100%;
            }

            .file-actions button {
                flex: 1;
            }
        }

        @media (min-width: 601px) and (max-width: 1024px) {
            body {
                padding: 18px;
            }

            #device-info,
            .file-list,
            .notice {
                max-width: 800px;
            }

            #drop-zone {
                max-width: 750px;
            }
        }

        @media (min-width: 1025px) {
            body {
                padding: 30px;
            }

            #device-info,
            .file-list,
            .notice {
                max-width: 1000px;
            }

            #drop-zone {
                max-width: 900px;
            }
        }

        @media (min-width: 1600px) {
            body {
                padding: 40px;
            }

            h1 {
                font-size: 34px;
            }

            h2 {
                font-size: 26px;
            }

            #device-info,
            .file-list,
            .notice {
                max-width: 1100px;
            }

            #drop-zone {
                max-width: 1000px;
                padding: 70px 30px;
            }

            button {
                padding: 10px 16px;
                font-size: 16px;
            }
        }

        .progress-container {
            width: 100%;
            max-width: 900px;
            margin: 10px 0 20px;
            display: none;
        }

        .progress-info {
            display: flex;
            justify-content: space-between;
            margin-bottom: 6px;
            font-size: 14px;
            color: var(--muted);
        }

        .progress-bar {
            width: 100%;
            height: 8px;
            overflow: hidden;
            border-radius: 4px;
            background: var(--surface-hover);
        }

        .progress-bar-fill {
            width: 0%;
            height: 100%;
            background: var(--primary);
            transition: width 0.15s ease;
        }

        .progress-bar-fill.indeterminate {
            width: 40%;
            animation: progress-indeterminate 1.2s infinite ease-in-out;
        }

        @keyframes progress-indeterminate {
            0% {
                transform: translateX(-100%);
            }

            100% {
                transform: translateX(350%);
            }
        }
    </style>
</head>

<body>

    <div class="top-bar">
        <button id="theme-toggle" onclick="toggleTheme()">
            ☀
        </button>
    </div>

    <h1>File Drop</h1>

    <p>
        Send and receive files on your Phone, Laptop or TV.
    </p>

    <div id="device-info">
        <div class="device-row">
            <span class="label">This device</span>
            <span class="value" id="this-device">Detecting...</span>
        </div>

        <div class="device-row">
            <span class="label">Connected device</span>
            <span class="value" id="other-device">Waiting...</span>
        </div>

        <div id="connection-status">
            Waiting for another device...
        </div>
    </div>

    <div id="notice" class="notice"></div>

    <div id="drop-zone" tabindex="0">
        <p>Drag & Drop files here, or <strong>click to browse</strong></p>
        <input type="file" id="file-input" multiple>
        <div id="status" class="status"></div>
    </div>

    <div class="progress-container" id="progress-container">
        <div class="progress-info">
            <span id="progress-label">Preparing...</span>
            <span id="progress-percent">0%</span>
        </div>

        <div class="progress-bar">
            <div class="progress-bar-fill" id="progress-bar-fill"></div>
        </div>
    </div>

    <h2>My Files</h2>

    <div class="file-list" id="file-list">
        {{if .Files}}
            {{range .Files}}
                <div class="file-item">

                    <a class="file-name"
                       href="/download/{{.Name}}"
                       download
                       tabindex="0">
                        {{.Name}}
                    </a>

                    <div class="file-actions">

                        <button
                            onclick="transferFile('{{.Name}}', 'copy')"
                            {{if not $.HasPeer}}disabled{{end}}>
                            Copy
                        </button>

                        <button
                            onclick="transferFile('{{.Name}}', 'move')"
                            {{if not $.HasPeer}}disabled{{end}}>
                            Move
                        </button>

                    </div>
                </div>
            {{end}}
        {{else}}
            <p style="text-align: center; color: var(--muted); padding: 20px;">
                No files shared yet.
            </p>
        {{end}}
    </div>

    <script>
        const dropZone = document.getElementById('drop-zone');
        const fileInput = document.getElementById('file-input');
        const statusDiv = document.getElementById('status');
        const thisDevice = document.getElementById('this-device');
        const otherDevice = document.getElementById('other-device');
        const connectionStatus = document.getElementById('connection-status');
        const notice = document.getElementById('notice');
        const themeToggle = document.getElementById('theme-toggle');
        const fileList = document.getElementById('file-list');
        const progressContainer = document.getElementById('progress-container');
        const progressLabel = document.getElementById('progress-label');
        const progressPercent = document.getElementById('progress-percent');
        const progressBarFill = document.getElementById('progress-bar-fill');

        // Helper functions to show progress
        function showProgress(label) {
            progressContainer.style.display = "block";
            progressLabel.textContent = label;
            progressPercent.textContent = "0%";
            progressBarFill.classList.remove("indeterminate");
            progressBarFill.style.width = "0%";
        }

        function updateProgress(percent, label) {
            progressContainer.style.display = "block";
            progressLabel.textContent = label;
            progressPercent.textContent = percent + "%";
            progressBarFill.classList.remove("indeterminate");
            progressBarFill.style.width = percent + "%";
        }

        function showIndeterminateProgress(label) {
            progressContainer.style.display = "block";
            progressLabel.textContent = label;
            progressPercent.textContent = "";
            progressBarFill.style.width = "40%";
            progressBarFill.classList.add("indeterminate");
        }

        function hideProgress() {
            progressContainer.style.display = "none";
            progressBarFill.classList.remove("indeterminate");
            progressBarFill.style.width = "0%";
        }
        // Load the user's saved theme preference
        function loadTheme() {
            const savedTheme = localStorage.getItem('filedrop-theme');

            if (savedTheme === 'light') {
                document.body.classList.add('light-mode');
                themeToggle.textContent = "🌙";
            } else {
                document.body.classList.remove('light-mode');
                themeToggle.textContent = "☀";
            }
        }

        // Toggle between dark and light mode
        function toggleTheme() {
            document.body.classList.toggle('light-mode');

            const isLight = document.body.classList.contains('light-mode');

            if (isLight) {
                localStorage.setItem('filedrop-theme', 'light');
                themeToggle.textContent = "🌙";
            } else {
                localStorage.setItem('filedrop-theme', 'dark');
                themeToggle.textContent = "☀";
            }
        }

        dropZone.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' || e.key === ' ') {
                fileInput.click();
            }
        });

        dropZone.addEventListener('click', () => fileInput.click());

        dropZone.addEventListener('dragover', (e) => {
            e.preventDefault();
            dropZone.classList.add('hover');
        });

        dropZone.addEventListener('dragleave', () => {
            dropZone.classList.remove('hover');
        });

        dropZone.addEventListener('drop', (e) => {
            e.preventDefault();
            dropZone.classList.remove('hover');
            handleFiles(e.dataTransfer.files);
        });

        fileInput.addEventListener('change', () => {
            handleFiles(fileInput.files);
        });

        function handleFiles(files) {
            if (files.length === 0) {
                return;
            }

            const formData = new FormData();

            for (let i = 0; i < files.length; i++) {
                formData.append('files', files[i]);
            }

            showProgress("Uploading...");

            const xhr = new XMLHttpRequest();

            xhr.open("POST", "/upload");

            xhr.upload.addEventListener("progress", (event) => {
                if (!event.lengthComputable) {
                    return;
                }

                const percent = Math.round(
                    (event.loaded / event.total) * 100
                );

                updateProgress(
                    percent,
                    "Uploading " + files.length + " file(s)..."
                );
            });

            xhr.addEventListener("load", () => {
                if (xhr.status >= 200 && xhr.status < 300) {
                    updateProgress(100, "Upload complete");

                    statusDiv.textContent = "Upload successful!";
                    updateFiles();

                    setTimeout(() => {
                        hideProgress();
                    }, 800);

                    return;
                }

                hideProgress();
                statusDiv.textContent =
                    xhr.responseText || "Upload failed.";
            });

            xhr.addEventListener("error", () => {
                hideProgress();
                statusDiv.textContent = "Network error occurred.";
            });

            xhr.addEventListener("abort", () => {
                hideProgress();
                statusDiv.textContent = "Upload cancelled.";
            });

            xhr.send(formData);
        }

        function transferFile(filename, action) {
            let message;

            if (action === "move") {
                message =
                    "Move \"" + filename +
                    "\" to the other device?\n\n" +
                    "The file will be removed from this device.";
            } else {
                message =
                    "Copy \"" + filename +
                    "\" to the other device?";
            }

            if (!confirm(message)) {
                return;
            }

            const formData = new URLSearchParams();

            formData.append("file", filename);
            formData.append("action", action);

            showIndeterminateProgress(
                action === "move"
                    ? "Moving " + filename + "..."
                    : "Copying " + filename + "..."
            );

            fetch("/transfer", {
                method: "POST",
                headers: {
                    "Content-Type": "application/x-www-form-urlencoded"
                },
                body: formData
            })
            .then(response => {
                if (!response.ok) {
                    return response.text().then(message => {
                        throw new Error(message);
                    });
                }

                return response.text();
            })
            .then(() => {
                progressPercent.textContent = "100%";
                progressBarFill.classList.remove("indeterminate");
                progressBarFill.style.width = "100%";

                statusDiv.textContent =
                    action === "move"
                        ? "File moved to the other device."
                        : "File copied to the other device.";

                updateFiles();

                setTimeout(() => {
                    hideProgress();
                }, 800);
            })
            .catch(error => {
                hideProgress();
                alert(error.message || "Unable to transfer file.");
            });
        }

        async function detectDeviceName() {
            const ua = navigator.userAgent;

            if (/iPhone/i.test(ua)) {
                return "iPhone";
            }

            if (/iPad/i.test(ua)) {
                return "iPad";
            }

            if (/Android/i.test(ua)) {
                if (navigator.userAgentData) {
                    try {
                        const data = await navigator.userAgentData.getHighEntropyValues([
                            "model"
                        ]);

                        if (data.model) {
                            return data.model;
                        }
                    } catch (error) {
                        console.warn("Unable to detect Android model:", error);
                    }
                }

                return /Mobile/i.test(ua)
                    ? "Android Phone"
                    : "Android Tablet";
            }

            if (/SMART-TV|SmartTV|Tizen/i.test(ua)) {
                return "Samsung TV";
            }

            if (/Web0S|WebOS/i.test(ua)) {
                return "LG TV";
            }

            if (/Windows/i.test(ua)) {
                return "Windows PC";
            }

            if (/Macintosh|MacIntel/i.test(ua)) {
                return "Mac";
            }

            if (/Linux/i.test(ua)) {
                return "Linux PC";
            }

            return "Unknown Device";
        }

        function updateDevices() {
            fetch('/devices')
                .then(response => {
                    if (!response.ok) {
                        throw new Error("Unable to check devices.");
                    }

                    return response.json();
                })
                .then(data => {
                    thisDevice.textContent = data.this_device;

                    if (data.other_device) {
                        otherDevice.textContent = data.other_device;
                        connectionStatus.textContent =
                            "Connected to " + data.other_device;
                    } else {
                        otherDevice.textContent = "Waiting...";
                        connectionStatus.textContent =
                            "Waiting for another device...";
                    }
                })
                .catch(() => {
                    connectionStatus.textContent =
                        "Unable to check connection.";
                });
        }

        function updateFiles() {
            fetch('/files')
                .then(response => {
                    if (!response.ok) {
                        throw new Error("Unable to check files.");
                    }

                    return response.json();
                })
                .then(data => {
                    fileList.replaceChildren();

                    if (data.files.length === 0) {
                        const emptyMessage = document.createElement('p');

                        emptyMessage.style.textAlign = "center";
                        emptyMessage.style.color = "var(--muted)";
                        emptyMessage.style.padding = "20px";
                        emptyMessage.textContent = "No files shared yet.";

                        fileList.appendChild(emptyMessage);
                        return;
                    }

                    data.files.forEach(file => {
                        const item = document.createElement('div');
                        item.className = "file-item";

                        const link = document.createElement('a');
                        link.className = "file-name";
                        link.href = "/download/" + encodeURIComponent(file.name);
                        link.download = "";
                        link.tabIndex = 0;
                        link.textContent = file.name;

                        const actions = document.createElement('div');
                        actions.className = "file-actions";

                        const copyButton = document.createElement('button');
                        copyButton.textContent = "Copy";
                        copyButton.disabled = !data.has_peer;
                        copyButton.onclick = () => transferFile(file.name, "copy");

                        const moveButton = document.createElement('button');
                        moveButton.textContent = "Move";
                        moveButton.disabled = !data.has_peer;
                        moveButton.onclick = () => transferFile(file.name, "move");

                        actions.appendChild(copyButton);
                        actions.appendChild(moveButton);

                        item.appendChild(link);
                        item.appendChild(actions);

                        fileList.appendChild(item);
                    });
                })
                .catch(() => {
                    console.warn("Unable to update files.");
                });
        }

        loadTheme();
        updateDevices();
        updateFiles();

        // Detects specific device
        detectDeviceName().then(deviceName => {
            fetch("/device", {
                method: "POST",
                headers: {
                    "Content-Type": "application/x-www-form-urlencoded"
                },
                body: new URLSearchParams({
                    name: deviceName
                })
            }).then(() => {
                updateDevices();
            });
        });

        // Check for the second device periodically
        setInterval(updateDevices, 2000);

        // Check for file changes periodically
        setInterval(updateFiles, 1000);
    </script>
</body>
</html>
`

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

func main() {
	// Ensure the shared files directory exists
	if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
		fmt.Printf("Failed to create directory: %v\n", err)
		return
	}

	// Setup HTTP Server Route Handlers
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/upload", handleUpload)
	http.HandleFunc("/transfer", handleTransfer)
	http.HandleFunc("/download/", handleDownload)
	http.HandleFunc("/devices", handleDevices)
	http.HandleFunc("/device", handleDevice)
	http.HandleFunc("/files", handleFiles)

	// Discover your computer's real Wi-Fi IP address
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

	if err := http.ListenAndServe(port, nil); err != nil {
		fmt.Printf("Error starting server: %v\n", err)
	}
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

// Renders the dashboard listing all files belonging to this device
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

	tmpl := template.Must(template.New("index").Parse(htmlTemplate))

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
