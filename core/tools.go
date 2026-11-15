package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// 해석된 외부 도구 경로(ffmpeg/ffplay). resolveTools() 로 설정.
var (
	FFmpegBin = "ffmpeg"
	FFplayBin = "ffplay"
)

// DataDir 은 앱 데이터 폴더다(접속정보·자동 설치한 ffmpeg 보관).
//
// Finder/탐색기에서 더블클릭으로 실행하면 작업 디렉토리가 "/" 가 되므로
// 상대 경로를 쓰면 안 된다. 항상 OS 가 정한 사용자 설정 폴더를 쓴다.
//   - macOS   ~/Library/Application Support/xmview
//   - Windows %AppData%\xmview
//   - Linux   ~/.config/xmview
func DataDir() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		return "."
	}
	d := filepath.Join(base, "xmview")
	_ = os.MkdirAll(d, 0o755)
	return d
}

// resolveTools 는 ffmpeg/ffplay 를 PATH → 실행파일과 같은 폴더 순으로 찾는다.
// 윈도우에서는 .exe 도 함께 찾는다. 없으면 기본 이름을 그대로 둔다(실행 시 오류 안내).
func ResolveTools() {
	FFmpegBin = findTool("ffmpeg")
	FFplayBin = findTool("ffplay")
}

func findTool(name string) string {
	var candidates []string

	// 1) 앱이 자동 설치한 것을 가장 먼저 쓴다(검증을 거친 알려진 버전).
	//    PATH 보다 앞에 두어, 설치 이후에는 매번 같은 바이너리를 쓰게 한다.
	installed := filepath.Join(DataDir(), "bin", name)
	if runtime.GOOS == "windows" {
		installed += ".exe"
	}
	if st, err := os.Stat(installed); err == nil && !st.IsDir() {
		return installed
	}

	// 2) PATH
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	// 3) 실행파일과 같은 폴더(동봉한 경우)
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(dir, name))
		if runtime.GOOS == "windows" {
			candidates = append(candidates, filepath.Join(dir, name+".exe"))
		}
	}
	// 4) OS 공통 설치 경로 (Finder/더블클릭 실행 시 PATH 가 비는 문제 대비)
	switch runtime.GOOS {
	case "darwin":
		candidates = append(candidates,
			"/opt/homebrew/bin/"+name, // Apple Silicon Homebrew
			"/usr/local/bin/"+name,    // Intel Homebrew
			"/opt/local/bin/"+name)    // MacPorts
	case "linux":
		candidates = append(candidates, "/usr/bin/"+name, "/usr/local/bin/"+name)
	case "windows":
		candidates = append(candidates,
			`C:\ffmpeg\bin\`+name+".exe",
			`C:\Program Files\ffmpeg\bin\`+name+".exe")
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return name // 못 찾으면 기본 이름(실행 시 에러로 안내)
}

// ffmpeg 존재 여부 확인(웹/CLI 시작 시 호출). 없으면 설치 안내 후 false.
func CheckFFmpeg() bool {
	if _, err := exec.LookPath(FFmpegBin); err == nil {
		return true
	}
	if st, err := os.Stat(FFmpegBin); err == nil && !st.IsDir() {
		return true
	}
	fmt.Fprintln(os.Stderr, "──────────────────────────────────────────────")
	fmt.Fprintln(os.Stderr, " ffmpeg 를 찾을 수 없습니다. 영상 재생에 필요합니다.")
	switch runtime.GOOS {
	case "windows":
		fmt.Fprintln(os.Stderr, " · 설치: https://www.gyan.dev/ffmpeg/builds/ 의 full 빌드")
		fmt.Fprintln(os.Stderr, "   또는 ffmpeg.exe 를 xmview.exe 와 같은 폴더에 두세요.")
	case "darwin":
		fmt.Fprintln(os.Stderr, " · 설치: brew install ffmpeg")
	default:
		fmt.Fprintln(os.Stderr, " · 설치: sudo apt install ffmpeg  (또는 배포판 패키지)")
	}
	fmt.Fprintln(os.Stderr, "──────────────────────────────────────────────")
	return false
}
