package cli

import (
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	maxOutputMetadataRunes = 256
	maxOutputDetailRunes   = 4096
)

type successEnvelope struct {
	OK       bool     `json:"ok"`
	Schema   string   `json:"schema"`
	Data     any      `json:"data"`
	Warnings []string `json:"warnings"`
	Hints    []string `json:"hints"`
}

type cliError struct {
	Code              string   `json:"code"`
	Message           string   `json:"message"`
	Fix               string   `json:"fix"`
	Retryable         bool     `json:"retryable"`
	RetryAfterSeconds int      `json:"retry_after_seconds,omitempty"`
	AcceptedValues    []string `json:"accepted_values,omitempty"`
	Examples          []string `json:"examples"`
}

type errorResponse struct {
	OK     bool      `json:"ok"`
	Schema string    `json:"schema"`
	Error  *cliError `json:"error"`
}

func okEnvelope(schema string, data any) successEnvelope {
	return successEnvelope{OK: true, Schema: schema, Data: data, Warnings: []string{}, Hints: []string{}}
}

func errorEnvelope(err *cliError) errorResponse {
	return errorResponse{OK: false, Schema: "plane.error.v1", Error: err}
}

func newError(code, message, fix string, retryable bool, examples ...string) *cliError {
	return &cliError{Code: code, Message: message, Fix: fix, Retryable: retryable, Examples: examples}
}

func newRateLimitError(retryAfterSeconds int) *cliError {
	return &cliError{
		Code:              "RATE_LIMITED",
		Message:           "Plane API returned HTTP 429.",
		Fix:               "Retry after the indicated delay or lower mutation concurrency.",
		Retryable:         true,
		RetryAfterSeconds: retryAfterSeconds,
		Examples:          []string{},
	}
}

func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(sanitizeAgentOutput(v))
}

func sanitizeAgentOutput(value any) any {
	encoded, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return value
	}
	return sanitizeJSONValue(decoded)
}

func sanitizeJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return sanitizeJSONMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = sanitizeJSONValue(item)
		}
		return out
	default:
		return value
	}
}

func sanitizeJSONMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		if (key == "description_html" || key == "comment_html") && value != nil {
			html := stringValue(value)
			textKey := strings.TrimSuffix(key, "_html") + "_text"
			text := stripHTML(html)
			bounded, truncated := boundOutputText(text, maxOutputDetailRunes)
			out[textKey] = bounded
			out[textKey+"_original_runes"] = utf8.RuneCountInString(text)
			if truncated {
				out[textKey+"_truncated"] = true
			}
			continue
		}
		if text, ok := value.(string); ok {
			if limit, boundedField := outputTextLimit(key); boundedField {
				bounded, truncated := boundOutputText(text, limit)
				out[key] = bounded
				if truncated {
					out[key+"_truncated"] = true
					out[key+"_original_runes"] = utf8.RuneCountInString(text)
				}
				continue
			}
		}
		out[key] = sanitizeJSONValue(value)
	}
	return out
}

func outputTextLimit(key string) (int, bool) {
	switch key {
	case "name", "display_name", "first_name", "last_name", "author_name", "title":
		return maxOutputMetadataRunes, true
	case "description_text", "comment_text", "excerpt":
		return maxOutputDetailRunes, true
	default:
		return 0, false
	}
}

func boundOutputText(text string, limit int) (string, bool) {
	if limit < 1 || utf8.RuneCountInString(text) <= limit {
		return text, false
	}
	runes := []rune(text)
	return string(runes[:limit-1]) + "…", true
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
