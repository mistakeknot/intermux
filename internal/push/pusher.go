package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/mistakeknot/intermux/internal/activity"
	"github.com/mistakeknot/intermux/internal/idle"
)

// idlePushInterval is the push interval when the server is idle.
const idlePushInterval = 5 * time.Minute

// Pusher periodically pushes agent metadata to intermute.
type Pusher struct {
	store       *activity.Store
	baseURL     string
	interval    time.Duration
	idleTracker *idle.Tracker
	httpClient  *http.Client
}

// NewPusher creates a metadata pusher.
func NewPusher(store *activity.Store, baseURL string, interval time.Duration) *Pusher {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Pusher{
		store:    store,
		baseURL:  baseURL,
		interval: interval,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// SetIdleTracker attaches an idle tracker for adaptive tick rates.
func (p *Pusher) SetIdleTracker(t *idle.Tracker) {
	p.idleTracker = t
}

// Run starts the push loop. Blocks until context is cancelled.
// When an idle tracker is attached, backs off to idlePushInterval when
// no MCP traffic has been seen.
func (p *Pusher) Run(ctx context.Context) {
	if p.baseURL == "" {
		log.Printf("intermux: metadata push disabled (no INTERMUTE_URL)")
		return
	}
	log.Printf("intermux: metadata pusher started (url=%s, interval=%s)", p.baseURL, p.interval)

	activeTicker := time.NewTicker(p.interval)
	idleTicker := time.NewTicker(idlePushInterval)
	defer activeTicker.Stop()
	defer idleTicker.Stop()

	for {
		if p.idleTracker != nil && p.idleTracker.IsIdle() {
			select {
			case <-ctx.Done():
				log.Printf("intermux: metadata pusher stopped")
				return
			case <-idleTicker.C:
				p.push(ctx)
			case <-p.idleTracker.WakeCh():
				p.push(ctx)
				activeTicker.Reset(p.interval)
			}
		} else {
			select {
			case <-ctx.Done():
				log.Printf("intermux: metadata pusher stopped")
				return
			case <-activeTicker.C:
				p.push(ctx)
			}
		}
	}
}

func (p *Pusher) push(ctx context.Context) {
	agents := p.store.List()
	for _, agent := range agents {
		if agent.AgentID == "" {
			continue // no intermute correlation yet
		}
		meta := buildMetadata(agent)

		if err := p.patchMetadata(ctx, agent.AgentID, meta); err != nil {
			log.Printf("intermux: push metadata for %s: %v", agent.AgentID, err)
		}
	}
}

func buildMetadata(agent activity.AgentActivity) map[string]string {
	meta := map[string]string{
		"tmux_session":     agent.TmuxSession,
		"agent_kind":       agent.AgentType,
		"repo":             agent.ProjectDir,
		"cwd":              agent.CWD,
		"git_branch":       agent.GitBranch,
		"status":           string(agent.Status),
		"last_activity":    agent.LastOutput,
		"last_activity_at": agent.LastSeen.Format(time.RFC3339),
		"last_seen":        agent.LastSeen.Format(time.RFC3339),
		"active_beads":     stringSliceJSON(agent.ActiveBeads),
		"files_touched":    stringSliceJSON(agent.FilesTouched),
	}
	if agent.Project != "" {
		meta["project"] = agent.Project
	}

	activeBeadID, confidence := resolveActiveBead(agent)
	meta["active_bead_id"] = activeBeadID
	meta["thread_id"] = activeBeadID
	if confidence != "" {
		meta["active_bead_confidence"] = confidence
	}
	meta["active_bead_candidates"] = "[]"
	if activeBeadID == "" && len(agent.ActiveBeads) > 1 {
		meta["active_bead_candidates"] = stringSliceJSON(agent.ActiveBeads)
	}

	for key, value := range agent.Metadata {
		if _, exists := meta[key]; !exists {
			meta[key] = value
		}
	}

	return meta
}

func stringSliceJSON(values []string) string {
	if values == nil {
		values = []string{}
	}
	data, _ := json.Marshal(values)
	return string(data)
}

func resolveActiveBead(agent activity.AgentActivity) (string, string) {
	if reported := strings.TrimSpace(agent.Metadata["active_bead_id"]); reported != "" {
		confidence := strings.TrimSpace(agent.Metadata["active_bead_confidence"])
		if confidence == "" {
			confidence = "reported"
		}
		return reported, confidence
	}

	if agent.ActiveBeadID != "" {
		confidence := agent.ActiveBeadConfidence
		if confidence == "" {
			confidence = "observed"
		}
		return agent.ActiveBeadID, confidence
	}

	if len(agent.ActiveBeads) == 1 {
		return agent.ActiveBeads[0], "observed"
	}
	return "", "unknown"
}

func (p *Pusher) patchMetadata(ctx context.Context, agentID string, meta map[string]string) error {
	body, err := json.Marshal(map[string]any{"metadata": meta})
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	url := fmt.Sprintf("%s/api/agents/%s/metadata", p.baseURL, agentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
