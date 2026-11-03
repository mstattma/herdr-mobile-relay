package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/0cv/herdr-mobile-relay/internal/conversation"
	"github.com/0cv/herdr-mobile-relay/internal/coordinator"
	"github.com/0cv/herdr-mobile-relay/internal/herdr"
	"github.com/0cv/herdr-mobile-relay/internal/protocol"
	"github.com/0cv/herdr-mobile-relay/internal/transport"
)

func configureDetailsProvider(t *testing.T, mode string) {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	argv, _ := json.Marshal([]string{binary, "-test.run=^TestSessionDetailsProviderProcess$", "--", mode})
	t.Setenv("HERDR_SESSION_DETAILS_COMMAND", string(argv))
}

func TestSessionDetailsProviderProcess(t *testing.T) {
	if len(os.Args) < 3 || os.Args[len(os.Args)-2] != "--" {
		return
	}
	var input map[string]string
	wantSession := "ses_detail"
	if os.Args[len(os.Args)-1] == "sessionless" {
		wantSession = ""
	}
	if json.NewDecoder(os.Stdin).Decode(&input) != nil || len(input) != 1 || input["session_id"] != wantSession || os.Getenv("SESSION_TEST_SECRET") != "" || os.Getenv("HERDR_SESSION_DETAILS_COMMAND") != "" {
		os.Exit(2)
	}
	fmt.Fprint(os.Stderr, "STDERR_SECRET_SENTINEL")
	switch os.Args[len(os.Args)-1] {
	case "sessionless":
		fmt.Fprint(os.Stdout, `{"rows":[{"label":"Account status","provider":"example","status":"ready","current":false}]}`)
	case "success":
		fmt.Fprint(os.Stdout, `{"rows":[{"label":"Personal display label","provider":"example","status":"ready","detail":"Usage 20%","weeklyReset":"Friday","limits":"20 / 100","paid":"$4.00","current":true,"token":"UNKNOWN_SECRET_SENTINEL","credentials":{"value":"NESTED_SECRET_SENTINEL"}}],"observed_at":123,"secret":"TOP_SECRET_SENTINEL"}`)
	case "failure":
		fmt.Fprint(os.Stdout, "RAW_ERROR_SECRET_SENTINEL")
		os.Exit(1)
	case "object":
		fmt.Fprint(os.Stdout, `{"rows":[{"label":"valid","limits":{"secret":"NESTED_SECRET_SENTINEL"}}]}`)
	case "overflow":
		fmt.Fprint(os.Stdout, strings.Repeat("x", 70*1024))
	case "wait":
		time.Sleep(time.Minute)
	default:
		os.Exit(3)
	}
	os.Exit(0)
}

func TestSessionDetailsProviderAllowlistAndCleanEnvironment(t *testing.T) {
	configureDetailsProvider(t, "success")
	t.Setenv("SESSION_TEST_SECRET", "ENV_SECRET_SENTINEL")
	result, reason := readSessionSubscriptions(context.Background(), "ses_detail")
	if reason != "" || result == nil || !result.Available || len(result.Rows) != 1 || result.Rows[0].Label != "Personal display label" || result.Rows[0].Paid != "$4.00" || !result.Rows[0].Current || result.ObservedAt == nil || *result.ObservedAt != 123 {
		t.Fatalf("provider result = %+v, reason %q", result, reason)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "SENTINEL") || strings.Contains(string(encoded), "credentials") || strings.Contains(string(encoded), "token") {
		t.Fatalf("provider leaked non-display material: %s", encoded)
	}
}

func TestSessionDetailsProviderUnavailableAndFailures(t *testing.T) {
	t.Setenv("HERDR_SESSION_DETAILS_COMMAND", "")
	if result, reason := readSessionSubscriptions(context.Background(), "ses_detail"); result != nil || reason != "" {
		t.Fatalf("unset provider = %+v, %q", result, reason)
	}
	for _, mode := range []string{"failure", "object", "overflow", "wait", "bad-config"} {
		t.Run(mode, func(t *testing.T) {
			configureDetailsProvider(t, mode)
			ctx := context.Background()
			if mode == "wait" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			}
			if mode == "bad-config" {
				t.Setenv("HERDR_SESSION_DETAILS_COMMAND", "RAW_CONFIG_SECRET_SENTINEL")
			}
			result, reason := readSessionSubscriptions(ctx, "ses_detail")
			if result == nil || result.Available || len(result.Rows) != 0 || reason != "subscriptions_failed" {
				t.Fatalf("failed provider = %+v, %q", result, reason)
			}
			encoded, _ := json.Marshal(result)
			if strings.Contains(string(encoded), "SENTINEL") {
				t.Fatalf("provider leaked error: %s", encoded)
			}
		})
	}
}

func TestSessionDetailsAuthorization(t *testing.T) {
	for _, operation := range []string{"get_session_details", "cycle_agent"} {
		action, known := protocol.ClassifyAction(operation)
		if !known {
			t.Fatalf("unregistered action %s", operation)
		}
		if authorizeAuthenticatedIdentity(transport.AuthenticatedIdentity{}, false, action, "") == nil {
			t.Fatalf("unauthenticated %s accepted", operation)
		}
		reader := transport.AuthenticatedIdentity{Role: string(protocol.RoleReader)}
		if denied := authorizeAuthenticatedIdentity(reader, true, action, "") != nil; denied != (operation == "cycle_agent") {
			t.Fatalf("reader %s denied = %v", operation, denied)
		}
		if authorizeAuthenticatedIdentity(transport.AuthenticatedIdentity{Role: string(protocol.RoleController)}, true, action, "") != nil {
			t.Fatalf("controller %s denied", operation)
		}
	}
}

func TestSessionDetailsFallbackAndStaleTarget(t *testing.T) {
	configureDetailsProvider(t, "success")
	state := coordinator.NewState(slog.New(slog.NewTextHandler(io.Discard, nil)))
	state.CommitInventory([]*coordinator.AgentState{{PaneID: "pane-1", TerminalID: "terminal-1", SessionID: "ses_detail", AgentSessionID: "ses_detail", Agent: "opencode", Cwd: "/work/fallback", SessionName: "Fallback title"}}, state.RevisionCounter())
	agent, _ := state.Agent("pane-1")
	server := &Server{state: state, conversationM: conversation.NewReader(t.TempDir())}
	inbound := protocol.Inbound{Type: "get_session_details", RequestID: "details-1", PaneID: "pane-1", Target: &protocol.TargetRef{ServerSessionID: "primary", PaneID: "pane-1", TerminalID: "terminal-1", Generation: agent.Generation, AgentSessionID: "ses_detail"}}
	result := server.getSessionDetails(context.Background(), inbound)
	if !result.OK {
		t.Fatalf("details = %+v", result)
	}
	encoded, _ := json.Marshal(result.Data)
	var details struct {
		conversation.SessionDetails
		Subscriptions *sessionSubscriptions `json:"subscriptions"`
	}
	if json.Unmarshal(encoded, &details) != nil || details.SessionID != inbound.Target.AgentSessionID || details.Title != "Fallback title" || details.CWD != "/work/fallback" || details.TodosAvailable || len(details.Todos) != 0 || details.Subscriptions == nil || !details.Subscriptions.Available {
		t.Fatalf("fallback wire = %s", encoded)
	}
	// The dispatch hook runs once before I/O and once after it. Replacing the
	// target at publication must drop even a successful provider response.
	checks := 0
	ctx := herdr.WithDispatchCheck(context.Background(), func() error {
		checks++
		if checks == 2 {
			state.BumpGeneration("pane-1")
		}
		return nil
	})
	result = server.getSessionDetails(ctx, inbound)
	if result.OK || result.Data != nil {
		t.Fatalf("stale details published: %+v", result)
	}
	inbound.Target = nil
	if result = server.getSessionDetails(context.Background(), inbound); result.OK || result.Data != nil {
		t.Fatalf("targetless details accepted: %+v", result)
	}
}

func TestSessionDetailsBeforeNativeSessionExists(t *testing.T) {
	configureDetailsProvider(t, "sessionless")
	state := coordinator.NewState(slog.New(slog.NewTextHandler(io.Discard, nil)))
	state.CommitInventory([]*coordinator.AgentState{{PaneID: "pane-1", TerminalID: "terminal-1", Agent: "opencode", Cwd: "/work/example"}}, state.RevisionCounter())
	agent, _ := state.Agent("pane-1")
	server := &Server{state: state}
	inbound := protocol.Inbound{Type: "get_session_details", PaneID: "pane-1", Target: &protocol.TargetRef{ServerSessionID: "primary", PaneID: "pane-1", TerminalID: "terminal-1", Generation: agent.Generation}}
	result := server.getSessionDetails(context.Background(), inbound)
	encoded, _ := json.Marshal(result.Data)
	var details struct {
		conversation.SessionDetails
		Subscriptions *sessionSubscriptions `json:"subscriptions"`
	}
	if !result.OK || json.Unmarshal(encoded, &details) != nil || details.SessionID != "" || details.Reason == "invalid_session" || details.TodosAvailable || details.Subscriptions == nil || !details.Subscriptions.Available || len(details.Subscriptions.Rows) != 1 || details.Subscriptions.Rows[0].Current {
		t.Fatalf("sessionless details = %+v, data %s", result, encoded)
	}
}
