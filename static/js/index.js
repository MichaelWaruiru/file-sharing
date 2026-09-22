// const API_BASE = "http://localhost:8080";
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
let detectedDeviceName = null;

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
        themeToggle.textContent = "☾";
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
        themeToggle.textContent = "☾";
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

    if (/BRAVIA|Sony/i.test(ua)) {
        return "Sony TV";
    }

    if (/SMART-TV|SmartTV|Tizen/i.test(ua)) {
        return "Samsung TV";
    }

    if (/WebOS/i.test(ua)) {
        return "LG TV";
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

        // Android WebViews commonly expose the model in the legacy UA even
        // when User-Agent Client Hints are unavailable.
        const modelMatch = ua.match(/Android[^;)]*;\s*(?:[a-z]{2}(?:-[A-Z]{2})?;\s*)?([^;)]+?)(?:\s+Build\/[^;)]+)?(?:[;)]|$)/i);
        if (modelMatch && modelMatch[1]) {
            const model = modelMatch[1].trim();
            if (model && !/^(wv|mobile)$/i.test(model)) {
                return model;
            }
        }

        return /Mobile/i.test(ua)
            ? "Android Phone"
            : "Android Tablet";
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
    fetch("/devices")
        .then(response => {
            if (!response.ok) {
                throw new Error("Unable to check devices.");
            }

            return response.json();
        })
        .then(data => {
            if (!detectedDeviceName || data.this_device !== "Unknown Device") {
                thisDevice.textContent = data.this_device;
            }

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
    fetch("/files")
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
                link.tabIndex = 0;
                link.textContent = file.name;

                if (window.go && window.go.main && window.go.main.App) {
                    link.href = "#";

                    link.addEventListener('click', async (event) => {
                        event.preventDefault();

                        try {
                            await window.go.main.App.DownloadFile(file.name);
                        } catch (error) {
                            console.error("Unable to download file:", error);
                        }
                    });
                } else {
                    link.href = "/download/" + encodeURIComponent(file.name);
                    link.download = "";
                }

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
    detectedDeviceName = deviceName;
    thisDevice.textContent = deviceName;

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
}).catch(() => {
    detectedDeviceName = "Unknown Device";
});

// Check for the second device periodically
setInterval(updateDevices, 2000);

// Check for file changes periodically
setInterval(updateFiles, 1000);