# SavageProxy

A custom Minecraft proxy for version 26.1 and above. 

It handles Mojang authentication on the proxy side and forwards player data (UUID + Skins) to a backend Fabric server running in offline mode. 

## Structure
- `/`: Go source code for the proxy.
- `/bridge`: Fabric mod source code to handle the incoming player data.

## How to use
### 1. The Proxy (Go)
Build:
`go build -o savage-proxy.exe main.go`

Run:
`./savage-proxy.exe`
(Listens on :25565 and connects to :25566 by default).

### 2. The Bridge (Java)
Build:
`cd bridge && ./gradlew build`

Setup:
- Put the jar in your server's `mods` folder.
- Set `online-mode=false` in `server.properties`.

## Security
The proxy signs forwarding data with a secret key. The bridge mod verifies this signature to ensure only the proxy can log players in.
