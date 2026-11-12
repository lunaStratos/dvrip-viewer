package core

import "io"

// splitJPEG: 연속된 JPEG 바이트스트림에서 FFD8..FFD9 단위로 프레임을 추출.
// ffmpeg(image2pipe mjpeg) 의 stdout 을 읽어 한 장씩 onFrame 으로 넘긴다.
func splitJPEG(r io.Reader, onFrame func([]byte)) {
	buf := make([]byte, 0, 1<<20)
	tmp := make([]byte, 64*1024)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			for {
				s := indexMarker(buf, 0, 0xD8)
				if s < 0 {
					if len(buf) > 1 {
						buf = buf[len(buf)-1:]
					}
					break
				}
				if s > 0 {
					buf = buf[s:]
				}
				e := indexMarker(buf, 2, 0xD9)
				if e < 0 {
					break // 더 받아야 함
				}
				frame := make([]byte, e+2)
				copy(frame, buf[:e+2])
				onFrame(frame)
				buf = buf[e+2:]
			}
		}
		if err != nil {
			return
		}
	}
}

// indexMarker: buf[from:] 에서 0xFF <code> 위치(FF 의 인덱스)를 찾는다.
func indexMarker(buf []byte, from int, code byte) int {
	for i := from; i+1 < len(buf); i++ {
		if buf[i] == 0xFF && buf[i+1] == code {
			return i
		}
	}
	return -1
}
