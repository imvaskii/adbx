# adbx

A terminal UI for Android wireless debugging. Discovers devices on your LAN via mDNS, handles pairing automatically, and connects with a single keypress — no IP addresses or manual `adb pair` commands needed.

## The problem it solves

Android's Wireless Debugging requires knowing the device's IP and port, running `adb pair <ip:port>`, entering a 6-digit code, then running `adb connect <ip:port>` with a *different* port. Getting this wrong (wrong port, stale pairing, dismissed dialog) produces cryptic `adb` error strings with no guidance.

`adbx` automates the entire flow:

1. Scans mDNS for all Android devices advertising wireless debug services
2. Detects when a device needs pairing and watches for the pairing dialog to open
3. Prompts for the 6-digit code, pairs, then auto-connects — no IP copying required

## Prerequisites

| Dependency | Linux | macOS |
|---|---|---|
| `adb` (Android SDK Platform Tools) | `sudo pacman -S android-tools` | `brew install android-platform-tools` |
| `avahi-daemon` | `sudo systemctl enable --now avahi-daemon` | not required |
| Android 11+ device | Settings → Developer Options → Wireless Debugging → enable | same |

## Install

```sh
# Recommended — builds from source, no Gatekeeper issues
go install github.com/imvaskii/adbx@latest

# Or build locally
make install   # builds and copies to ~/.local/bin/adbx
```

Pre-built binaries for Linux and macOS (arm64/amd64) are attached to each [release](https://github.com/imvaskii/adbx/releases).

### macOS: first run

macOS quarantines binaries downloaded from the internet. If you see *"Apple could not verify adbx is free of malware"* or *"adbx is damaged"*, remove the quarantine attribute:

```sh
xattr -d com.apple.quarantine ./adbx
```

Using `go install` (above) avoids this entirely — locally-built binaries are never quarantined.

## Usage

```sh
adbx                     # launch TUI — scan, select device, connect
adbx diag <ip:port>      # print raw adb output (debugging parser issues)
```

### Keybindings

| Key | Action |
|---|---|
| `j` / `k` | move down / up |
| `G` / `gg` | jump to bottom / top |
| `enter` | select device |
| `r` | rescan |
| `esc` | back |
| `q` / `ctrl+c` | quit |

## Build from source

```sh
make build       # native binary → bin/adbx
make build-all   # linux/darwin binaries → bin/
make test        # run tests
make help        # list all targets
```

## How it works

`adbx` watches two mDNS service types that Android publishes:

- `_adb-tls-connect._tcp` — device is paired and ready to connect
- `_adb-tls-pairing._tcp` — device's pairing dialog is currently open

### Linux

Uses `avahi-browse` for long-running service watches. This avoids multicast socket conflicts with the system `avahi-daemon`, which holds exclusive access to the mDNS port on most distros. A `zeroconf` scan runs once on startup for an immediate device list.

### macOS

Uses the three-step `dns-sd` pipeline:

1. `dns-sd -B` — browse: discovers service instances as they appear/disappear
2. `dns-sd -L` — lookup: resolves a service instance to hostname + port
3. `dns-sd -G v4` — get address: resolves hostname to IPv4 address

Each step is a separate subprocess; all three are injectable via a `subprocessRunner` seam for testing without network access.

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| No devices shown | mDNS not reaching host | Linux: check `avahi-daemon` is running; macOS: check Wi-Fi is on same network as device |
| "Cannot connect to daemon" | `adb` server not running | Run `adb start-server` |
| Pairing times out | Dialog dismissed on device | Re-open Wireless Debugging → Pair device with pairing code |
| "Apple could not verify" | macOS Gatekeeper quarantine | `xattr -d com.apple.quarantine ./adbx` or use `go install` |
| Wrong port after pairing | Stale `adb` state | Run `adb disconnect` then rescan with `r` |
| `adbx diag` shows daemon noise | Expected — `adb` prints startup lines to stdout | `adbx` strips these automatically |
