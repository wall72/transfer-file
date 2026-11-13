# transfer-file

A minimal WebDAV server for ad-hoc PC-to-PC file transfer over a LAN.
No authentication, no TLS — intended only for trusted local networks
(e.g. moving files between two computers on the same Wi-Fi/Ethernet segment).

## Build

```powershell
go build -o transfer-file.exe .
```

## Run

```powershell
./transfer-file.exe -dir ./shared -port 8081
```

On startup the server prints the LAN URL(s) to use from another machine,
e.g. `http://192.168.1.23:8081/`.

### Flags

| Flag        | Default    | Description                                   |
|-------------|------------|------------------------------------------------|
| `-dir`      | `./shared` | Directory to expose over WebDAV                |
| `-port`     | `8081`     | TCP port to listen on                           |
| `-readonly` | `false`    | Reject write methods (PUT/DELETE/MKCOL/...)     |

## Connecting from another PC

- **Windows**: File Explorer → "Map network drive" → "Connect to a website" → enter `http://<ip>:8081/`.
- **macOS**: Finder → Go → "Connect to Server" → `http://<ip>:8081/`.
- **Linux**: `davfs2` or a file manager with WebDAV support, pointed at `http://<ip>:8081/`.

## Security

There is no authentication or encryption. Only run this on a network you
trust, and stop the server when you're done transferring files.
