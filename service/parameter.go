package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/Moonlight-Companies/goconvert/convert"

	"github.com/google/uuid"
)

type parameterKey string

const parameter_request_params = parameterKey("request_params")
const parameter_request_body = parameterKey("request_body")

func (s *Service) parameters(r *http.Request, params_uri map[string]string) (context.Context, error) {
	contentType := r.Header.Get("Content-Type")
	params := make(map[string]interface{})
	ctx := r.Context()

	// start with query parameters, these get clobbered by anything else
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}

	for k, v := range params_uri {
		if len(v) > 0 {
			params[k] = v
		}
	}

	if strings.HasPrefix(contentType, "application/json") {
		// Read and store raw body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return ctx, err
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		ctx = context.WithValue(ctx, parameter_request_body, body)

		// Unmarshal JSON into map[string]interface{}
		var j map[string]interface{}
		if err := json.Unmarshal(body, &j); err == nil {
			// Convert each value to string (using fmt.Sprintf)
			for k, v := range j {
				params[k] = v
			}
		} else {
			log.Println("Service::parameters: failed to unmarshal json", err)
		}
	} else if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") ||
		strings.HasPrefix(contentType, "multipart/form-data") {
		// Parse form parameters
		if err := r.ParseForm(); err != nil {
			return ctx, err
		}
		for k, v := range r.Form {
			if len(v) > 0 {
				params[k] = v[0]
			}
		}
	}

	// Always store the unified parameters
	ctx = context.WithValue(ctx, parameter_request_params, params)
	return ctx, nil
}

func HttpRemoteIP(r *http.Request) string {
	ip := r.Header.Get("X-Real-IP")
	if ip == "" {
		ip = r.Header.Get("X-Forwarded-For")
	}
	if ip == "" {
		ip = r.RemoteAddr
	}
	return ip
}

// HttpParameters retrieves the unified parameters from context
func HttpParameters(r *http.Request) map[string]interface{} {
	if params, ok := r.Context().Value(parameter_request_params).(map[string]interface{}); ok {
		return params
	}
	return map[string]interface{}{}
}

// HttpParameterInto decodes JSON from the raw request body into a given type T.
// Only works when the middleware stored a JSON body.
func HttpParameterInto[T any](r *http.Request) (result T, err error) {
	rawBody, ok := r.Context().Value(parameterKey(parameter_request_body)).([]byte)
	if !ok {
		return result, errors.New("no data found in request context")
	}
	decoder := json.NewDecoder(bytes.NewReader(rawBody))
	err = decoder.Decode(&result)
	return result, err
}

// HttpParameterIntoHash works like HttpParameterInto but also returns a checksum
// computed from the raw JSON body.
func HttpParameterIntoHash[T any](r *http.Request) (result T, ck uint64, err error) {
	rawBody, ok := r.Context().Value(parameter_request_body).([]byte)
	if !ok {
		return result, 0, errors.New("no data found in request context")
	}
	ck, err = Hash(rawBody)
	if err != nil {
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(rawBody))
	err = decoder.Decode(&result)
	return result, ck, err
}

// HttpParameterGeneric retrieves a parameter (as interface{}) by name.
// It first looks in the unified parameter map; if not found, it checks query parameters.
func HttpParameterGeneric(r *http.Request, name string) (interface{}, error) {
	params := HttpParameters(r)
	if val, ok := params[name]; ok {
		return val, nil
	}

	return nil, fmt.Errorf("parameter name not found: %s", name)
}

// HttpParameterT retrieves a parameter by name and converts it into type T.
func HttpParameterT[T any](r *http.Request, name string) (result T, ok bool) {
	value, err := HttpParameterGeneric(r, name)
	if err != nil {
		return result, false
	}
	return convert.ConvertInto[T](value)
}

// HttpParameterUUID retrieves a parameter by name, expects it as a string,
// and returns a parsed UUID.
func HttpParameterUUID(r *http.Request, name string) (uuid.UUID, error) {
	var output uuid.UUID
	if val, ok := HttpParameterT[string](r, name); ok {
		if u, err := uuid.Parse(val); err == nil {
			output = uuid.UUID(u)
			return output, nil
		}
		return output, fmt.Errorf("invalid UUID format: %s", val)
	}
	return output, fmt.Errorf("parameter name not found: %s", name)
}
