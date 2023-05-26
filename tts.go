package speech

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Microsoft/cognitive-services-speech-sdk-go/audio"
	"github.com/Microsoft/cognitive-services-speech-sdk-go/common"
	"github.com/Microsoft/cognitive-services-speech-sdk-go/speech"
	"github.com/rs/zerolog/log"
)

func synthesizeStartedHandler(event speech.SpeechSynthesisEventArgs) {
	defer event.Close()
	fmt.Println("Synthesis started.")
}

func synthesizingHandler(event speech.SpeechSynthesisEventArgs) {
	defer event.Close()
	fmt.Printf("Synthesizing, audio chunk size %d.\n", len(event.Result.AudioData))
}

func synthesizedHandler(event speech.SpeechSynthesisEventArgs) {
	defer event.Close()
	fmt.Printf("Synthesized, audio length %d.\n", len(event.Result.AudioData))
}

func cancelledHandler(event speech.SpeechSynthesisEventArgs) {
	defer event.Close()
	fmt.Println("Received a cancellation.")
}

func (s *Server) Tts(text string, voiceName string) (*bytes.Buffer, error) {
	audioBuffer := bytes.Buffer{}

	audioConfig, err := audio.NewAudioConfigFromDefaultSpeakerOutput()
	if err != nil {
		log.Fatal().Msgf("AudioConfig Got an error: %v", err)
		return &audioBuffer, err
	}
	defer audioConfig.Close()

	speechConfig, err := speech.NewSpeechConfigFromSubscription(s.SpeechKey, s.SpeechRegion)
	if err != nil {
		log.Fatal().Msgf("SpeechConfig Got an error: %v", err)
		return &audioBuffer, err
	}
	defer speechConfig.Close()

	// "zh-CN-XiaoyouNeural"
	err = speechConfig.SetSpeechSynthesisVoiceName(voiceName)
	if err != nil {
		log.Fatal().Msgf("SetSpeechSynthesisVoiceName Got an error: %v", err)
		return &audioBuffer, err
	}
	err = speechConfig.SetSpeechSynthesisOutputFormat(common.Riff16Khz16BitMonoPcm)
	if err != nil {
		log.Fatal().Msgf("SetSpeechSynthesisOutputFormat Got an error: %v", err)
		return &audioBuffer, err
	}

	speechSynthesizer, err := speech.NewSpeechSynthesizerFromConfig(speechConfig, audioConfig)
	if err != nil {
		log.Fatal().Msgf("SpeechSynthesizer Got an error: %v", err)
		return &audioBuffer, err
	}
	defer speechSynthesizer.Close()

	speechSynthesizer.SynthesisStarted(synthesizeStartedHandler)
	speechSynthesizer.Synthesizing(synthesizingHandler)
	speechSynthesizer.SynthesisCompleted(synthesizedHandler)
	speechSynthesizer.SynthesisCanceled(cancelledHandler)

	task := speechSynthesizer.SpeakTextAsync(text)
	var outcome speech.SpeechSynthesisOutcome

	select {
	case outcome = <-task:
	case <-time.After(60 * time.Second):
		log.Error().Msgf("tts timeout")
		return &audioBuffer, err
	}

	defer outcome.Close()

	if outcome.Error != nil {
		log.Err(outcome.Error).Msgf("Stream tts got an error!")
		return &audioBuffer, err
	}

	// in most case we want to streaming receive the audio to lower the latency,
	// we can use AudioDataStream to do so.
	stream, err := speech.NewAudioDataStreamFromSpeechSynthesisResult(outcome.Result)
	defer stream.Close()
	if err != nil {
		log.Err(err).Msgf("NewAudioDataStreamFromSpeechSynthesisResult stream tts got an error!")
		return &audioBuffer, err
	}

	// var allAudio []byte
	audioChunk := make([]byte, 2048)
	for {
		n, err := stream.Read(audioChunk)
		if err == io.EOF {
			break
		}
		audioBuffer.Write(audioChunk[:n])
	}

	return &audioBuffer, nil
}

type BinaryMessage struct {
	Err  error
	Data []byte
}

type TtsRequest struct {
	Start func() <-chan *BinaryMessage
	Close func()
}

type TtsService func() *TtsRequest

func (s *Server) TtsStream(text string, voiceName string) (TtsService, error) {
	audioConfig, err := audio.NewAudioConfigFromDefaultSpeakerOutput()
	if err != nil {
		log.Fatal().Msgf("AudioConfig Got an error: %v", err)
		return nil, err
	}
	defer audioConfig.Close()

	speechConfig, err := speech.NewSpeechConfigFromSubscription(s.SpeechKey, s.SpeechRegion)
	if err != nil {
		log.Fatal().Msgf("SpeechConfig Got an error: %v", err)
		return nil, err
	}
	defer speechConfig.Close()

	// "zh-CN-XiaoyouNeural"
	err = speechConfig.SetSpeechSynthesisVoiceName(voiceName)
	if err != nil {
		log.Fatal().Msgf("SetSpeechSynthesisVoiceName Got an error: %v", err)
		return nil, err
	}
	err = speechConfig.SetSpeechSynthesisOutputFormat(common.Riff16Khz16BitMonoPcm)
	if err != nil {
		log.Fatal().Msgf("SetSpeechSynthesisOutputFormat Got an error: %v", err)
		return nil, err
	}

	speechSynthesizer, err := speech.NewSpeechSynthesizerFromConfig(speechConfig, audioConfig)
	if err != nil {
		log.Fatal().Msgf("SpeechSynthesizer Got an error: %v", err)
		return nil, err
	}
	defer speechSynthesizer.Close()

	speechSynthesizer.SynthesisStarted(synthesizeStartedHandler)
	speechSynthesizer.Synthesizing(synthesizingHandler)
	speechSynthesizer.SynthesisCompleted(synthesizedHandler)
	speechSynthesizer.SynthesisCanceled(cancelledHandler)

	if len(text) == 0 {
		return nil, errors.New("text is null")
	}

	// StartSpeakingTextAsync sends the result to channel when the synthesis starts.
	task := speechSynthesizer.StartSpeakingTextAsync(text)
	var outcome speech.SpeechSynthesisOutcome
	return func() *TtsRequest {
		start := func() <-chan *BinaryMessage {
			ch := make(chan *BinaryMessage)
			go func() {
				select {
				case outcome = <-task:
				case <-time.After(60 * time.Second):
					log.Error().Msgf("tts timeout")
					ch <- &BinaryMessage{Err: errors.New("tts timeout")}
					return
				}

				defer outcome.Close()

				if outcome.Error != nil {
					log.Err(outcome.Error).Msgf("Stream tts got an error!")
					ch <- &BinaryMessage{Err: outcome.Error}
					return
				}

				// todo: 可以使用尝试使用synthesizingHandler来处理流
				stream, err := speech.NewAudioDataStreamFromSpeechSynthesisResult(outcome.Result)
				defer stream.Close()
				if err != nil {
					log.Err(err).Msgf("NewAudioDataStreamFromSpeechSynthesisResult stream tts got an error!")
					ch <- &BinaryMessage{Err: err}
					return
				}

				audioChunk := make([]byte, 2048)
				for {
					n, err := stream.Read(audioChunk)
					if err == io.EOF {
						ch <- &BinaryMessage{Err: err}
						break
					}
					ch <- &BinaryMessage{Data: audioChunk[:n]}
				}
			}()
			return ch
		}
		return &TtsRequest{Start: start}
	}, nil
}
