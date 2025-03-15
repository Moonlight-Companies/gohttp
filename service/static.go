package service

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var Extensions = []string{
	".js",
	".css",
	".html",
	".json",
	".txt",
	".png",
	".jpg",
	".jpeg",
}

func getContentType(suffix string) string {
	switch suffix {
	case ".js":
		return "application/javascript"
	case ".css":
		return "text/css"
	case ".html":
		return "text/html"
	case ".json":
		return "application/json"
	case ".txt":
		return "text/plain"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	}
	return "text/plain"
}

type FnReplaceMacros func(r *http.Request, content []byte) []byte

var StaticReplaceMacrosFn FnReplaceMacros

func (s *Service) static(w http.ResponseWriter, r *http.Request) (bool, error) {
	relativePath := "/"

	if s.serviceName != "" {
		prefix := "/service/" + s.serviceName
		relativePath = strings.TrimPrefix(r.URL.Path, prefix)
	} else {
                relativePath = r.URL.Path
        }

	if relativePath == "" || relativePath == "/" {
		relativePath = "/index.html"
	}

	var StaticPath string = "./static"
	if s.staticPath != "" {
		StaticPath = s.staticPath
	}

	if _, err := os.Stat(StaticPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	filePath := filepath.Join(StaticPath, relativePath)
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	shouldIntercept := false
	var matchingSuffix string
	for _, suffix := range Extensions {
		if strings.HasSuffix(filePath, suffix) {
			shouldIntercept = true
			matchingSuffix = suffix
			break
		}
	}

	if shouldIntercept {
		contentType := getContentType(matchingSuffix)
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-cache")

		contents, err := os.ReadFile(filePath)
		if err != nil {
			return false, nil
		}

		if StaticReplaceMacrosFn != nil {
			contents = StaticReplaceMacrosFn(r, contents)
		}

		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(contents)))
		if _, err := w.Write(contents); err != nil {
			s.Logger.Errorln("http_sse_static_middleware", "failed to write", err)
			return false, err
		}
		return true, nil
	}

	return false, nil
}
