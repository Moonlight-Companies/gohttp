package service

import "strings"

func containsAcceptType(acceptHeader, expectedType string) bool {
	parts := strings.Split(acceptHeader, ",")
	for _, part := range parts {
		part = strings.TrimSpace(strings.Split(part, ";")[0]) // Ignore parameters like charset
		if part == expectedType {
			return true
		}
	}
	return false
}

func replaceAllDoubleSlashes(uri string) string {
	for strings.Contains(uri, "//") {
		uri = strings.ReplaceAll(uri, "//", "/")
	}
	return uri
}
