package core

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeZip 은 상위 폴더가 있는 zip 을 만든다(실제 배포판 구조와 같게).
func makeZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip 항목 생성 실패: %v", err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("zip 항목 쓰기 실패: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip 마감 실패: %v", err)
	}
	return buf.Bytes()
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// serveBytes 는 주어진 바이트를 내려주는 임시 HTTP 서버를 띄운다.
func serveBytes(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestInstallExtractsEntryWhenChecksumMatches(t *testing.T) {
	zipBytes := makeZip(t, map[string]string{
		"ffmpeg-9.0.2-essentials_build/LICENSE":        "license text",
		"ffmpeg-9.0.2-essentials_build/bin/ffmpeg.exe": "FAKE-FFMPEG-BINARY",
		"ffmpeg-9.0.2-essentials_build/bin/ffplay.exe": "other",
	})
	srv := serveBytes(t, zipBytes)
	dir := t.TempDir()

	src := ffmpegSource{
		URL:    srv.URL + "/pkg.zip",
		SHA256: sha256Hex(zipBytes),
		Size:   int64(len(zipBytes)),
		Entry:  "ffmpeg.exe",
		Dest:   "ffmpeg.exe",
	}

	var phases []string
	got, err := installFrom(src, dir, func(phase string, done, total int64) {
		if len(phases) == 0 || phases[len(phases)-1] != phase {
			phases = append(phases, phase)
		}
	})
	if err != nil {
		t.Fatalf("설치 실패: %v", err)
	}

	want := filepath.Join(dir, "ffmpeg.exe")
	if got != want {
		t.Errorf("경로 = %q, 기대 %q", got, want)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("설치된 파일을 읽을 수 없음: %v", err)
	}
	// 같은 이름의 다른 항목(ffplay)이 아니라 정확히 ffmpeg 를 꺼내야 한다.
	if string(data) != "FAKE-FFMPEG-BINARY" {
		t.Errorf("내용 = %q", data)
	}
	// 실행 권한이 있어야 한다.
	st, err := os.Stat(got)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm()&0o111 == 0 {
		t.Errorf("실행 권한 없음: %v", st.Mode())
	}
	if len(phases) == 0 {
		t.Error("진행 상황 콜백이 호출되지 않음")
	}
}

// 체크섬이 다르면 파일을 설치하지 않아야 한다(핵심 보안 장치).
func TestInstallRejectsChecksumMismatch(t *testing.T) {
	zipBytes := makeZip(t, map[string]string{
		"pkg/bin/ffmpeg.exe": "TAMPERED",
	})
	srv := serveBytes(t, zipBytes)
	dir := t.TempDir()

	src := ffmpegSource{
		URL: srv.URL + "/pkg.zip",
		// 의도적으로 틀린 해시
		SHA256: strings.Repeat("ab", 32),
		Size:   int64(len(zipBytes)),
		Entry:  "ffmpeg.exe",
		Dest:   "ffmpeg.exe",
	}

	if _, err := installFrom(src, dir, nil); err == nil {
		t.Fatal("체크섬 불일치인데도 성공했다")
	} else if !strings.Contains(err.Error(), "무결성 검증 실패") {
		t.Errorf("예상과 다른 오류: %v", err)
	}

	// 설치 파일이 생기지 않아야 하고, 임시 파일도 남지 않아야 한다.
	if _, err := os.Stat(filepath.Join(dir, "ffmpeg.exe")); !os.IsNotExist(err) {
		t.Error("검증 실패인데 파일이 설치됐다")
	}
	left, _ := os.ReadDir(dir)
	for _, e := range left {
		t.Errorf("임시 파일이 남았다: %s", e.Name())
	}
}

func TestInstallReusesExistingBinary(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "ffmpeg.exe")
	if err := os.WriteFile(existing, []byte("ALREADY-HERE"), 0o755); err != nil {
		t.Fatal(err)
	}
	// URL 이 가짜여도 이미 설치돼 있으면 네트워크를 쓰지 않아야 한다.
	src := ffmpegSource{
		URL:    "http://127.0.0.1:1/should-not-be-called",
		SHA256: "unused",
		Entry:  "ffmpeg.exe",
		Dest:   "ffmpeg.exe",
	}
	got, err := installFrom(src, dir, nil)
	if err != nil {
		t.Fatalf("기존 설치를 재사용하지 못했다: %v", err)
	}
	if got != existing {
		t.Errorf("경로 = %q, 기대 %q", got, existing)
	}
}

func TestInstallFailsWhenEntryMissing(t *testing.T) {
	zipBytes := makeZip(t, map[string]string{"pkg/README.txt": "no binary here"})
	srv := serveBytes(t, zipBytes)
	dir := t.TempDir()

	src := ffmpegSource{
		URL:    srv.URL + "/pkg.zip",
		SHA256: sha256Hex(zipBytes),
		Entry:  "ffmpeg.exe",
		Dest:   "ffmpeg.exe",
	}
	_, err := installFrom(src, dir, nil)
	if err == nil || !strings.Contains(err.Error(), "찾지 못했습니다") {
		t.Errorf("예상과 다른 오류: %v", err)
	}
}

func TestInstallFailsOnHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	src := ffmpegSource{URL: srv.URL, SHA256: "x", Entry: "ffmpeg.exe", Dest: "ffmpeg.exe"}
	if _, err := installFrom(src, t.TempDir(), nil); err == nil {
		t.Fatal("404 인데도 성공했다")
	}
}

// 고정한 다운로드 원본이 검증 가능한 상태인지 확인한다.
// (롤링 URL + 고정 해시 조합은 언젠가 반드시 깨지므로 금지)
func TestPinnedSourceIsImmutableAndHashed(t *testing.T) {
	src := ffmpegWindowsAMD64
	if !strings.HasPrefix(src.URL, "https://") {
		t.Error("HTTPS 가 아니다")
	}
	if strings.Contains(src.URL, "release-essentials") || strings.Contains(src.URL, "latest") {
		t.Errorf("롤링 URL 을 쓰면 고정 해시가 깨진다: %s", src.URL)
	}
	if len(src.SHA256) != 64 {
		t.Errorf("SHA-256 길이가 %d", len(src.SHA256))
	}
	if _, err := hex.DecodeString(src.SHA256); err != nil {
		t.Errorf("SHA-256 이 16진수가 아니다: %v", err)
	}
	if src.Size <= 0 {
		t.Error("크기 정보가 없다")
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{
		512:         "512B",
		2048:        "2KB",
		114768076:   "109MB",
		3 * 1 << 30: "3.0GB",
	}
	for in, want := range cases {
		if got := HumanSize(in); got != want {
			t.Errorf("HumanSize(%d) = %q, 기대 %q", in, got, want)
		}
	}
}
