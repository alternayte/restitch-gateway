package composition

import (
	"net/http"
	"net/url"
)

// RequestData captures the inbound request data needed by the executor,
// decoupling the composition engine from *http.Request.
type RequestData struct {
	Method  string
	Path    string
	Params  map[string]string
	Query   url.Values
	Headers http.Header
	Body    any
}

// NewRequestData extracts request data from an HTTP request.
func NewRequestData(r *http.Request, params map[string]string, body any) *RequestData {
	return &RequestData{
		Method:  r.Method,
		Path:    r.URL.Path,
		Params:  params,
		Query:   r.URL.Query(),
		Headers: r.Header,
		Body:    body,
	}
}
