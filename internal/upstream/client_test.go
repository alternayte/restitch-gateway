package upstream

import (
	"testing"
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
