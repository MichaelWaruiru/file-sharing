# File Drop

A lightweight local-network file sharing application built with **Go**. Transfer files between a **phone, laptop, or TV** without relying on cloud storage.

Right now this is a skeleton which isn't polished yet to connect to devices automatically, especially in cases there's no WIFI at all. I am just playing around to see the end product, or abandon it for the next 2 years. Lol!

## Features

*  Phone, Laptop & TV support
*  Upload and download files
*  Send a copy/move to the connected device
*  Works over the same local network
*  Terminal QR code for quick connection

## How It Works

Run the server on one device and open the generated network address on another device connected to the **same Wi-Fi/network**.

The application supports **two connected devices at a time**:

`Phone ↔ Laptop`
`Phone ↔ TV`
`Laptop ↔ TV`

Each browser receives its own persistent session, keeping files isolated from other users.


## Platform Support

*  **Linux (Ubuntu)** — Supported
*  **Windows** — Under development

> Currently, File Drop is supported on Ubuntu. Windows support is in development.


## Run

```bash
go run .
```

The terminal will display a local network URL and a QR code.

Scan the QR code with the second device to connect or copy the second URL on the device.

## Security

Files are stored inside session-specific directories and are not exposed to other browser sessions.

> File Drop is designed for trusted local networks. It is not intended to be exposed directly to the public internet.

## License

Licensed under the [MIT License](LICENSE).