package helper

import "net/http"

func Used() int {
	return 42
}

func UnusedHTTP() *http.Client {
	return http.DefaultClient
}
