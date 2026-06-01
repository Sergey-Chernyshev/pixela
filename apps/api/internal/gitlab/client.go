// Package gitlab is a tiny client for the two GitLab REST calls Pixela's Mode A workflow needs:
// committing an approved baseline PNG back to the repo (git-native baseline, invariant #1) and mirroring
// a build's review state to the commit/MR (Phase 5b/5c). It is intentionally minimal — no SDK — and is
// only used when a project is wired to a GitLab repo and a token is configured; callers no-op otherwise.
package gitlab

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Action is a GitLab Commits-API file action, chosen from the snapshot's status by the caller.
type Action string

const (
	ActionCreate Action = "create" // NEW snapshot — baseline file does not exist yet
	ActionUpdate Action = "update" // CHANGED snapshot — baseline file exists, move it
	ActionDelete Action = "delete" // REMOVED snapshot — accept the deletion, drop the file
)

// CommitState is a GitLab commit-status state.
type CommitState string

const (
	StatePending CommitState = "pending"
	StateSuccess CommitState = "success"
	StateFailed  CommitState = "failed"
)

// Client talks to one GitLab instance with a single token (a project/personal access token with
// api + write_repository scope).
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// New builds a client. baseURL defaults to https://gitlab.com; an empty token yields an "unconfigured"
// client — call Enabled() to skip GitLab work cleanly when it returns false.
func New(baseURL, token string) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://gitlab.com"
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   strings.TrimSpace(token),
		http:    &http.Client{Timeout: 20 * time.Second},
	}
}

// Enabled reports whether a token is configured (so callers no-op git-native actions when it is not).
func (c *Client) Enabled() bool { return c != nil && c.token != "" }

// CommitFile creates/updates/deletes a single file on a branch in one commit (GitLab Commits API).
// content is the raw bytes (base64-encoded on the wire); ignored for ActionDelete.
func (c *Client) CommitFile(ctx context.Context, projectID, branch string, action Action, filePath string, content []byte, message, authorName, authorEmail string) error {
	fileAction := map[string]any{"action": string(action), "file_path": filePath}
	if action != ActionDelete {
		fileAction["content"] = base64.StdEncoding.EncodeToString(content)
		fileAction["encoding"] = "base64"
	}
	body := map[string]any{
		"branch":         branch,
		"commit_message": message,
		"author_name":    authorName,
		"author_email":   authorEmail,
		"actions":        []map[string]any{fileAction},
	}
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/repository/commits", c.baseURL, url.PathEscape(projectID))
	return c.post(ctx, endpoint, body)
}

// SetCommitStatus mirrors a build's review state onto its commit (shows up on the MR pipeline widget).
func (c *Client) SetCommitStatus(ctx context.Context, projectID, sha string, state CommitState, name, targetURL, description string) error {
	body := map[string]any{"state": string(state), "name": name, "description": description}
	if targetURL != "" {
		body["target_url"] = targetURL
	}
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/statuses/%s", c.baseURL, url.PathEscape(projectID), url.PathEscape(sha))
	return c.post(ctx, endpoint, body)
}

func (c *Client) post(ctx context.Context, endpoint string, body map[string]any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("gitlab request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("gitlab %s → %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(snippet)))
}
