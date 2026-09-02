// Copyright 2026 Restitch maintainers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
