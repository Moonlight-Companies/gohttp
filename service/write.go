package service

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

func WriteT[T any](w http.ResponseWriter, msg T) error {
	encoded, err := json.Marshal(msg)
	if err != nil {
		log.Println("WriteT failed to marshal", "error", err)
		return err
	}

	return WriteRaw(w, "application/json", encoded)
}

// WriteRaw writes raw data (string or []byte) to an HTTP response writer.
// It's optimized for non-JSON data like plain text, HTML, or binary data.
//
// Parameters:
//   - w: The http.ResponseWriter to write the response to
//   - contentType: The MIME type of the content (e.g., "text/plain")
//   - data: The data to write (string or []byte)
//   - opts: Optional status code (default: 200 OK)
//
// Returns:
//   - error: Any error that occurred during writing
func WriteRaw[T ~string | ~[]byte](w http.ResponseWriter, contentType string, data T, opts ...int) error {
	// Set default status code if not provided
	statusCode := http.StatusOK
	if len(opts) > 0 {
		statusCode = opts[0]
	}

	// Convert data to []byte
	var responseData []byte
	switch v := any(data).(type) {
	case string:
		responseData = []byte(v)
	case []byte:
		responseData = v
	default:
		return fmt.Errorf("unsupported type: %T", data)
	}

	// Set headers before writing status and body
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(statusCode)

	// Write the response
	if _, err := w.Write(responseData); err != nil {
		return fmt.Errorf("failed to write response: %w", err)
	}

	return nil
}

func WriteError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(fmt.Sprintf(`{"error": "%s"}`, err.Error())))
}
