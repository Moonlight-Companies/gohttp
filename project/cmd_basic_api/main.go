package main

import (
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/Moonlight-Companies/gohttp/service"
)

var srv *service.Service = service.NewServiceWithName("project-test-service")

func main() {
	srv.Start()
	defer srv.Close()

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
