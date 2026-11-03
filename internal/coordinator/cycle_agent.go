package coordinator

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/0cv/herdr-mobile-relay/internal/herdr"
	"github.com/0cv/herdr-mobile-relay/internal/noecho"
	"github.com/0cv/herdr-mobile-relay/internal/protocol"
	"github.com/0cv/herdr-mobile-relay/internal/question"
)

func CycleAgentEligible(agent *AgentState) bool {
	if agent == nil || !strings.EqualFold(strings.TrimSpace(agent.Agent), "opencode") || agent.Interaction != nil || agent.QuestionLayout || len(agent.Options) != 0 {
		return false
	}
	if agent.AttentionKind != "" && agent.AttentionKind != question.AttentionChat {
		return false
	}
	return agent.Status == "idle" || agent.Status == "done" || agent.Status == "blocked" && agent.AttentionKind == question.AttentionChat
}

func (d *Dispatcher) HandleCycleAgent(ctx context.Context, message map[string]any, admitted func()) *CommandResult {
	const action = "cycle_agent"
	ctx = context.WithValue(ctx, admissionContextKey{}, admitted)
	if admitted != nil {
		defer admitted()
	}
	inbound, err := protocol.DecodeMap(message)
	reject := func() *CommandResult {
		return &CommandResult{RequestID: inbound.RequestID, Action: action, PaneID: inbound.PaneID, Phase: "not_started", Error: "Agent cycling requires a current normal OpenCode prompt"}
	}
	if err != nil || inbound.PaneID == "" || inbound.Target == nil {
		return reject()
	}
	for key := range message {
		switch key {
		case "type", "action", "protocol", "request_id", "action_id", "target", "pane_id", "server_session_id", "client_id", "_server_received_at", "_server_sequence":
		default:
			return reject()
		}
	}
	current := func() error {
		agent, exists := d.state.Agent(inbound.PaneID)
		if !exists || !d.state.InventoryReady() || !CycleAgentEligible(agent) {
			return ErrPaneReplaced
		}
		server := agent.ServerSessionID
		if server == "" {
			server = "primary"
		}
		target := inbound.Target
		if target.PaneID != agent.PaneID || target.ServerSessionID != server || target.TerminalID == "" || target.TerminalID != agent.TerminalID || target.Generation != agent.Generation || target.AgentSessionID != agent.SessionID {
			return ErrPaneReplaced
		}
		return nil
	}
	ctx = herdr.WithDispatchCheck(ctx, current)
	if herdr.CheckDispatch(ctx) != nil {
		return reject()
	}
	receivedAt := time.Now()
	if raw, ok := message["_server_received_at"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			receivedAt = parsed
		}
	}
	if sequence := uint64Value(message["_server_sequence"]); sequence != 0 {
		ctx = context.WithValue(ctx, receiptContextKey{}, CommandID(sequence))
	}
	return d.schedule(ctx, ScheduleOptions{Command: d.command(ctx, receivedAt, inbound.RequestID, CommandCycleAgent, inbound.PaneID, commandDeadline, nil)}, EffectFunc(func(effectCtx context.Context, token WorkerToken) EffectResult {
		if herdr.CheckDispatch(effectCtx) != nil {
			return EffectResult{Result: reject()}
		}
		read, err := d.herdr.ReadPaneVisible(effectCtx, inbound.PaneID, 200, "text")
		if err != nil || read.Truncated || !question.OpenCodeCyclePrompt(string(read.Content)) {
			return EffectResult{Result: reject()}
		}
		if _, secret := noecho.Match(string(read.Content)); secret || herdr.CheckDispatch(effectCtx) != nil {
			return EffectResult{Result: reject()}
		}
		// No requested mode or phone key is forwarded. The native UI owns the
		// mode transition; a successful send does not identify the selected mode.
		if err := d.herdr.SendKeys(effectCtx, inbound.PaneID, []string{"Tab"}); err != nil {
			if errors.Is(err, ErrPaneReplaced) {
				return EffectResult{Result: reject()}
			}
			return EffectResult{Result: d.failErr(inbound.RequestID, action, inbound.PaneID, err)}
		}
		d.recordActivity(action, "completed", "Agent cycle key sent", inbound.PaneID, inbound.RequestID)
		return EffectResult{Result: &CommandResult{RequestID: inbound.RequestID, Action: action, PaneID: inbound.PaneID, OK: true, Phase: "sent", Data: map[string]any{"sent": true}}}
	}))
}
