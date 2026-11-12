# XMView — XM(DVRIP) CCTV 뷰어 (단독 네이티브 앱)

Xiongmai(XM) 계열 DVR/NVR/IPC(미라클·XMEye·이지피스 등)에 **DVRIP 프로토콜**로 직접
접속해 실시간 영상을 보는 **단독 데스크톱 앱**. 웹·브라우저·cmd 창 없이
**창 하나**가 뜨고 그 안에서 로그인 → 저장 → **2×2 분할 4채널**을 본다.
**Windows / macOS(Intel·Apple Silicon)** 지원. 영상 디코딩에만 `ffmpeg`를 쓴다.

화면 흐름: **로그인 폼 → 저장 → 2×2 분할 영상**(전부 창 하나 안, 네이티브 위젯).

## GPU 하드웨어 가속
무거운 작업은 **HEVC(H.265) 디코딩**이며, ffmpeg 하드웨어 디코더로 **GPU에서 처리**한다
(`-hwaccel auto` → macOS **VideoToolbox**, Windows **D3D11VA/DXVA2**, 없으면 소프트웨어 폴백).
그 위에 **OpenGL(Fyne)** 로 렌더링한다. 화면 프레임은 640px 로 축소되므로 4채널 동시도 가볍다.

## 요구사항
- **ffmpeg** (영상 디코딩용)
  - macOS: `brew install ffmpeg`
  - Windows: [gyan.dev full 빌드](https://www.gyan.dev/ffmpeg/builds/)를 PATH 에 넣거나 `ffmpeg.exe`를 실행파일과 같은 폴더에 둠
- 빌드 시: **Go 1.21+**, C 컴파일러(CGO, OpenGL 필요)

## 빌드 / 실행

### macOS
```bash
./build.sh          # dist/XMView-Native.app 생성(아이콘 포함)
```
생성된 `dist/XMView-Native.app` 더블클릭으로 실행.
접속정보 저장 위치: `~/Library/Application Support/xmview/connections.json`

### Windows
윈도우 PC에서 빌드(CGO + OpenGL 필요):
```bat
: Go 와 TDM-GCC(또는 MSYS2 mingw) 설치 후
set CGO_ENABLED=1
go mod tidy
go build -ldflags "-H=windowsgui -s -w" -o xmview.exe .
```
접속정보 저장 위치: `%AppData%\xmview\connections.json`
ffmpeg.exe 는 PATH 또는 실행파일과 같은 폴더에 둔다.

## 사용법
1. 앱 실행 → 좌측 폼에 **주소 / 포트 / 아이디 / 비밀번호 / 채널** 입력
2. **저장** 으로 접속정보 보관(다음엔 "저장목록"에서 선택)
3. **연결** → 좌측 바가 자동으로 접히고 **2×2 분할 4채널** 재생
4. 스트림: `Main`(고화질) / `Extra1`(저지연) 선택

## 프로젝트 구조
```
main.go            네이티브 GUI 앱(Fyne) — 로그인 폼 + 2×2 그리드
icon.png           앱 아이콘(빌드 시 .icns 로 변환)
build.sh           macOS .app 빌드 스크립트
core/
  protocol.go      DVRIP 프로토콜(로그인·KeepAlive·모니터·프레임 디먹스)
  jpeg.go          채널 → ffmpeg(HEVC→MJPEG) → JPEG 프레임 콜백
  jpeg_split.go    JPEG 바이트스트림 분리(FFD8..FFD9)
  tools.go         ffmpeg 경로 탐색
```

## 동작 원리(요약)
1. TCP 접속 → DVRIP 20바이트 헤더 + JSON 본문.
2. 로그인(msg 1000): 비밀번호는 XM "Sofia" MD5 해시(8자). 응답에서 SessionID 획득.
3. 실시간 요청: OPMonitor **Claim(msg 1413)** → **Start(msg 1410)**.
4. 영상 데이터(msg 1412)에서 XM 프레임 헤더(`00 00 01 FC/FD/...`)를 떼고
   H.265(HEVC) Annex-B 만 추출 → ffmpeg 로 디코딩(GPU) → JPEG 프레임으로 화면에 렌더.
5. KeepAlive(msg 1006) 를 AliveInterval 주기로 전송.
