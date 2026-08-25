package mailcom

import (
	"encoding/json"
	"strings"
)

type serverSentEvent struct {
	ID    string
	Event string
	Data  string
}

func parseSSE(text string) []serverSentEvent {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	chunks := strings.Split(normalized, "\n\n")
	events := make([]serverSentEvent, 0, len(chunks))
	for _, chunk := range chunks {
		lines := strings.Split(chunk, "\n")
		var id, event string
		var dataLines []string
		for _, line := range lines {
			if line == "" || strings.HasPrefix(line, ":") {
				continue
			}
			field, value, _ := strings.Cut(line, ":")
			if strings.HasPrefix(value, " ") {
				value = value[1:]
			}
			switch field {
			case "id":
				id = value
			case "event":
				event = value
			case "data":
				dataLines = append(dataLines, value)
			}
		}
		if id == "" && event == "" && len(dataLines) == 0 {
			continue
		}
		events = append(events, serverSentEvent{ID: id, Event: event, Data: strings.Join(dataLines, "\n")})
	}
	return events
}

func parseSSEJSON[T any](text string) ([]T, error) {
	events := parseSSE(text)
	out := make([]T, 0, len(events))
	for _, event := range events {
		data := strings.TrimSpace(event.Data)
		if !strings.HasPrefix(data, "{") && !strings.HasPrefix(data, "[") {
			continue
		}
		var item T
		if err := json.Unmarshal([]byte(data), &item); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func parseMailSubmissionResult(text string) (SubmissionResult, error) {
	events := parseSSE(text)
	for _, event := range events {
		if event.Event == "error" && strings.TrimSpace(event.Data) != "" {
			return SubmissionResult{}, &Error{Message: "mail.com submission failed: " + strings.TrimSpace(event.Data)}
		}
	}
	for _, event := range events {
		if event.Event == "success" && strings.TrimSpace(event.Data) != "" {
			raw := strings.TrimSpace(event.Data)
			parts := strings.Split(strings.Trim(raw, "/"), "/")
			encoded := raw
			if len(parts) > 0 {
				encoded = parts[len(parts)-1]
			}
			return SubmissionResult{MessageID: safeDecode(encoded), RawLocation: raw}, nil
		}
	}
	return SubmissionResult{}, &Error{Message: "mail.com submission did not return a success event"}
}
