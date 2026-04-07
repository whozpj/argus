package alerts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Notifier dispatches drift alerts to an external channel.
type Notifier interface {
	// Fire is called when drift_score crosses the alert threshold.
	Fire(model string, score, pOutputTokens, pLatencyMs float64) error
	// Clear is called when the alert is resolved.
	Clear(model string) error
}

// Noop is a no-op Notifier used when no webhook is configured.
type Noop struct{}

func (Noop) Fire(string, float64, float64, float64) error { return nil }
func (Noop) Clear(string) error                           { return nil }

// SlackNotifier posts messages to a Slack incoming webhook URL.
type SlackNotifier struct {
	webhookURL string
	client     *http.Client
}

// NewSlack returns a Notifier that posts to the given Slack webhook URL.
func NewSlack(webhookURL string) *SlackNotifier {
	return &SlackNotifier{
		webhookURL: webhookURL,
		client:     &http.Client{Timeout: 5 * time.Second},
	}
}

func (s *SlackNotifier) Fire(model string, score, pOutputTokens, pLatencyMs float64) error {
	msg := slackMessage{
		Text: fmt.Sprintf("⚠️ *Drift detected:* `%s`  (score: %.2f)", model, score),
		Blocks: []slackBlock{
			headerBlock("⚠️ Argus Drift Alert"),
			sectionBlock([][2]string{
				{"Model", fmt.Sprintf("`%s`", model)},
				{"Score", fmt.Sprintf("%.4f", score)},
				{"p(output_tokens)", fmt.Sprintf("%.4f", pOutputTokens)},
				{"p(latency_ms)", fmt.Sprintf("%.4f", pLatencyMs)},
			}),
		},
	}
	return s.post(msg)
}

func (s *SlackNotifier) Clear(model string) error {
	msg := slackMessage{
		Text: fmt.Sprintf("✅ *Drift resolved:* `%s`", model),
		Blocks: []slackBlock{
			headerBlock("✅ Argus Drift Resolved"),
			sectionBlock([][2]string{
				{"Model", fmt.Sprintf("`%s`", model)},
				{"Status", "Back to baseline"},
			}),
		},
	}
	return s.post(msg)
}

func (s *SlackNotifier) post(msg slackMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}
	resp, err := s.client.Post(s.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("post to slack: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("slack returned %d", resp.StatusCode)
	}
	return nil
}

// ---- Slack Block Kit types (minimal subset) ----

type slackMessage struct {
	Text   string       `json:"text"`
	Blocks []slackBlock `json:"blocks,omitempty"`
}

type slackBlock struct {
	Type   string      `json:"type"`
	Text   *slackText  `json:"text,omitempty"`
	Fields []slackText `json:"fields,omitempty"`
}

type slackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func headerBlock(text string) slackBlock {
	return slackBlock{
		Type: "header",
		Text: &slackText{Type: "plain_text", Text: text},
	}
}

func sectionBlock(fields [][2]string) slackBlock {
	f := make([]slackText, 0, len(fields))
	for _, kv := range fields {
		f = append(f, slackText{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*%s*\n%s", kv[0], kv[1]),
		})
	}
	return slackBlock{Type: "section", Fields: f}
}
