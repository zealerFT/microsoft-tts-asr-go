package speech

type Server struct {
	SpeechKey    string // 密钥
	SpeechRegion string // 区域
}

func NewServer(options ...Option) *Server {
	s := &Server{}
	for _, option := range options {
		option(s)
	}
	return s
}

type Option func(*Server)

func KeyOption(speechKey string) Option {
	return func(s *Server) {
		s.SpeechKey = speechKey
	}
}

func RegionOption(speechRegion string) Option {
	return func(s *Server) {
		s.SpeechRegion = speechRegion
	}
}
