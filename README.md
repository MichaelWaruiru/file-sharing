# File Drop

A lightweight local-network file sharing application built with **Go**. Transfer files between a **phone, laptop, or TV** without relying on cloud storage.

## Features

* 📱 Phone, 💻 Laptop & 📺 TV support
* 🔗 Automatic browser sessions
* 🔒 Private session-based file storage
* 📤 Upload and download files
* 📋 Send a copy to the connected device
* 🚚 Send and move files to the connected device
* 🌐 Works over the same local network
* 📊 Download history per session
* 🔲 Terminal QR code for quick connection

## How It Works

Run the server on one device and open the generated network address on another device connected to the **same Wi-Fi/network**.

The application supports **two connected devices at a time**:

`Phone ↔ Laptop`
`Phone ↔ TV`
`Laptop ↔ TV`

Each browser receives its own persistent session, keeping files isolated from other users.

## Requirements

* [Go](https://go.dev/)
* Devices connected to the same local network

## Platform Support

* 🐧 **Linux (Ubuntu)** — Supported
* 🪟 **Windows** — Under development

> Currently, File Drop is supported on Ubuntu. Windows support is in development.


## Run

```bash
go run .
```

The terminal will display a local network URL and a QR code.

Scan the QR code with the second device to connect.

## Project Structure

```
.
├── main.go
├── shared_files/
└── README.md
```

## Security

Files are stored inside session-specific directories and are not exposed to other browser sessions.

> File Drop is designed for trusted local networks. It is not intended to be exposed directly to the public internet.

## License

Licensed under the [MIT License](LICENSE).