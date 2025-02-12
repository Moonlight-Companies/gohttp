package service

import (
	"net/http"
	"strings"
)

const CONSTANT_SSE_JS = `
// inlined from project/service/constant.go
class SSEClient {
  constructor(endpoint) {
    if (!endpoint) {
      throw new Error('Endpoint must be provided')
    }
    this.endpoint = endpoint
    this.callbackEndpoint = endpoint + '/callback'
    this.eventSource = null
    this.connected = false
    this.client_id = null
    this.messageHandlers = []
    this.reconnectDelay = 3000
    this._connect()
  }

  _connect() {
    if (this.eventSource) {
      this.eventSource.close()
    }
    this.eventSource = new EventSource(this.endpoint)
    this.eventSource.onopen = (event) => {
      this.connected = true
    }
    this.eventSource.onmessage = (event) => {
      let msg = null
      try {
        msg = JSON.parse(event.data)
      } catch (err) {
        msg = { data: event.data }
      }
      // Automatically handle some events
      if (msg && msg.event) {
        switch (msg.event) {
          case 'on_connect':
            this.client_id = msg.client_id
            break
          case 'ping':
            this.publish({ event: 'pong', payload: msg.payload })
            break
        }
      }
      // Propagate message to user-registered handlers
      this.messageHandlers.forEach((handler) => {
        try {
          handler(msg)
        } catch (err) {
          console.error('Error in message handler', err)
        }
      })
    }
    this.eventSource.onerror = (error) => {
      this.connected = false
      console.error('SSE error:', error)
      this.eventSource.close()
      // Reconnect after a delay
      setTimeout(() => {
        this._connect()
      }, this.reconnectDelay)
    }
  }

  // Sends data to the server using the callback endpoint
  publish(data) {
    if (!this.client_id) {
      console.error('Client ID not set, cannot publish')
      return
    }
    fetch(this.callbackEndpoint, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Client-ID': this.client_id
      },
      body: JSON.stringify(data)
    }).catch((error) => {
      console.error('Publish error:', error)
    })
  }

  // register a message handler callback
  onMessage(callback) {
    if (typeof callback === 'function') {
      this.messageHandlers.push(callback)
    }
  }

  disconnect() {
    if (this.eventSource) {
      this.eventSource.close()
      this.eventSource = null
    }
    this.connected = false
  }
}

// Returns an instance of SSEClient when invoked with a relative endpoint
export default function createSSE(endpoint) {
  return new SSEClient(endpoint)
}

`

func (s *Service) static_constant(w http.ResponseWriter, r *http.Request) (bool, error) {
	if strings.HasSuffix(r.URL.Path, "/sse.js") {
		w.Header().Set("Content-Type", "application/javascript")
		w.Write([]byte(CONSTANT_SSE_JS))
		return true, nil
	}

	return false, nil
}
