cd $(dirname "$0")
CGO_CFLAGS="-I/绝对路径/microsoft-tts-asr-go/microsoft.cognitiveservices.speech.1.28.0/build/native/include/c_api"  \
CGO_LDFLAGS="-L. -lMicrosoft.CognitiveServices.Speech.core"  \
CGO_ENABLED=1 \
GOARCH=amd64 \
go build -ldflags "-s -w -r=@executable_path" -o fermi