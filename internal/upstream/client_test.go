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

package upstream

import (
	"testing"
	"time"
)

func TestBuildTransport_Defaults(t *testing.T) {
	tr := BuildTransport(TransportConfig{})

	if tr.ForceAttemptHTTP2 != true {
		t.Error("expected ForceAttemptHTTP2 = true")
	}

	if tr.MaxIdleConnsPerHost != 100 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 100", tr.MaxIdleConnsPerHost)
	}

	if tr.TLSClientConfig == nil {
		t.Fatal("expected TLSClientConfig")
	}

	if tr.TLSClientConfig.InsecureSkipVerify != false {
		t.Error("expected InsecureSkipVerify = false by default")
	}

	if tr.Proxy == nil {
		t.Error("expected Proxy to be set")
	}
}

func TestBuildTransport_CustomValues(t *testing.T) {
	tr := BuildTransport(TransportConfig{
		MaxIdleConnsPerHost: 50,
		InsecureSkipVerify:  true,
	})

	if tr.MaxIdleConnsPerHost != 50 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 50", tr.MaxIdleConnsPerHost)
	}

	if !tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify = true")
	}
}

// docs/configuration.md documents response_header_timeout as defaulting to
// "10s". Before this default existed, a zero-value TransportConfig produced
// an *http.Transport with NO response-header timeout at all.
func TestBuildTransport_ResponseHeaderTimeoutDefault(t *testing.T) {
	tr := BuildTransport(TransportConfig{})

	if tr.ResponseHeaderTimeout != 10*time.Second {
		t.Errorf("ResponseHeaderTimeout = %v, want 10s default", tr.ResponseHeaderTimeout)
	}
}

func TestBuildTransport_ResponseHeaderTimeoutOverride(t *testing.T) {
	tr := BuildTransport(TransportConfig{ResponseHeaderTimeout: 2 * time.Second})

	if tr.ResponseHeaderTimeout != 2*time.Second {
		t.Errorf("ResponseHeaderTimeout = %v, want 2s override", tr.ResponseHeaderTimeout)
	}
}
