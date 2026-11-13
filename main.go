// Command transfer-file runs a minimal WebDAV server for ad-hoc PC-to-PC
// file transfer over a LAN. It has no authentication and is intended for
// use on trusted local networks only.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"golang.org/x/net/webdav"
)

func main() {
	dir := flag.String("dir", "./shared", "directory to share over WebDAV")
	port := flag.Int("port", 8081, "TCP port to listen on")
	readOnly := flag.Bool("readonly", false, "disallow writes (serve files read-only)")
	flag.Parse()

	absDir, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatalf("resolve dir: %v", err)
	}
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		log.Fatalf("create shared dir: %v", err)
	}

	handler := &webdav.Handler{
		Prefix:     "/",
		FileSystem: webdav.Dir(absDir),
		LockSystem: webdav.NewMemLS(),
		Logger: func(r *http.Request, err error) {
			if err != nil {
				log.Printf("%s %s -> error: %v", r.Method, r.URL.Path, err)
			} else {
				log.Printf("%s %s", r.Method, r.URL.Path)
			}
		},
	}

	mux := http.NewServeMux()
	mux.Handle("/", withReadOnly(*readOnly, handler))

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Sharing %q as WebDAV root", absDir)
	if *readOnly {
		log.Printf("Mode: read-only")
	}
	for _, url := range lanURLs(*port) {
		log.Printf("Available at: %s", url)
	}
	log.Printf("Listening on %s (Ctrl+C to stop)", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// withReadOnly rejects WebDAV write methods when readOnly is true.
func withReadOnly(readOnly bool, next http.Handler) http.Handler {
	if !readOnly {
		return next
	}
	writeMethods := map[string]bool{
		"PUT": true, "DELETE": true, "MKCOL": true,
		"COPY": true, "MOVE": true, "PROPPATCH": true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeMethods[r.Method] {
			http.Error(w, "server is read-only", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// lanURLs returns http://<ip>:<port>/ for every non-loopback IPv4 address
// found on the machine, so the user can tell what to type on the other PC.
func lanURLs(port int) []string {
	var urls []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return urls
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
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
			if ip == nil || ip.To4() == nil || ip.IsLoopback() {
				continue
			}
			urls = append(urls, fmt.Sprintf("http://%s:%d/", ip.String(), port))
		}
	}
	return urls
}
