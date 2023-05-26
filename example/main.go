package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"
	"github.com/rs/zerolog/log"
	speech "github.com/zealerFT/microsoft-tts-asr-go"
)

type Event struct {
	// Events are pushed to this channel by the main events-gathering routine
	Message chan string

	// New client connections
	NewClients chan chan string

	// Closed client connections
	ClosedClients chan chan string

	// Total client connections
	TotalClients map[chan string]bool
}

// ClientChan New event messages are broadcast to all registered client connection channels
type ClientChan chan string

func TtsEventStream(c *gin.Context) {
	var body struct {
		Text         string `json:"text"`
		SpeechKey    string `json:"speech_key"`
		SpeechRegion string `json:"speech_region"`
		VoiceName    string `json:"voice_name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		log.Err(err).Msgf("malformed request")
		return
	}

	server := speech.NewServer(speech.KeyOption(body.SpeechKey), speech.RegionOption(body.SpeechRegion))
	tts, err := server.TtsStream(body.Text, body.VoiceName)
	if err != nil {
		log.Err(err).Msgf("stream tts begin error~")
		return
	}

	ttsService := tts()
	ch := ttsService.Start()

	defer func() {
		fmt.Println("tts service close!")
	}()

	v, ok := c.Get("clientChan")
	if !ok {
		return
	}
	clientChan, ok := v.(ClientChan)
	if !ok {
		return
	}
	c.Stream(func(w io.Writer) bool {
		// Stream message to client from message channel
		go func() {
			if msg, ok := <-clientChan; ok {
				c.SSEvent("message", msg)
			}
		}()

		for {
			select {
			case res := <-ch:
				if res.Err != nil {
					if res.Err == io.EOF {
						c.SSEvent("byte", "io.EOF")
						log.Info().Msgf("stream tts is normal end~")
					} else {
						log.Err(res.Err).Msgf("stream tts is not normal end~")
					}
					return false
				}

				c.SSEvent("byte", res.Data)
			default:
			}
		}
	})
}

func Tts(c *gin.Context) {
	var body struct {
		Text         string `json:"text"`
		SpeechKey    string `json:"speech_key"`
		SpeechRegion string `json:"speech_region"`
		VoiceName    string `json:"voice_name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.AbortWithStatusJSON(400, gin.H{"message": "malformed request"})
		return
	}

	server := speech.NewServer(speech.KeyOption(body.SpeechKey), speech.RegionOption(body.SpeechRegion))
	bytes, err := server.Tts(body.Text, body.VoiceName)
	if err != nil {
		c.AbortWithStatusJSON(500, gin.H{"message": err.Error()})
		return
	}

	err = server.WritePcmToWav(bytes, "./"+strconv.FormatInt(time.Now().Unix(), 10)+".wav")
	if err != nil {
		c.AbortWithStatusJSON(500, gin.H{"message": err.Error()})
		return
	}
	c.AbortWithStatusJSON(200, gin.H{})
}

func main() {
	flag.Parse()
	_ = flag.Set("logtostderr", "true")
	r := gin.Default()
	r.Use(
		func(c *gin.Context) {
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
			c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")
			if c.Request.Method == "OPTIONS" {
				c.AbortWithStatus(204)
			}
		},
	)

	// Initialize new streaming server
	stream := NewServer()

	// We are streaming current time to clients in the interval 10 seconds
	go func() {
		for {
			time.Sleep(time.Second * 10)
			now := time.Now().Format("2006-01-02 15:04:05")
			currentTime := fmt.Sprintf("The Current Time Is %v", now)

			// Send current time to clients message channel
			stream.Message <- currentTime
		}
	}()

	// Basic Authentication
	// authorized := r.Group("/", gin.BasicAuth(gin.Accounts{
	// 	"admin": "admin123", // username : admin, password : admin123
	// }))

	// Authorized client can stream the event
	// Add event-streaming headers
	r.POST("/tts_stream", HeadersMiddleware(), stream.serveHTTP(), TtsEventStream)
	r.POST("/tts", Tts)
	server := &http.Server{Addr: ":8080", Handler: r}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			glog.Fatalf("server failure: %v", err)
		}
	}()

	log.Log().Msg("Ws is begining :)")

	termination := make(chan os.Signal)
	signal.Notify(termination, syscall.SIGINT, syscall.SIGTERM)
	<-termination

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		glog.Fatalf("Failed to shut down: %v", err)
	}

	log.Log().Msg("Ws shutting down :)")

}

// NewServer Initialize event and Start procnteessing requests
func NewServer() (event *Event) {
	event = &Event{
		Message:       make(chan string),
		NewClients:    make(chan chan string),
		ClosedClients: make(chan chan string),
		TotalClients:  make(map[chan string]bool),
	}

	go event.listen()

	return
}

// It Listens all incoming requests from clients.
// Handles addition and removal of clients and broadcast messages to clients.
func (stream *Event) listen() {
	for {
		select {
		// Add new available client
		case client := <-stream.NewClients:
			stream.TotalClients[client] = true
			log.Printf("Client added. %d registered clients", len(stream.TotalClients))

		// Remove closed client
		case client := <-stream.ClosedClients:
			delete(stream.TotalClients, client)
			close(client)
			log.Printf("Removed client. %d registered clients", len(stream.TotalClients))

		// Broadcast message to client
		case eventMsg := <-stream.Message:
			for clientMessageChan := range stream.TotalClients {
				clientMessageChan <- eventMsg
			}
		}
	}
}

func (stream *Event) serveHTTP() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Initialize client channel
		clientChan := make(ClientChan)

		// Send new connection to event server
		stream.NewClients <- clientChan

		defer func() {
			// Send closed connection to event server
			stream.ClosedClients <- clientChan
		}()

		c.Set("clientChan", clientChan)

		c.Next()
	}
}

func HeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("Transfer-Encoding", "chunked")
		c.Next()
	}
}
