package upstream

import (
	"io"
)

// DrainAndClose drains and closes a response body to return the connection
// to the pool. Safe to call with a nil body.
func DrainAndClose(body io.ReadCloser) {
	if body != nil {
		_, _ = io.Copy(io.Discard, body)
		_ = body.Close()
	}
}
