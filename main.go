// transfer-file 명령은 LAN에서 PC 간 임시 파일 전송을 위한
// 최소 기능의 WebDAV 서버를 실행한다. 인증 기능이 없으므로
// 신뢰할 수 있는 로컬 네트워크에서만 사용해야 한다.
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
	dir := flag.String("dir", "./shared", "WebDAV로 공유할 디렉터리")
	port := flag.Int("port", 8081, "서버가 수신할 TCP 포트")
	readOnly := flag.Bool("readonly", false, "쓰기 금지(읽기 전용으로 서비스)")
	flag.Parse()

	absDir, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatalf("디렉터리 경로 확인 실패: %v", err)
	}
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		log.Fatalf("공유 디렉터리 생성 실패: %v", err)
	}

	handler := &webdav.Handler{
		Prefix:     "/",
		FileSystem: webdav.Dir(absDir),
		LockSystem: webdav.NewMemLS(),
		Logger: func(r *http.Request, err error) {
			if err != nil {
				log.Printf("%s %s -> 오류: %v", r.Method, r.URL.Path, err)
			} else {
				log.Printf("%s %s", r.Method, r.URL.Path)
			}
		},
	}

	mux := http.NewServeMux()
	mux.Handle("/", withReadOnly(*readOnly, handler))

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("%q 디렉터리를 WebDAV 루트로 공유합니다", absDir)
	if *readOnly {
		log.Printf("모드: 읽기 전용")
	}
	for _, url := range lanURLs(*port) {
		log.Printf("접속 주소: %s", url)
	}
	log.Printf("%s 포트에서 대기 중입니다 (Ctrl+C로 종료)", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("서버 오류: %v", err)
	}
}

// withReadOnly는 readOnly가 true일 때 WebDAV 쓰기 메서드를 거부한다.
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
			http.Error(w, "읽기 전용 서버입니다", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// lanURLs는 이 PC에 있는 루프백이 아닌 모든 IPv4 주소에 대해
// http://<ip>:<port>/ 형태의 URL을 반환한다. 다른 PC에서 접속할 때
// 입력할 주소를 사용자에게 알려주기 위함이다.
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
