package speech

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/rs/zerolog/log"
)

const (
	SampleRate  = 16_000 // 采样率
	Precision   = 2      // 每个样本的位深度
	NumChannels = 1      // 声道
)

func (s *Server) PcmToMp3(pcmFile, mp3File string) error {
	// 使用 ffmpeg 命令进行音频转换
	cmd := exec.Command("ffmpeg", "-f", "s16le", "-ar", "44100", "-ac", "2", "-i", pcmFile, "-codec:a", "libmp3lame", "-qscale:a", "2", mp3File)
	err := cmd.Run()
	if err != nil {
		log.Err(err).Msg("PcmToMp3 error")
	}
	return nil
}

func (s *Server) PcmToWav(pcm []byte) ([]byte, error) {
	cmd := exec.Command("ffmpeg", "-y", "-i", "pipe:0", "-f", "wav", "-acodec", "pcm_s16le", "pipe:1")
	cmd.Stdin = bytes.NewReader(pcm)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to convert pcm to wav: %w", err)
	}
	return out, nil
}

func (s *Server) WritePcmToMp3(buf *bytes.Buffer, filePath string) error {
	// 将 WAV 保存到文件中 ./player/example/audio/output_pcm.wav
	err := os.WriteFile(filePath, buf.Bytes(), 0644)
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) WritePcmToWav(buf *bytes.Buffer, filePath string) error {
	// 标准的wav是包含头信息的，所以对于纯pcm流，需要补足头部
	wavHeader, err := s.WAVHeaderForNormalizedPCM(buf.Bytes())
	if err != nil {
		return err
	}
	// 将 Pcm 数据转换成 精准参数的 WAV pcm 格式
	// wavBytes, err := s.PcmToWav(append(wavHeader, buf.Bytes()...))
	// if err != nil {
	// 	return err
	// }
	// 将 WAV 保存到文件中 ./player/example/audio/output_pcm.wav
	err = os.WriteFile(filePath, append(wavHeader, buf.Bytes()...), 0644)
	if err != nil {
		return err
	}
	return nil
}

type writeSeeker struct {
	pos  int64
	data []byte
}

func newWriteSeeker(cap int) *writeSeeker {
	return &writeSeeker{
		data: make([]byte, 0, cap),
	}
}

func (ws *writeSeeker) Write(p []byte) (int, error) {
	newPos := ws.pos + int64(len(p))
	if l := newPos - int64(len(ws.data)); l > 0 {
		ws.data = append(ws.data, make([]byte, l)...)
	}
	n := copy(ws.data[ws.pos:newPos], p)
	ws.pos = newPos
	return n, nil
}

func (ws *writeSeeker) Seek(offset int64, whence int) (int64, error) {
	var newPos int64
	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = ws.pos + offset
	case io.SeekEnd:
		newPos = int64(len(ws.data)) - offset
	}
	if newPos < 0 || newPos > int64(len(ws.data)) {
		return 0, errors.New("seek an invalid pos")
	}
	ws.pos = newPos
	return newPos, nil
}

type wavHeader struct {
	RiffMark      [4]byte
	FileSize      int32
	WaveMark      [4]byte
	FmtMark       [4]byte
	FormatSize    int32
	FormatType    int16
	NumChans      int16
	SampleRate    int32
	ByteRate      int32
	BytesPerFrame int16
	BitsPerSample int16
	DataMark      [4]byte
	DataSize      int32
}

// WAVHeaderForNormalizedPCM wav标准头部信息
func (s *Server) WAVHeaderForNormalizedPCM(withoutHeader []byte) ([]byte, error) {
	h := wavHeader{
		RiffMark:      [4]byte{'R', 'I', 'F', 'F'},
		FileSize:      int32(44 + len(withoutHeader)),
		WaveMark:      [4]byte{'W', 'A', 'V', 'E'},
		FmtMark:       [4]byte{'f', 'm', 't', ' '},
		FormatSize:    16,
		FormatType:    1,
		NumChans:      int16(NumChannels),
		SampleRate:    int32(SampleRate),
		ByteRate:      int32(SampleRate * NumChannels * Precision),
		BytesPerFrame: int16(NumChannels * Precision),
		BitsPerSample: int16(Precision) * 8,
		DataMark:      [4]byte{'d', 'a', 't', 'a'},
		DataSize:      int32(len(withoutHeader)),
	}
	ws := newWriteSeeker(44)
	err := binary.Write(ws, binary.LittleEndian, &h)
	if err != nil {
		return nil, err
	}
	return ws.data, nil
}

// StripWavHeader 去除标准wav格式文件的头信息，转为纯pcm音频二进制
func StripWavHeader(data []byte) (output []byte) {
	output = data
	if !bytes.HasPrefix(output, []byte("RIFF")) {
		return
	}
	if len(output) < 44 {
		return make([]byte, 0)
	}
	output = output[8:]
	if !bytes.HasPrefix(output, []byte("WAVE")) {
		return make([]byte, 0)
	}
	output = output[4:]
	for len(output) >= 8 {
		size := binary.LittleEndian.Uint32(output[4:8])
		if size > uint32(len(output)-8) {
			size = uint32(len(output) - 8)
		}
		if bytes.HasPrefix(output, []byte("data")) {
			return output[8 : size+8]
		}
		output = output[size+8:]
	}
	return
}
