package service

import (
	"net"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Moonlight-Companies/goconvert/glob"
	"github.com/Moonlight-Companies/gologger/logger"
)

type ServiceHandleFunc func(http.ResponseWriter, *http.Request)

type serviceHttpRouteInfo struct {
	URI    string
	Method string
	Fn     ServiceHandleFunc
	Hits   int32
	Logger *logger.Logger
}

func NewServiceHttpRouteInfo(uri, method string, fn ServiceHandleFunc) *serviceHttpRouteInfo {
	info := &serviceHttpRouteInfo{
		URI:    uri,
		Method: method,
		Fn:     fn,
		Hits:   0,
		Logger: logger.NewLogger(logger.LogLevelDebug, uri),
	}

	return info
}

func (s *serviceHttpRouteInfo) MatchURL(r *http.Request) (matched bool, named_parameters map[string]string) {
	matched, matched_named_parameters, err := glob.MatchNamed(s.URI, r.URL.Path)

	if err != nil {
		return false, nil
	}

	return matched, matched_named_parameters
}

func (s *serviceHttpRouteInfo) MatchMethod(method string) bool {
	return s.Method == method || s.Method == "*"
}

type Service struct {
	FnLastChance http.HandlerFunc
	Logger       *logger.Logger
	serviceName  string
	staticPath   string
	routes       []*serviceHttpRouteInfo
	done         chan struct{}
	mu           sync.RWMutex
	server       *http.Server
	port         int
}

func (s *Service) String() string {
	return "service::" + s.serviceName
}

// SetLoggingLevel sets the logging level for the server.
func (s *Service) SetLoggingLevel(level logger.LogLevel) *Service {
	s.Logger.SetLevel(level)
	return s
}

// SetStaticPath sets the static path for the server.
func (s *Service) SetStaticPath(path string) *Service {
	s.staticPath = path
	return s
}

// Start initializes the HTTP server and, if a service name is set,
// starts the load balancer registration goroutine.
func (s *Service) Start() error {
	addr := "0.0.0.0:" + strconv.Itoa(s.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		s.Logger.Errorln("Error starting HTTP server:", err)
		return err
	}

	var port int
	_, portStr, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return err
	}
	port, err = strconv.Atoi(portStr)
	if err != nil {
		return err
	}

	s.Logger.Infoln("Service started on", listener.Addr().String())

	// Only register with load balancer if a service name is set.
	if s.serviceName != "" {
		go func() {
			s.Logger.Infoln("Starting registration goroutine", s.serviceName, port)
			s.Logger.Infoln("https://io.moonlightcompanies.com/service/" + s.serviceName + "/")
			first := true
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-s.done:
					return
				case <-ticker.C:
				}

				resp, _, _ := Invoke[any]("__internal_register_service/register_service", map[string]interface{}{
					"name": s.serviceName,
					"port": port,
				})
				if first {
					s.Logger.Infoln("register_service result:", resp)
					first = false
					ticker.Reset(60 * time.Second)
				}
			}
		}()
	}

	s.server = &http.Server{
		Handler: s,
	}
	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			s.Logger.Errorln("HTTP server error:", err)
		}
	}()
	return nil
}

func (s *Service) Close() {
	close(s.done)
}

func (s *Service) RegisterRouteGET(uri string, fn ServiceHandleFunc) *serviceHttpRouteInfo {
	return s.RegisterRoute(uri, "GET", fn)
}

func (s *Service) RegisterRoutePOST(uri string, fn ServiceHandleFunc) *serviceHttpRouteInfo {
	return s.RegisterRoute(uri, "POST", fn)
}

func (s *Service) RegisterRouteALL(uri string, fn ServiceHandleFunc) *serviceHttpRouteInfo {
	return s.RegisterRoute(uri, "*", fn)
}

func (s *Service) RegisterRoute(uri, method string, fn ServiceHandleFunc) *serviceHttpRouteInfo {
	uri = replaceAllDoubleSlashes(uri)

	s.mu.Lock()
	defer s.mu.Unlock()

	result := NewServiceHttpRouteInfo(uri, method, fn)
	s.routes = append(s.routes, result)

	sort.SliceStable(s.routes, func(i, j int) bool {
		return len(s.routes[i].URI) > len(s.routes[j].URI)
	})

	return result
}

func (s *Service) ResolveRoute(r *http.Request) (*serviceHttpRouteInfo, map[string]string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// look for exact match first
	for _, route := range s.routes {
		if !route.MatchMethod(r.Method) {
			continue
		}
		if r.URL.Path == route.URI {
			return route, nil, true
		}
	}

	// look for glob match
	for _, route := range s.routes {
		if !route.MatchMethod(r.Method) {
			continue
		}

		if matched, named_parameters := route.MatchURL(r); matched {
			return route, named_parameters, true
		}
	}

	return nil, nil, false
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Logger.Debugln("Request", r.Method, r.URL.Path)

	sh, params_uri, found := s.ResolveRoute(r)
	parametersCtx, parametersErr := s.parameters(r, params_uri)
	if parametersErr != nil {
		WriteError(w, parametersErr)
		return
	}
	r = r.WithContext(parametersCtx)

	if found {
		atomic.AddInt32(&sh.Hits, 1)
		sh.Fn(w, r)
		return
	}

	if constantOk, _ := s.static_constant(w, r); constantOk {
		return
	}

	staticOk, staticErr := s.static(w, r)
	if staticOk {
		return
	}
	if staticErr != nil {
		WriteError(w, staticErr)
		return
	}

	if s.FnLastChance != nil {
		s.FnLastChance(w, r)
		return
	}

	http.Error(w, "not found", http.StatusNotFound)
}

// ServiceBuilder implements a builder pattern for Service.
type ServiceBuilder struct {
	port        int
	serviceName string
	logger      *logger.Logger
}

// NewServiceBuilder creates a new ServiceBuilder with default values.
func NewServiceBuilder() *ServiceBuilder {
	return &ServiceBuilder{
		port:        0,
		serviceName: "",
		logger:      logger.NewLogger(logger.LogLevelDebug, "service"),
	}
}

// SetPort sets the port for the service.
func (b *ServiceBuilder) SetPort(port int) *ServiceBuilder {
	b.port = port
	return b
}

// SetServiceName sets the service name for the service.
func (b *ServiceBuilder) SetServiceName(name string) *ServiceBuilder {
	b.serviceName = name
	return b
}

// Build creates a Service instance based on the builder's configuration.
func (b *ServiceBuilder) Build() *Service {
	return &Service{
		Logger:      b.logger,
		done:        make(chan struct{}),
		routes:      make([]*serviceHttpRouteInfo, 0),
		serviceName: b.serviceName,
		port:        b.port,
	}
}

// NewServiceWithName creates a new Service with the given service name.
// useful for top level initialization.
func NewServiceWithName(name string) *Service {
	return NewServiceBuilder().SetServiceName(name).Build()
}
