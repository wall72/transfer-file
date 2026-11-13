// transfer-file 명령은 LAN에서 PC 간 임시 파일 전송을 위한
// 최소 기능의 WebDAV 서버를 실행한다. 인증 기능이 없으므로
// 신뢰할 수 있는 로컬 네트워크에서만 사용해야 한다.
package main

import (
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

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
	mux.Handle("/upload", uploadHandler(absDir, *readOnly))
	mux.Handle("/", withDirListing(absDir, *readOnly, withReadOnly(*readOnly, handler)))

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

// withDirListing은 GET/HEAD 요청 대상이 디렉터리일 경우
// WebDAV 핸들러(405 반환) 대신 업로드 폼이 포함된 HTML 디렉터리 목록을
// 보여준다. 파일 요청은 그대로 다음 핸들러(webdav.Handler)로 넘긴다.
func withDirListing(absDir string, readOnly bool, next http.Handler) http.Handler {
	dir := http.Dir(absDir)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			if f, err := dir.Open(r.URL.Path); err == nil {
				fi, statErr := f.Stat()
				if statErr == nil && fi.IsDir() {
					entries, _ := f.Readdir(-1)
					f.Close()
					renderDirListing(w, r.URL.Path, entries, readOnly)
					return
				}
				f.Close()
			}
		}
		next.ServeHTTP(w, r)
	})
}

// dirListingTmpl은 디렉터리 목록과 업로드 폼을 렌더링하는 템플릿이다.
var dirListingTmpl = template.Must(template.New("dir").Parse(`<!doctype html>
<meta name="viewport" content="width=device-width">
<title>{{.Path}}</title>
<h1>{{.Path}}</h1>
{{if .Parent}}<p><a href="{{.Parent}}">⬆ 상위 폴더</a></p>{{end}}
<ul>
{{range .Entries}}<li><a href="{{.Href}}">{{.Name}}</a></li>
{{else}}<li>(비어 있음)</li>
{{end}}
</ul>
{{if not .ReadOnly}}
<hr>
<h2>파일 업로드</h2>
<form method="post" action="/upload" enctype="multipart/form-data">
  <input type="hidden" name="dir" value="{{.Path}}">
  <input type="file" name="files" multiple>
  <button type="submit">업로드</button>
</form>
{{end}}
`))

type dirEntryView struct {
	Name string
	Href string
}

type dirListingView struct {
	Path     string
	Parent   string
	Entries  []dirEntryView
	ReadOnly bool
}

// renderDirListing은 디렉터리 항목을 이름순으로 정렬해 HTML로 출력한다.
func renderDirListing(w http.ResponseWriter, urlPath string, entries []os.FileInfo, readOnly bool) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	if !strings.HasSuffix(urlPath, "/") {
		urlPath += "/"
	}
	view := dirListingView{Path: urlPath, ReadOnly: readOnly}
	if urlPath != "/" {
		view.Parent = path.Dir(strings.TrimSuffix(urlPath, "/"))
		if !strings.HasSuffix(view.Parent, "/") {
			view.Parent += "/"
		}
	}
	for _, e := range entries {
		name := e.Name()
		href := url.PathEscape(name)
		if e.IsDir() {
			href += "/"
			name += "/"
		}
		view.Entries = append(view.Entries, dirEntryView{Name: name, Href: href})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := dirListingTmpl.Execute(w, view); err != nil {
		log.Printf("디렉터리 목록 렌더링 실패: %v", err)
	}
}

// uploadHandler는 업로드 폼의 POST 요청을 받아 파일을 공유 디렉터리(또는
// 그 하위 디렉터리)에 저장한다. readOnly가 true면 항상 거부한다.
func uploadHandler(absDir string, readOnly bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if readOnly {
			http.Error(w, "읽기 전용 서버입니다", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "허용되지 않는 메서드입니다", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, "업로드 양식을 해석할 수 없습니다: "+err.Error(), http.StatusBadRequest)
			return
		}
		targetDir, err := safeJoin(absDir, r.FormValue("dir"))
		if err != nil {
			http.Error(w, "잘못된 대상 경로입니다", http.StatusBadRequest)
			return
		}
		for _, fh := range r.MultipartForm.File["files"] {
			if err := saveUploadedFile(targetDir, fh); err != nil {
				http.Error(w, "파일 저장 실패: "+err.Error(), http.StatusInternalServerError)
				return
			}
			log.Printf("업로드 완료: %s", filepath.Join(targetDir, filepath.Base(fh.Filename)))
		}
		http.Redirect(w, r, r.FormValue("dir"), http.StatusSeeOther)
	})
}

// saveUploadedFile은 업로드된 파일 하나를 targetDir에 기록한다.
func saveUploadedFile(targetDir string, fh *multipart.FileHeader) error {
	src, err := fh.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	name := filepath.Base(filepath.FromSlash(fh.Filename))
	if name == "." || name == string(filepath.Separator) || name == "" {
		return fmt.Errorf("잘못된 파일 이름입니다")
	}
	dst, err := os.Create(filepath.Join(targetDir, name))
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

// safeJoin은 absDir 기준 상대 경로 rel을 안전하게 결합한다.
// ".." 등으로 absDir 밖을 벗어나려는 경로는 오류를 반환한다.
func safeJoin(absDir, rel string) (string, error) {
	cleanRel := filepath.Clean(strings.TrimPrefix(rel, "/"))
	joined := filepath.Join(absDir, cleanRel)
	if joined != absDir && !strings.HasPrefix(joined, absDir+string(filepath.Separator)) {
		return "", fmt.Errorf("경로가 공유 디렉터리를 벗어납니다")
	}
	return joined, nil
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
