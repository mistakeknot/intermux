package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/mistakeknot/intermux/internal/activity"
)

// Pusher periodically pushes agent metadata to intermute.
type Pusher struct {
	store      *activity.Store
	baseURL    string
	interval   time.Duration
	httpClient *http.Client
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

// Run starts the push loop. Blocks until context is cancelled.
func (p *Pusher) Run(ctx context.Context) {
	if p.baseURL == "" {
		log.Printf("intermux: metadata push disabled (no INTERMUTE_URL)")
		return
	}
	log.Printf("intermux: metadata pusher started (url=%s, interval=%s)", p.baseURL, p.interval)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("intermux: metadata pusher stopped")
			return
		case <-ticker.C:
			p.push(ctx)
		}
	}
}

func (p *Pusher) push(ctx context.Context) {
	agents := p.store.List()
	for _, agent := range agents {
		if agent.AgentID == "" {
			continue // no intermute correlation yet
		}
		meta := map[string]string{
			"tmux_session":    agent.TmuxSession,
			"cwd":             agent.CWD,
			"git_branch":      agent.GitBranch,
			"status":          string(agent.Status),
			"last_activity":   agent.LastOutput,
			"last_activity_at": agent.LastSeen.Format(time.RFC3339),
		}
		if len(agent.ActiveBeads) > 0 {
			beadsJSON, _ := json.Marshal(agent.ActiveBeads)
			meta["active_beads"] = string(beadsJSON)
		}

		if err := p.patchMetadata(ctx, agent.AgentID, meta); err != nil {
			log.Printf("intermux: push metadata for %s: %v", agent.AgentID, err)
		}
	}
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
