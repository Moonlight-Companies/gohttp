package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Moonlight-Companies/gohttp/service"
)

// SseChatRoomClient implements SseEventHandler
type SseChatRoomClient struct {
	server   *service.SseServer
	session  *service.SseSession
	username string
}

// OnInitialize captures the server and assigns an IP-based username
func (seh *SseChatRoomClient) OnInitialize(w http.ResponseWriter, r *http.Request, server *service.SseServer, session *service.SseSession) error {
	seh.server = server
	seh.session = session
	seh.username = service.HttpRemoteIP(r) // Capture IP as username
	return nil
}

// OnConnect notifies all users about a new user joining
func (seh *SseChatRoomClient) OnConnect(w http.ResponseWriter, r *http.Request) error {
	joinMessage := service.SseMessage{
		"event":   "user_join",
		"user":    seh.username,
		"message": "has joined the chat.",
	}
	seh.server.Broadcast(joinMessage)
	return nil
}

// OnDisconnect notifies all users about a user leaving
func (seh *SseChatRoomClient) OnDisconnect(w http.ResponseWriter, r *http.Request) {
	leaveMessage := service.SseMessage{
		"event":   "user_leave",
		"user":    seh.username,
		"message": "has left the chat.",
	}
	seh.server.Broadcast(leaveMessage)
}

// OnMessage filters messages (allow all messages in this case)
func (seh *SseChatRoomClient) OnMessage(w http.ResponseWriter, r *http.Request, msg service.SseMessage) bool {
	return true // Allow all messages
}

// OnCallback handles messages sent from clients
func (seh *SseChatRoomClient) OnCallback(w http.ResponseWriter, r *http.Request) {
	var incomingMessage struct {
		Event   string `json:"event"`
		Message string `json:"message"`
	}

	if err := json.NewDecoder(r.Body).Decode(&incomingMessage); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if incomingMessage.Event != "chat_message" {
		// ping response or some other msg
		return
	}

	// Construct chat message and broadcast it
	if incomingMessage.Message == "" {
		http.Error(w, "Empty message", http.StatusBadRequest)
		return
	}

	chatMessage := service.SseMessage{
		"event":   "chat_message",
		"user":    seh.username,
		"message": incomingMessage.Message,
	}
	seh.server.Broadcast(chatMessage)

	w.WriteHeader(http.StatusOK)
}

var srv *service.Service = service.NewServiceWithName("project-test-service")
var sse *service.SseServer = nil

func main() {
	srv.Start()
	defer srv.Close()

	go func() {
		for {
			time.Sleep(5 * time.Second)

			if sse != nil {
				sse.Broadcast(service.SseMessage{
					"event":   "test",
					"message": "Hello, world!",
				})
			}
		}
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
	srv.Close()
}

func init() {
	srv.RegisterRoute("*/mul/:a/:b", "GET", func(w http.ResponseWriter, r *http.Request) {
		a, a_ok := service.HttpParameterT[float64](r, "a")
		b, b_ok := service.HttpParameterT[float64](r, "b")

		if !a_ok || !b_ok {
			service.WriteError(w, errors.New("missing parameters"))
			return
		}

		service.WriteT(w, map[string]interface{}{
			"a":      a,
			"b":      b,
			"result": a * b,
		})
	})

	srv.RegisterRoute("*/add", "GET", func(w http.ResponseWriter, r *http.Request) {
		a, a_ok := service.HttpParameterT[int](r, "a")
		b, b_ok := service.HttpParameterT[int](r, "b")

		if !a_ok || !b_ok {
			service.WriteError(w, errors.New("missing parameters"))
			return
		}

		service.WriteT(w, map[string]interface{}{
			"a":      a,
			"b":      b,
			"result": a + b,
		})
	})

	srv.RegisterRoute("*/foo/bar/test", "GET", func(w http.ResponseWriter, r *http.Request) {
		service.WriteRaw(w, "text/plain", "Foo, Bar!")
	})
}

func init() {
	sse = srv.RegisterSSE("*/events", func() service.SseEventHandler { return &SseChatRoomClient{} })
}
