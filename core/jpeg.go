package core

import (
	"io"
	"os/exec"
)

// StreamJPEG 는 한 채널에 접속해 ffmpeg 로 JPEG 프레임을 뽑아
// 프레임마다 onFrame(jpegBytes) 를 호출한다.
// 네이티브 GUI 앱이 이 프레임을 그대로 디코딩해 화면에 그린다(웹/HTTP 불필요).
// stop 채널이 닫히면 정리하고 반환한다.
func StreamJPEG(stop <-chan struct{}, host string, port int, user, pass string, ch int, stream string, onFrame func([]byte)) error {
	pr, pw := io.Pipe()
	// 주의: -fflags nobuffer / -flags low_delay 는 GOP 중간부터 즉시 디코딩을
	// 시도하다 깨진 참조 프레임에서 멈춰 "첫 한 장만 나오고 정지"하는 원인이 된다.
	// 대신 +discardcorrupt 로 손상 NALU 만 버리고 정상적으로 버퍼링해 디코딩한다.
	ff := exec.Command(FFmpegBin, "-hide_banner", "-loglevel", "error",
		"-hwaccel", "auto", // GPU 하드웨어 디코드(Mac=VideoToolbox, Win=D3D11VA), 없으면 소프트웨어로 폴백
		"-fflags", "+discardcorrupt",
		"-analyzeduration", "500000", "-probesize", "500000", // 분석 버퍼 축소 → 첫 프레임 지연 단축
		"-f", "hevc", "-i", "-",
		"-r", "12", "-vf", "scale=640:-2", "-q:v", "6",
		"-f", "image2pipe", "-vcodec", "mjpeg", "-")
	ff.Stdin = pr
	out, err := ff.StdoutPipe()
	if err != nil {
		return err
	}
	if err := ff.Start(); err != nil {
		return err
	}

	// 카메라 → ffmpeg stdin
	go func() {
		_ = StreamChannel(host, port, user, pass, ch, stream, pw, nil)
		pw.Close()
	}()

	// stop 신호 → 정리
	go func() {
		<-stop
		if ff.Process != nil {
			ff.Process.Kill()
		}
		pw.Close()
		pr.Close()
	}()

	// ffmpeg stdout → JPEG 분리 → 콜백
	splitJPEG(out, onFrame)
	ff.Wait()
	return nil
}
