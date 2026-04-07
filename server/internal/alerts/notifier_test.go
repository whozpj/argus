package alerts

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// captureServer returns a test HTTP server that records the last request body.
func captureServer(t *testing.T, statusCode int) (*httptest.Server, *[]string) {
	t.Helper()
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		w.WriteHeader(statusCode)
	}))
	t.Cleanup(srv.Close)
	return srv, &bodies
}

// ---------------------------------------------------------------------------
// SlackNotifier.Fire
// ---------------------------------------------------------------------------

func TestSlackFire_PostsToWebhook(t *testing.T) {
	srv, bodies := captureServer(t, http.StatusOK)
	n := NewSlack(srv.URL)

	if err := n.Fire("claude-sonnet-4-6", 0.94, 0.001, 0.42); err != nil {
		t.Fatalf("Fire: %v", err)
	}
	if len(*bodies) != 1 {
		t.Fatalf("expected 1 POST, got %d", len(*bodies))
	}
}

func TestSlackFire_BodyContainsModel(t *testing.T) {
	srv, bodies := captureServer(t, http.StatusOK)
	n := NewSlack(srv.URL)

	n.Fire("gpt-4o", 0.85, 0.01, 0.3) //nolint:errcheck

	if !strings.Contains((*bodies)[0], "gpt-4o") {
		t.Errorf("body does not contain model name:\n%s", (*bodies)[0])
	}
}

func TestSlackFire_BodyContainsScore(t *testing.T) {
	srv, bodies := captureServer(t, http.StatusOK)
	n := NewSlack(srv.URL)

	n.Fire("model-x", 0.9876, 0.01, 0.3) //nolint:errcheck

	if !strings.Contains((*bodies)[0], "0.9876") {
		t.Errorf("body does not contain score:\n%s", (*bodies)[0])
	}
}

func TestSlackFire_ValidJSON(t *testing.T) {
	srv, bodies := captureServer(t, http.StatusOK)
	n := NewSlack(srv.URL)

	n.Fire("model-x", 0.9, 0.01, 0.3) //nolint:errcheck

	var msg slackMessage
	if err := json.Unmarshal([]byte((*bodies)[0]), &msg); err != nil {
		t.Errorf("body is not valid JSON: %v\nbody: %s", err, (*bodies)[0])
	}
	if msg.Text == "" {
		t.Error("text field is empty")
	}
}

func TestSlackFire_ReturnsErrorOn4xx(t *testing.T) {
	srv, _ := captureServer(t, http.StatusForbidden)
	n := NewSlack(srv.URL)

	err := n.Fire("model-x", 0.9, 0.01, 0.3)
	if err == nil {
		t.Error("expected error for 403 response")
	}
}

func TestSlackFire_ReturnsErrorOnNetworkFailure(t *testing.T) {
	n := NewSlack("http://127.0.0.1:1") // nothing listening
	err := n.Fire("model-x", 0.9, 0.01, 0.3)
	if err == nil {
		t.Error("expected error when server is unreachable")
	}
}

// ---------------------------------------------------------------------------
// SlackNotifier.Clear
// ---------------------------------------------------------------------------

func TestSlackClear_PostsToWebhook(t *testing.T) {
	srv, bodies := captureServer(t, http.StatusOK)
	n := NewSlack(srv.URL)

	if err := n.Clear("claude-sonnet-4-6"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if len(*bodies) != 1 {
		t.Fatalf("expected 1 POST, got %d", len(*bodies))
	}
}

func TestSlackClear_BodyContainsModel(t *testing.T) {
	srv, bodies := captureServer(t, http.StatusOK)
	n := NewSlack(srv.URL)

	n.Clear("gpt-4o") //nolint:errcheck

	if !strings.Contains((*bodies)[0], "gpt-4o") {
		t.Errorf("clear body does not contain model name:\n%s", (*bodies)[0])
	}
}

// ---------------------------------------------------------------------------
// Noop
// ---------------------------------------------------------------------------

func TestNoop_NeverErrors(t *testing.T) {
	n := Noop{}
	if err := n.Fire("m", 0.9, 0.01, 0.3); err != nil {
		t.Errorf("Noop.Fire returned error: %v", err)
	}
	if err := n.Clear("m"); err != nil {
		t.Errorf("Noop.Clear returned error: %v", err)
	}
}
