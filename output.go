package main

import (
	"encoding/json"
	"io"
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
	_ = enc.Encode(v)
}
