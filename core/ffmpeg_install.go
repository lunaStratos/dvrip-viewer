package core

// ffmpeg 자동 설치.
//
// ffmpeg 는 라이선스(GPL/LGPL) 때문에 앱에 동봉하지 않는다. 대신 필요할 때
// 사용자 동의를 받아 내려받아 앱 데이터 폴더에 둔다. 런타임 다운로드는
// 재배포가 아니므로 배포 의무가 생기지 않는다.
//
// 인터넷에서 받은 실행파일을 돌리는 기능이므로 다음을 지킨다:
//   - HTTPS 고정 버전 URL만 사용한다(롤링 URL 금지 — 내용이 바뀌면 검증이 무의미).
//   - 내려받은 바이트를 SHA-256 으로 검증하고, 불일치하면 폐기한다.
//   - 검증을 통과한 뒤에만 최종 위치로 원자적 이동(rename)한다.

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ffmpegSource 는 플랫폼별 자동 설치 원본이다.
type ffmpegSource struct {
	// 버전이 고정된 불변 URL.
	URL string
	// URL 이 가리키는 zip 전체의 SHA-256(소문자 16진).
	SHA256 string
	// 다운로드 크기(바이트). 진행률 표시 및 안내용.
	Size int64
	// zip 안에서 꺼낼 항목의 파일명.
	Entry string
	// 설치 후 저장할 파일명.
	Dest string
	// 사람이 읽는 출처 설명.
	Origin string
}

// gyan.dev 는 ffmpeg.org 가 안내하는 공식 윈도우 빌드 배포처다.
// essentials 빌드에 HEVC 디코딩·MJPEG 인코딩·하드웨어 가속이 모두 들어 있다.
var ffmpegWindowsAMD64 = ffmpegSource{
	URL:    "https://www.gyan.dev/ffmpeg/builds/packages/ffmpeg-9.0.2-essentials_build.zip",
	SHA256: "60f467265b1e312373dbcd92200c2618a74850f98d3d078e94296bb3fa2047ba",
	Size:   114768076,
	Entry:  "ffmpeg.exe",
	Dest:   "ffmpeg.exe",
	Origin: "gyan.dev (ffmpeg 9.0.2 essentials)",
}

// ffmpegSourceFor 는 현재 플랫폼의 자동 설치 원본을 돌려준다.
//
// 윈도우 x64 만 지원한다. macOS 는 arm64 정적 빌드의 신뢰할 수 있는 배포처가
// 없어 Homebrew 를 안내하고, Linux 는 배포판 패키지를 쓰는 편이 낫다.
func ffmpegSourceFor() (ffmpegSource, bool) {
	if runtime.GOOS == "windows" && runtime.GOARCH == "amd64" {
		return ffmpegWindowsAMD64, true
	}
	return ffmpegSource{}, false
}

// FFmpegInstallable 은 이 플랫폼에서 자동 설치를 지원하는지 알려준다.
func FFmpegInstallable() bool {
	_, ok := ffmpegSourceFor()
	return ok
}

// FFmpegInstallInfo 는 자동 설치 안내에 쓸 출처와 크기를 돌려준다.
func FFmpegInstallInfo() (origin string, size int64, ok bool) {
	src, ok := ffmpegSourceFor()
	return src.Origin, src.Size, ok
}

// FFmpegManualHint 는 자동 설치가 불가능한 플랫폼의 수동 설치 안내다.
func FFmpegManualHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "터미널에서: brew install ffmpeg"
	case "linux":
		return "배포판 패키지로: sudo apt install ffmpeg"
	default:
		return "https://ffmpeg.org/download.html 참고"
	}
}

// ffmpegInstallDir 은 자동 설치한 ffmpeg 를 두는 폴더다.
func ffmpegInstallDir() string {
	return filepath.Join(DataDir(), "bin")
}

// InstallFFmpeg 는 ffmpeg 를 내려받아 앱 데이터 폴더에 설치하고 그 경로를 돌려준다.
//
// progress 는 진행 상황을 알리는 콜백이며 nil 이어도 된다.
// 성공하면 FFmpegBin 이 설치된 경로로 갱신된다.
func InstallFFmpeg(progress func(phase string, done, total int64)) (string, error) {
	src, ok := ffmpegSourceFor()
	if !ok {
		return "", fmt.Errorf("이 플랫폼은 자동 설치를 지원하지 않습니다 — %s", FFmpegManualHint())
	}
	return installFrom(src, ffmpegInstallDir(), progress)
}

// installFrom 은 지정한 원본을 dir 에 설치한다. 테스트에서 가짜 원본을 넣을 수
// 있도록 InstallFFmpeg 와 분리해 둔다.
func installFrom(src ffmpegSource, dir string, progress func(phase string, done, total int64)) (string, error) {
	report := func(phase string, done, total int64) {
		if progress != nil {
			progress(phase, done, total)
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("설치 폴더 생성 실패: %w", err)
	}
	final := filepath.Join(dir, src.Dest)

	// 이미 설치돼 있으면 그대로 쓴다.
	if st, err := os.Stat(final); err == nil && !st.IsDir() && st.Size() > 0 {
		FFmpegBin = final
		return final, nil
	}

	// 1) 내려받으면서 동시에 해시를 계산한다(메모리에 다 담지 않는다).
	report("다운로드 중", 0, src.Size)
	tmpZip, err := os.CreateTemp(dir, "ffmpeg-*.zip.part")
	if err != nil {
		return "", fmt.Errorf("임시 파일 생성 실패: %w", err)
	}
	tmpName := tmpZip.Name()
	defer func() {
		tmpZip.Close()
		os.Remove(tmpName) // 성공 시엔 이미 지워져 있어 무해하다
	}()

	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Get(src.URL)
	if err != nil {
		return "", fmt.Errorf("다운로드 실패: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("다운로드 실패: 서버가 %s 응답", resp.Status)
	}

	total := src.Size
	if resp.ContentLength > 0 {
		total = resp.ContentLength
	}
	hasher := sha256.New()
	counter := &progressWriter{total: total, report: func(done int64) {
		report("다운로드 중", done, total)
	}}
	if _, err := io.Copy(tmpZip, io.TeeReader(resp.Body, io.MultiWriter(hasher, counter))); err != nil {
		return "", fmt.Errorf("다운로드 중 끊김: %w", err)
	}
	if err := tmpZip.Close(); err != nil {
		return "", fmt.Errorf("임시 파일 저장 실패: %w", err)
	}

	// 2) 무결성 검증. 불일치하면 쓰지 않고 버린다.
	report("검증 중", total, total)
	got := hex.EncodeToString(hasher.Sum(nil))
	if got != src.SHA256 {
		return "", fmt.Errorf(
			"무결성 검증 실패 — 내려받은 파일을 사용하지 않습니다\n기대: %s\n실제: %s",
			src.SHA256, got)
	}

	// 3) zip 에서 필요한 실행파일 하나만 꺼낸다.
	report("압축 해제 중", total, total)
	if err := extractZipEntry(tmpName, src.Entry, final); err != nil {
		return "", err
	}

	FFmpegBin = final
	report("완료", total, total)
	return final, nil
}

// extractZipEntry 는 zip 안에서 파일명이 entry 인 항목을 찾아 dest 에 쓴다.
// 임시 파일에 먼저 쓰고 rename 해서, 중간에 실패해도 깨진 파일이 남지 않게 한다.
func extractZipEntry(zipPath, entry, dest string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("zip 열기 실패: %w", err)
	}
	defer zr.Close()

	var found *zip.File
	for _, f := range zr.File {
		// zip 경로는 항상 '/' 구분자다. 디렉토리 구조에 의존하지 않도록
		// 파일명만 비교한다(배포판마다 상위 폴더 이름이 바뀐다).
		if path.Base(f.Name) == entry && !f.FileInfo().IsDir() {
			found = f
			break
		}
	}
	if found == nil {
		return fmt.Errorf("압축 파일에서 %s 를 찾지 못했습니다", entry)
	}

	rc, err := found.Open()
	if err != nil {
		return fmt.Errorf("%s 열기 실패: %w", entry, err)
	}
	defer rc.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".extract-*")
	if err != nil {
		return fmt.Errorf("임시 파일 생성 실패: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // rename 성공 후엔 없으므로 무해하다

	if _, err := io.Copy(tmp, rc); err != nil {
		tmp.Close()
		return fmt.Errorf("압축 해제 실패: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("압축 해제 저장 실패: %w", err)
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return fmt.Errorf("실행 권한 설정 실패: %w", err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("설치 위치로 이동 실패: %w", err)
	}
	return nil
}

// progressWriter 는 쓰인 바이트를 세어 진행률을 보고한다.
// 콜백이 너무 자주 불리지 않도록 일정 간격으로만 보고한다.
type progressWriter struct {
	done     int64
	total    int64
	lastSent time.Time
	report   func(done int64)
}

func (w *progressWriter) Write(p []byte) (int, error) {
	w.done += int64(len(p))
	if time.Since(w.lastSent) > 200*time.Millisecond {
		w.lastSent = time.Now()
		if w.report != nil {
			w.report(w.done)
		}
	}
	return len(p), nil
}

// HumanSize 는 바이트 수를 사람이 읽기 좋게 바꾼다.
func HumanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0fKB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// HaveFFmpeg 는 ffmpeg 를 지금 실행할 수 있는지 확인한다.
func HaveFFmpeg() bool {
	if FFmpegBin == "" {
		return false
	}
	// 경로가 들어 있으면 그 파일이 있는지, 이름뿐이면 PATH 에 있는지 본다.
	if strings.ContainsAny(FFmpegBin, `/\`) {
		st, err := os.Stat(FFmpegBin)
		return err == nil && !st.IsDir()
	}
	_, err := exec.LookPath(FFmpegBin)
	return err == nil
}
