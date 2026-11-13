# transfer-file을 위한 Copilot 지침

## 언어 정책

- 이 저장소의 모든 문서(README 등)와 코드 주석은 **한글**로 작성한다.
- Copilot CLI 세션 응답도 **한글**로 작성한다.

## 프로젝트 개요

LAN에서 PC 간 파일을 임시로 주고받기 위한 단일 바이너리 WebDAV 서버
(Go, `golang.org/x/net/webdav`)입니다. 인증과 TLS가 없는 것은 설계
의도이며, 실수가 아닙니다. 모든 로직은 `main.go` 한 파일에 있으며
별도의 패키지 구조는 없습니다.

## 빌드 및 실행

```powershell
go build -o transfer-file.exe .
./transfer-file.exe -dir ./shared -port 8081 [-readonly]
```

- `go vet ./...` — 정적 분석 (아직 테스트는 없음. 테스트를 추가하면
  `go test ./... -run TestName`으로 개별 테스트 실행 가능)
- 커밋 전 `go fmt ./...` 실행
- Go 1.21+ 필요 (`golang.org/x/net/webdav`는 `go.mod`로 관리)

## 아키텍처

- `main()`은 네 부분을 연결한다: `-dir`을 DAV 루트로 서비스하는
  `webdav.Handler`(`webdav.Dir` 사용), 디렉터리 요청을 가로채
  목록/업로드 폼을 보여주는 `withDirListing`, 선택적 읽기 전용 래퍼
  (`withReadOnly`), 그리고 다른 PC에서 접속할 주소를 알려주기 위해
  루프백이 아닌 모든 IPv4 주소를 출력하는 시작 로그(`lanURLs`).
- `withDirListing`은 GET/HEAD 요청 대상이 디렉터리면(webdav가 405를
  반환하는 경우) `renderDirListing`으로 자체 HTML 목록 + 업로드 폼을
  그리고, 파일이면 그대로 webdav 핸들러로 위임한다.
- `/upload`는 별도 라우트(`uploadHandler`)로, multipart 폼을 받아
  `safeJoin`으로 경로 탈출을 막은 뒤 `saveUploadedFile`로 저장하고
  업로드 전 폴더로 303 리다이렉트한다. `-readonly`일 때는 항상 거부.
- `withReadOnly`는 WebDAV 쓰기 메서드(`PUT`, `DELETE`, `MKCOL`,
  `COPY`, `MOVE`, `PROPPATCH`)를 실제 핸들러로 넘기기 전에 차단하는
  `http.Handler` 미들웨어이며, 파일 시스템 계층은 건드리지 않는다.
- 공유 디렉터리는 없으면 `os.MkdirAll`로 생성하고, `webdav.Dir`에
  넘기기 전에 절대 경로로 변환한다.

## 코딩 컨벤션

- 꼭 필요한 경우가 아니면 `main.go` 한 파일로 유지한다 — 이 프로젝트는
  의도적으로 내부 패키지 분리를 피한다.
- 플래그만 사용한다(`-dir`, `-port`, `-readonly`); 요청이 없는 한
  별도의 설정 파일 형식을 추가하지 않는다.
- 쓰기를 제한하는 새로운 동작은 별도 핸들러가 아니라
  `withReadOnly`의 `writeMethods` 집합(및 `uploadHandler`의
  `readOnly` 체크)에 추가한다.
- 사용자가 지정한 상대 경로(업로드 대상 폴더 등)를 파일 시스템 경로와
  결합할 때는 반드시 `safeJoin`을 거쳐 경로 탈출을 막는다.
- 기본 데이터 디렉터리 `./shared/`와 빌드된 바이너리는 gitignore
  대상이다 — 전송된 파일이나 `transfer-file.exe`를 커밋하지 않는다.
