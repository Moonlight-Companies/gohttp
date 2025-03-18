package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Moonlight-Companies/gologger/logger"
	"github.com/Moonlight-Companies/gompmc/mpmc"
)

type ClientID string

type SseMessage map[string]interface{}

func (m *SseMessage) Event() string {
	if event, ok := (*m)["event"].(string); ok {
		return event
	}
	return ""
}

func (m *SseMessage) Encode() ([]byte, error) {
	// Marshal the data into JSON.
	encoded_message, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}

	// Prepare the SSE format with proper prefixes and suffixes.
	sseFormattedMessage := fmt.Sprintf("data: %s\r\n\r\n", encoded_message)

	return []byte(sseFormattedMessage), nil
}

// EventHandler lets user provide interface such that state can be maintained,
// message filtering, and arbitrary callbacks can be handled per client.
type SseEventHandler interface {
	// OnInitialize is called when the http request is initialized.
	OnInitialize(w http.ResponseWriter, r *http.Request, server *SseServer, session *SseSession) error
	// OnConnect is called when a new session is created.
	OnConnect(w http.ResponseWriter, r *http.Request) error
	// OnDisconnect is called when a session is closed.
	OnDisconnect(w http.ResponseWriter, r *http.Request)
	// OnMessage is called before a message is sent for filtering.
	// Returning false skips sending the message.
	OnMessage(w http.ResponseWriter, r *http.Request, msg SseMessage) bool
	// OnCallback handles user-defined callbacks (e.g. via POST endpoints).
	OnCallback(w http.ResponseWriter, r *http.Request)
}

type SseEventHandlerFactory func() SseEventHandler

// SseSession represents an individual SSE client session.
type SseSession struct {
	client_id          ClientID
	user_handler       SseEventHandler
	done               chan struct{}
	broadcast_messages *mpmc.Consumer[SseMessage]
	direct_messages    chan SseMessage
	mu                 sync.Mutex
	closed             bool
}

func (s *SseSession) String() string {
	return "sse::" + string(s.client_id)
}

func (s *SseSession) ClientID() ClientID {
	return s.client_id
}

// DirectMessage attempts to queue a direct message non-blockingly.
func (s *SseSession) DirectMessage(msg SseMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("session closed")
	}
	select {
	case s.direct_messages <- msg:
		return nil
	default:
		return errors.New("direct message buffer full")
	}
}

// Close shuts down the session exactly once.
func (s *SseSession) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()

	close(s.done)
	s.broadcast_messages.Close()
	close(s.direct_messages)
}

// FnSseCallback is used for user session callbacks.
type FnSseCallback func(w http.ResponseWriter, r *http.Request, s *SseSession)

// SseServer holds the global fanout and active client sessions.
type SseServer struct {
	Logging *logger.Logger
	fanout  *mpmc.Producer[SseMessage]
	factory SseEventHandlerFactory
	clients map[ClientID]*SseSession
	mu      sync.RWMutex
}

func (s *SseServer) String() string {
	return "sse::server"
}

// Broadcast sends a message to all connected consumers.
func (s *SseServer) Broadcast(msg SseMessage) {
	s.fanout.Write(msg)
}

// Find retrieves a client session by client ID.
func (s *SseServer) Find(client_id ClientID) (*SseSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, exists := s.clients[client_id]
	return session, exists
}

// CloneClientList returns a copy of the client list. Can avoid mtx of range
func (s *SseServer) CloneClientList() map[ClientID]*SseSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	clones := make(map[ClientID]*SseSession, len(s.clients))
	for k, v := range s.clients {
		clones[k] = v
	}
	return clones
}

// Range iterates over all client sessions.
func (s *SseServer) Range(fn func(*SseSession) bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, session := range s.clients {
		if !fn(session) {
			break
		}
	}
}

// SetLoggingLevel sets the logging level for the server.
func (s *SseServer) SetLoggingLevel(level logger.LogLevel) *SseServer {
	s.Logging.SetLevel(level)
	return s
}

// Start creates the SSE server and registers its HTTP routes.
func (svc *Service) RegisterSSE(uri string, factory SseEventHandlerFactory) *SseServer {
	srv := &SseServer{
		fanout:  mpmc.NewProducer[SseMessage](mpmc.ProducerKind_All, 2048, 2048),
		Logging: logger.NewLogger("sse::" + uri),
		factory: factory,
		clients: make(map[ClientID]*SseSession),
	}

	callbacks := []string{
		uri + "/callback",
	}

	handleCallback := func(w http.ResponseWriter, r *http.Request) {
		var clientID ClientID = ""

		// Get the client ID from the request from X-Client-ID
		if clientIDHeader := r.Header.Get("X-Client-ID"); clientIDHeader != "" {
			clientID = ClientID(clientIDHeader)
		}

		// Get the client ID from the request from client_id (query string or body)
		if clientID == "" {
			clientIDParameters, ok := HttpParameterT[string](r, "client_id")
			if ok && clientIDParameters != "" {
				clientID = ClientID(clientIDParameters)
			}
		}

		// Get the client ID from the request from observer_id (query string or body)
		if clientID == "" {
			clientIDParameters, ok := HttpParameterT[string](r, "observer_id")
			if ok && clientIDParameters != "" {
				clientID = ClientID(clientIDParameters)
			}
		}

		if clientID == "" {
			WriteError(w, errors.New("missing client_id"))
			return
		}

		srv.mu.RLock()
		session, exists := srv.clients[clientID]
		srv.mu.RUnlock()
		if !exists {
			WriteError(w, errors.New("client not found"))
			return
		}

		if session.user_handler != nil {
			session.user_handler.OnCallback(w, r)
		}
	}

	// Register callback endpoints.
	for _, callbackURL := range callbacks {
		// capture the current URL
		url := callbackURL
		svc.RegisterRoutePOST(url, handleCallback)
	}

	// Register the main SSE route.
	// This route is used for both SSE and callback messages.
	svc.RegisterRoute(uri, "*", func(w http.ResponseWriter, r *http.Request) {
		acceptHeader := r.Header.Get("Accept")
		clientID := r.Header.Get("X-Client-ID")
		if !containsAcceptType(acceptHeader, "text/event-stream") && clientID != "" {
			handleCallback(w, r)
			return
		}

		// Set SSE headers.
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		rctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		broadcastConsumer := srv.fanout.CreateConsumer(rctx)
		client_id := ClientID(broadcastConsumer.Id())

		session := &SseSession{
			client_id:          client_id,
			done:               make(chan struct{}),
			direct_messages:    make(chan SseMessage, 256),
			broadcast_messages: broadcastConsumer,
		}
		srv.mu.Lock()
		srv.clients[client_id] = session
		srv.mu.Unlock()
		defer session.Close()

		// Initialize the user handler.
		if srv.factory != nil {
			uh := srv.factory()
			if uh != nil {
				session.user_handler = uh
				if err := uh.OnInitialize(w, r, srv, session); err != nil {
					WriteError(w, err)
					return
				}
			}
		}

		// On disconnect, call the disconnect callback and clean up.
		defer func() {
			srv.mu.Lock()
			delete(srv.clients, session.client_id)
			srv.mu.Unlock()

			if session.user_handler != nil {
				session.user_handler.OnDisconnect(w, r)
			}
		}()

		// Send the client ID to the client.
		session.DirectMessage(SseMessage{
			"event":       "on_connect",
			"observer_id": session.client_id,
			"client_id":   session.client_id,
		})

		// Call the connect callback.
		if session.user_handler != nil {
			if err := session.user_handler.OnConnect(w, r); err != nil {
				WriteError(w, err)
				return
			}
		}

		done := rctx.Done()

		pingInterval := 60 * time.Second
		pingTicker := time.NewTicker(pingInterval)
		defer pingTicker.Stop()

		for {
			select {
			// Broadcast messages.
			case msg, ok := <-session.broadcast_messages.Messages:
				if !ok {
					return
				}

				if session.user_handler != nil && !session.user_handler.OnMessage(w, r, msg) {
					continue
				}

				if encoded, err := msg.Encode(); err == nil {
					if _, err := w.Write(encoded); err != nil {
						return
					}
				} else {
					WriteError(w, err)
					return
				}

				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
					pingTicker.Reset(pingInterval)
				}
			// Direct messages.
			case directMsg, ok := <-session.direct_messages:
				if !ok {
					return
				}

				if session.user_handler != nil && !session.user_handler.OnMessage(w, r, directMsg) {
					continue
				}

				if encoded, err := directMsg.Encode(); err == nil {
					if _, err := w.Write(encoded); err != nil {
						return
					}
				} else {
					WriteError(w, err)
					return
				}

				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
					pingTicker.Reset(pingInterval)
				}
			// Ping messages.
			case <-pingTicker.C:
				pingMsg := SseMessage{
					"event":   "ping",
					"payload": time.Now().Unix(),
				}

				if encoded, err := pingMsg.Encode(); err == nil {
					if _, err := w.Write(encoded); err != nil {
						return
					}
				} else {
					WriteError(w, err)
					return
				}

				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
			case <-done:
				return
			}
		}
	})

	return srv
}
