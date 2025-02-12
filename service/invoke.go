package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

func getEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("Missing environment variable %s", key)
	}
	return value
}

var Token = getEnv("MOONLIGHT_TOKEN")

func Invoke[T any](Call string, Parameters map[string]interface{}) (results T, body []byte, err error) {
	return InvokeTimeout[T](Call, Parameters, 30*time.Second)
}

func InvokeTimeout[T any](Call string, Parameters map[string]interface{}, timeout time.Duration) (results T, body []byte, err error) {
	Parameters["Token"] = Token

	j, err := json.Marshal(Parameters)
	if err != nil {
		return
	}
	u := bytes.NewReader(j)

	method := "POST"
	request, err := http.NewRequest(method, "https://io.moonlightcompanies.com/"+Call, u)
	if err != nil {
		return
	}

	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	// Create a context with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Assign the context to the HTTP request
	request = request.WithContext(ctx)

	//log.Println("Waiting INVOKE", Call, Parameters)
	ta := time.Now()
	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return
	}
	defer response.Body.Close()
	if time.Since(ta) > 1*time.Second {
		log.Println("Waiting INVOKE DONE", Call, time.Since(ta))
	}

	if response.StatusCode != 200 {
		err = fmt.Errorf("received non-200 status code: %d from: %s", response.StatusCode, Call)
		return
	}

	body, err = io.ReadAll(response.Body)
	if err != nil {
		return
	}

	err = json.Unmarshal(body, &results)

	return
}
