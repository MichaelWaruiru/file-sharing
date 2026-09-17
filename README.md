# File Drop

A lightweight local-network file sharing application(both desktop and android app) built with **Go**. Transfer files between a **phone, laptop, or TV** without relying on cloud storage.

Right now this is a skeleton which isn't polished yet to connect to devices automatically, especially in cases there's no WIFI at all. I am just playing around to see the end product, or abandon it for the next 2 years. Lol!

## DISCLAIMER

Wails3 is still in beta stage so this may work for me, but not work for you. Fuck around and find it like I did!

## Features

*  Phone, Laptop & TV support
*  Upload and download files
*  Send a copy/move to the connected device
*  Works over the same local network
*  Terminal QR code for quick connection

## How It Works

Run the server on one device and open the generated network address on another device connected to the **same Wi-Fi/network**.

*Note:* It runs as desktop app, android app and on browser.

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
*Note*: For desktop applications:

Run:
```bash
 wails3 build (Ubuntu version)
```

Then:
 ```bash
    ./bin/file-drop
```

*Note*: For mobile applications:

Run:
```bash
 wails3 task android:build (Ubuntu version)
```

Make sure your emulator is running using:

```bash
emulator -avd wails &
```

Run:
```bash
 wails3 task android:run (Ubuntu version)
```
You can read wails documentation on how to go about desktop app [here](https://v3.wails.io/) and about mobile app [here](https://v3.wails.io/guides/mobile/first-mobile-app/).



The terminal will display a local network URL and a QR code.

Scan the QR code with the second device to connect or copy the second URL on the device.

## Security

Files are stored inside session-specific directories and are not exposed to other browser sessions.

> File Drop is designed for trusted local networks. It is not intended to be exposed directly to the public internet.

## License

Licensed under the [MIT License](LICENSE).