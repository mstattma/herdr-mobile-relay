package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode"

	"github.com/0cv/herdr-mobile-relay/internal/processgroup"
)

// HERDR_SESSION_DETAILS_COMMAND is a local administrator's JSON argv array,
// never a phone-supplied command. The exporter must be read-only: no network
// requests, authentication refresh, or mutations. Only session_id is supplied
// on stdin. Display strings are account-wide; current is session-specific.
// All display fields (including limits, paid and weeklyReset) are strings.
// observed_at is Unix milliseconds. Exporters must exclude credentials from
// display fields. Unknown JSON fields are dropped, never forwarded as objects.
type sessionSubscriptionRow struct {
	Label       string `json:"label"`
	Provider    string `json:"provider"`
	Status      string `json:"status"`
	Detail      string `json:"detail"`
	WeeklyReset string `json:"weeklyReset"`
	Limits      string `json:"limits"`
	Paid        string `json:"paid"`
	Current     bool   `json:"current"`
}

type sessionSubscriptions struct {
	Available  bool                     `json:"available"`
	Rows       []sessionSubscriptionRow `json:"rows"`
	ObservedAt *int64                   `json:"observed_at,omitempty"`
}

var sessionProviderSlots = make(chan struct{}, 2)

func readSessionSubscriptions(ctx context.Context, sessionID string) (*sessionSubscriptions, string) {
	config := strings.TrimSpace(os.Getenv("HERDR_SESSION_DETAILS_COMMAND"))
	if config == "" {
		return nil, ""
	}
	result := &sessionSubscriptions{Rows: []sessionSubscriptionRow{}}
	var argv []string
	if len(config) > 16*1024 || json.Unmarshal([]byte(config), &argv) != nil || len(argv) == 0 || len(argv) > 64 || argv[0] == "" || len(sessionID) > 256 {
		return result, "subscriptions_failed"
	}
	for _, arg := range argv {
		if len(arg) > 4096 || strings.ContainsRune(arg, 0) {
			return result, "subscriptions_failed"
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	select {
	case sessionProviderSlots <- struct{}{}:
		defer func() { <-sessionProviderSlots }()
	case <-ctx.Done():
		return result, "subscriptions_failed"
	}
	input, _ := json.Marshal(struct {
		SessionID string `json:"session_id"`
	}{sessionID})
	command := exec.Command(argv[0], argv[1:]...)
	command.Dir = "/"
	command.Env = []string{"PATH=/usr/bin:/bin", "LANG=C.UTF-8"}
	if home, err := os.UserHomeDir(); err == nil {
		command.Env = append(command.Env, "HOME="+home)
	}
	command.Stdin = bytes.NewReader(input)
	output := &sessionProviderOutput{cancel: cancel}
	command.Stdout, command.Stderr = output, io.Discard
	if err := processgroup.RunWithWriters(ctx, command, processgroup.Options{PostKillWait: time.Second}); err != nil {
		return result, "subscriptions_failed"
	}
	var response struct {
		Rows       []sessionSubscriptionRow `json:"rows"`
		ObservedAt *int64                   `json:"observed_at"`
	}
	if json.Unmarshal(output.buffer.Bytes(), &response) != nil || response.Rows == nil || len(response.Rows) > 64 || (response.ObservedAt != nil && *response.ObservedAt < 0) {
		return result, "subscriptions_failed"
	}
	for _, row := range response.Rows {
		row.Label = sessionDisplayText(row.Label, 256)
		row.Provider = sessionDisplayText(row.Provider, 128)
		row.Status = sessionDisplayText(row.Status, 128)
		row.Detail = sessionDisplayText(row.Detail, 1024)
		row.WeeklyReset = sessionDisplayText(row.WeeklyReset, 256)
		row.Limits = sessionDisplayText(row.Limits, 1024)
		row.Paid = sessionDisplayText(row.Paid, 256)
		result.Rows = append(result.Rows, row)
	}
	result.Available, result.ObservedAt = true, response.ObservedAt
	return result, ""
}

type sessionProviderOutput struct {
	buffer bytes.Buffer
	cancel context.CancelFunc
}

func (output *sessionProviderOutput) Write(value []byte) (int, error) {
	if output.buffer.Len()+len(value) > 64*1024 {
		output.cancel()
		return 0, errors.New("session provider output limit")
	}
	return output.buffer.Write(value)
}

func sessionDisplayText(value string, limit int) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	if len(value) > limit {
		value = strings.ToValidUTF8(value[:limit], "")
	}
	return strings.TrimSpace(value)
}
