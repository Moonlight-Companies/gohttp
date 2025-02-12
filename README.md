# gohttp Library

A Go library for building HTTP services with API routing, static file serving, load balancer registration, and integrated Server-Sent Events (SSE) support.

## Features

### Simple API Routing
- Easily register HTTP routes with pattern matching
- Support for named parameters in URI patterns (e.g., `/users/:id`)
- Use `service.HttpParameterT[T]` to convert user-provided parameters (e.g. converting "1" to 1 for numeric types)
- Use globbing from goconvert to make endpoint patterns (e.g., `*/users/:id`)
- `https://io.moonlightcompanies.com/service/project-test-service/users/userid123` -> `map[string]interface{}{"id":"userid123"}`

### Static File Serving
- Serves files from the `./static` directory (if it exists)
- In Docker builds, visiting `https://io.moonlightcompanies.com/service/project-test-service/` will serve `index.html`

### Load Balancer Registration
- Automatically registers the service at `/service/(service_name)/...` using the dynamic port
- Requires a valid `MOONLIGHT_TOKEN` environment variable and must run on an allowed internal VLAN

### Ultra-Simple Service Initialization
- Declare a top-level service variable (e.g. in `init()` functions) so that multiple modules can register their API endpoints

### Integrated SSE Support
- Implement the `SseEventHandler` interface for custom event handling
- Broadcast messages to all connected clients
- Handle user callback events
- Provides an internal `sse.js` endpoint for auto-reconnection and client-side event handling

## Requirements

- Go (version 1.24 or later)
- `MOONLIGHT_TOKEN` environment variable must be set for load balancer registration
- The service must run on an allowed internal VLAN for registration to succeed

## Installation

Clone the repository or add it to your Go module:

```bash
go get github.com/Moonlight-Companies/gohttp
```

## Usage

### Basic Web Server Example

This example shows how to create a simple web server with basic route handling, including named parameters:

```go
package main

import (
    "errors"
    "gohttp/service"
    "net/http"
)

var srv *service.Service = service.NewServiceWithName("project-test-service")

func main() {
    srv.Start()
    defer srv.Close()

    // Example with query parameters
    srv.RegisterRoute("*/add", "GET", func(w http.ResponseWriter, r *http.Request) {
        a, a_ok := service.HttpParameterT[int](r, "a")
        b, b_ok := service.HttpParameterT[int](r, "b")

        if !a_ok || !b_ok {
            service.WriteError(w, errors.New("missing parameters"))
            return
        }

        service.WriteT(w, map[string]interface{}{
            "result": a + b,
        })
    })

    // Example with named parameters in URI
    srv.RegisterRoute("*/mul/:a/:b", "GET", func(w http.ResponseWriter, r *http.Request) {
        a, a_ok := service.HttpParameterT[float64](r, "a")
        b, b_ok := service.HttpParameterT[float64](r, "b")

        if !a_ok || !b_ok {
            service.WriteError(w, errors.New("missing parameters"))
            return
        }

        service.WriteT(w, map[string]interface{}{
            "result": a * b,
        })
    })

    // Block until interrupted
    signals := make(chan os.Signal, 1)
    signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
    <-signals
}
```

### SSE Managing State

The following interface is provided for cases where the application requires state per connection, otherwise a nil builder
can be passed in

```go
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
```

With state we pass a builder
```go
sse := srv.RegisterSSE("*/events", func() service.SseEventHandler { 
    return &SseChatRoomClient{} 
})
```

And with no state, we just sse.Broadcast to all connected clients
```go
sse := srv.RegisterSSE("*/events", nil)
```


### SSE Chat Room Example

This example demonstrates implementing a chat room using Server-Sent Events (SSE):


```go
package main

import (
    "encoding/json"
    "gohttp/service"
    "net/http"
)

// SseChatRoomClient implements SseEventHandler
type SseChatRoomClient struct {
    server   *service.SseServer
    session  *service.SseSession
    username string
}

// Implement the SseEventHandler interface
func (seh *SseChatRoomClient) OnInitialize(w http.ResponseWriter, r *http.Request, 
    server *service.SseServer, session *service.SseSession) error {
    seh.server = server
    seh.session = session
    seh.username = service.HttpRemoteIP(r)
    return nil
}

func (seh *SseChatRoomClient) OnConnect(w http.ResponseWriter, r *http.Request) error {
    joinMessage := service.SseMessage{
        "event": "user_join",
        "user": seh.username,
        "message": "has joined the chat.",
    }
    seh.server.Broadcast(joinMessage)
    return nil
}

func (seh *SseChatRoomClient) OnDisconnect(w http.ResponseWriter, r *http.Request) {
    leaveMessage := service.SseMessage{
        "event": "user_leave",
        "user": seh.username,
        "message": "has left the chat.",
    }
    seh.server.Broadcast(leaveMessage)
}

func (seh *SseChatRoomClient) OnMessage(w http.ResponseWriter, r *http.Request, 
    msg service.SseMessage) bool {
    return true // Allow all messages
}

func (seh *SseChatRoomClient) OnCallback(w http.ResponseWriter, r *http.Request) {
    var incomingMessage struct {
        Event   string `json:"event"`
        Message string `json:"message"`
    }
    if err := json.NewDecoder(r.Body).Decode(&incomingMessage); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

    if incomingMessage.Event == "chat_message" && incomingMessage.Message != "" {
        chatMessage := service.SseMessage{
            "event":   "chat_message",
            "user":    seh.username,
            "message": incomingMessage.Message,
        }
        seh.server.Broadcast(chatMessage)
        w.WriteHeader(http.StatusOK)
    }
}

var srv *service.Service = service.NewServiceWithName("project-test-service")

func main() {
    // Register SSE endpoint with chat room handler
    sse := srv.RegisterSSE("*/events", func() service.SseEventHandler { 
        return &SseChatRoomClient{} 
    })

    srv.Start()
    defer srv.Close()

    // Block until interrupted
    signals := make(chan os.Signal, 1)
    signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
    <-signals
}
```

### SSE Client Example

The server provides an embedded `sse.js` file that handles auto reconnection. Use it in your client code:

```javascript
<script type="module">
  import createSSE from './sse.js'
  const client = createSSE('./events')
  
  client.onMessage(msg => {
    switch(msg.event) {
      case 'chat_message':
        console.log(`${msg.user}: ${msg.message}`)
        break
      case 'user_join':
      case 'user_leave':
        console.log(`${msg.user} ${msg.message}`)
        break
      default:
        console.log('Received event', msg)
    }
  })
  
  // Send a chat message
  function sendMessage(message) {
    client.publish({ 
      event: 'chat_message', 
      message: message 
    })
  }
</script>
```

## Running in Docker

1. Build your Docker container including your `./static` directory
2. When running with a dynamic port, use `--net host` to ensure the dynamic port is accessible for reverse proxying
3. Ensure the `MOONLIGHT_TOKEN` environment variable is set for load balancer registration

## Summary

- **API Routing**: Register endpoints with support for named parameters and type conversion
- **Static Serving & Load Balancer**: Serve static files and automatically register with the load balancer
- **SSE Integration**: Implement the `SseEventHandler` interface for real-time event handling
- **Modular Design**: Use a top-level service instance for module-level registration
- **Example Applications**: Includes a full chat room implementation demonstrating SSE capabilities

Use this library to quickly build robust Go web services with modern API features and real-time communication support.