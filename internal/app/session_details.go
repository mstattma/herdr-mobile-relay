package app

import (
	"context"
	"strings"

	"github.com/0cv/herdr-mobile-relay/internal/conversation"
	"github.com/0cv/herdr-mobile-relay/internal/coordinator"
	"github.com/0cv/herdr-mobile-relay/internal/herdr"
	"github.com/0cv/herdr-mobile-relay/internal/protocol"
)

func (s *Server) getSessionDetails(ctx context.Context, inbound protocol.Inbound) *coordinator.CommandResult {
	result := &coordinator.CommandResult{RequestID: inbound.RequestID, Action: "get_session_details", PaneID: inbound.PaneID, Phase: "not_started", Error: "Session details target is unavailable"}
	if inbound.PaneID == "" || inbound.Target == nil || validateExactPaneTarget(s.state, inbound, true) != nil || herdr.CheckDispatch(ctx) != nil {
		return result
	}
	agent, exists := s.state.Agent(inbound.PaneID)
	if !exists {
		return result
	}
	details := conversation.SessionDetails{SessionID: agent.SessionID, Todos: []conversation.SessionTodo{}, Reason: "todos_unavailable"}
	if strings.EqualFold(strings.TrimSpace(agent.Agent), "opencode") && agent.SessionID != "" {
		details = s.conversationM.OpenCodeSessionDetails(ctx, agent.SessionID)
	}
	if details.Title == "" {
		details.Title = sessionDisplayText(agent.SessionName, 512)
	}
	if details.CWD == "" {
		details.CWD = sessionDisplayText(agent.Cwd, 4096)
	}
	subscriptions, reason := readSessionSubscriptions(ctx, agent.SessionID)
	if reason != "" {
		details.Reason = reason
	}
	if ctx.Err() != nil || herdr.CheckDispatch(ctx) != nil {
		return result
	}
	current, exists := s.state.Agent(inbound.PaneID)
	if !exists || !sameSlashTarget(agent, current) || validateExactPaneTarget(s.state, inbound, true) != nil {
		return result
	}
	result.OK, result.Phase, result.Error = true, "completed", ""
	result.Data = struct {
		conversation.SessionDetails
		Subscriptions *sessionSubscriptions `json:"subscriptions,omitempty"`
	}{details, subscriptions}
	return result
}
