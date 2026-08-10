package main

import (
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/skip2/go-qrcode"
)

// The folder where shared files will be saved on your machine
const uploadDir = "./shared_files"

// HTML Template with embedded CSS and JavaScript for the drag-and-drop interface
const htmlTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Go Local File Share</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; background-color: #121212; color: #e0e0e0; margin: 0; padding: 20px; display: flex; flex-direction: column; align-items: center; }
        h1 { color: #ffffff; }
        #drop-zone { border: 2px dashed #4299e1; border-radius: 8px; width: 100%; max-width: 600px; padding: 40px 20px; text-align: center; cursor: pointer; background: #1a1a1a; transition: background 0.3s ease; margin-bottom: 30px; }
        #drop-zone.hover { background: #2d3748; border-color: #63b3ed; }
        #file-input { display: none; }
        .file-list { width: 100%; max-width: 620px; background: #1a1a1a; border-radius: 8px; padding: 10px; box-sizing: border-box; }
        .file-item { display: flex; justify-content: space-between; align-items: center; padding: 12px; border-bottom: 1px solid #2d3748; }
        .file-item:last-child { border-bottom: none; }
        .file-name { color: #a0aec0; text-decoration: none; font-weight: 500; }
        .file-name:hover, .file-name:focus { color: #4299e1; outline: none; text-decoration: underline; }
        .status { margin-top: 10px; color: #48bb78; font-weight: bold; }
    </style>
</head>
<body>

    <h1>Go Local File Drop</h1>
    <p>Access this dashboard on your Phone or TV Browser to send/receive files.</p>

    <div id="drop-zone" tabindex="0">
        <p>Drag & Drop files here, or <strong>click to browse</strong></p>
        <input type="file" id="file-input" multiple>
        <div id="status" class="status"></div>
    </div>

    <h2>Shared Files</h2>
    <div class="file-list">
        {{if .}}
            {{range .}}
                <div class="file-item">
                    <a class="file-name" href="/download/{{.}}" download tabindex="0">{{.}}</a>
                </div>
            {{end}}
        {{else}}
            <p style="text-align: center; color: #718096; padding: 20px;">No files shared yet.</p>
        {{end}}
    </div>

    <script>
        const dropZone = document.getElementById('drop-zone');
        const fileInput = document.getElementById('file-input');
        const statusDiv = document.getElementById('status');

        // Allow D-Pad / Keyboard activation of the drop zone
        dropZone.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' || e.key === ' ') { fileInput.click(); }
        });

        dropZone.addEventListener('click', () => fileInput.click());

        dropZone.addEventListener('dragover', (e) => { e.preventDefault(); dropZone.classList.add('hover'); });
        dropZone.addEventListener('dragleave', () => dropZone.classList.remove('hover'));
        dropZone.addEventListener('drop', (e) => {
            e.preventDefault();
            dropZone.classList.remove('hover');
            handleFiles(e.dataTransfer.files);
        });

        fileInput.addEventListener('change', () => handleFiles(fileInput.files));

        function handleFiles(files) {
            if (files.length === 0) return;
            statusDiv.textContent = "Uploading " + files.length + " file(s)...";
            
            const formData = new FormData();
            for (let i = 0; i < files.length; i++) {
                formData.append('files', files[i]);
            }

            fetch('/upload', { method: 'POST', body: formData })
            .then(response => {
                if (response.ok) {
                    statusDiv.textContent = "Upload successful! Reloading...";
                    setTimeout(() => window.location.reload(), 1000);
                } else {
                    statusDiv.textContent = "Upload failed.";
                }
            })
            .catch(() => statusDiv.textContent = "Network error occurred.");
        }
    </script>
</body>
</html>
`

func main() {
	// Ensure the shared files directory exists
	if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
		fmt.Printf("Failed to create directory: %v\n", err)
		return
	}

	// Setup HTTP Server Route Handlers
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/upload", handleUpload)
	http.Handle("/download/", http.StripPrefix("/download/", http.FileServer(http.Dir(uploadDir))))

	// Discover your computer's real Wi-Fi IP address
	localIP := getLocalIP()
	port := ":8080"
	targetURL := fmt.Sprintf("http://%s%s", localIP, port)

	fmt.Printf("File Server started successfully!\n")
	fmt.Printf("Local Host: http://localhost%s\n", port)
	fmt.Printf("Target URL: %s\n", targetURL)
	fmt.Println("Scan this QR code with your phone to connect instantly:")
	fmt.Println("") // Empty space btwn qrcode

	// 2. Generate and print the terminal QR code
	// qrcode.Medium refers to error correction level. true inverses colors for dark terminals.
	qr, err := qrcode.New(targetURL, qrcode.Medium)
	if err == nil {
		// Prints using unicode block characters
		fmt.Print(qr.ToSmallString(false))
	} else {
		fmt.Printf("Could not generate QR code: %v\n", err)
	}

	if err := http.ListenAndServe(port, nil); err != nil {
		fmt.Printf("Error starting server: %v\n", err)
	}
}

// Renders the dashboard listing all active files
func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	files, err := os.ReadDir(uploadDir)
	var fileNames []string
	if err == nil {
		for _, file := range files {
			if !file.IsDir() && !strings.HasPrefix(file.Name(), ".") {
				fileNames = append(fileNames, file.Name())
			}
		}
	}

	tmpl := template.Must(template.New("index").Parse(htmlTemplate))
	tmpl.Execute(w, fileNames)
}

// Processes multipart form data file uploads
func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse up to 200MB files into memory; anything larger streams to temp files automatically
	err := r.ParseMultipartForm(200 << 20)
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
		defer file.Close()

		// Protect against directory traversal attacks by taking only the base name
		dstPath := filepath.Join(uploadDir, filepath.Base(fileHeader.Filename))
		dst, err := os.Create(dstPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer dst.Close()

		// Efficiently stream data from the network connection right onto your storage drive
		if _, err := io.Copy(dst, file); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "Upload Complete")
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
