// Package api is BINNY's client for the existing OpenBIN/OpenAPK backend.
// Every BINNY tool that reads a project or writes a report/highlight/rename
// goes through here. There are NO new backend endpoints — this only speaks to
// surfaces that already ship (verified against the controllers 2026-07-16..18).
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"openbin.ai/binny/internal/config"
)

// TokenSource yields a valid bearer token (implemented by auth.TokenSource).
type TokenSource interface {
	Token() (string, error)
}

// Client is a thin authenticated HTTP client over the backend.
type Client struct {
	ep   config.Endpoints
	toks TokenSource
	http *http.Client
}

// New builds a client. The 30-minute timeout covers large analysis-blob GETs
// on slow links; individual small calls return long before that.
func New(ep config.Endpoints, toks TokenSource) *Client {
	return &Client{ep: ep, toks: toks, http: &http.Client{Timeout: 30 * time.Minute}}
}

// APIError carries the backend status + body for callers (and the LLM) to see.
type APIError struct {
	Status int
	Body   string
	Method string
	Path   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s → %d: %s", e.Method, e.Path, e.Status, abbreviate(e.Body, 400))
}

// do performs a request with a fresh bearer token and retries 5xx/network
// errors up to 3 times (4xx is a caller error, returned immediately). `out`,
// when non-nil, is JSON-decoded from the response body.
func (c *Client) do(method, path string, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		token, err := c.toks.Token()
		if err != nil {
			return fmt.Errorf("auth: %w", err)
		}
		var rdr io.Reader
		if payload != nil {
			rdr = bytes.NewReader(payload)
		}
		req, _ := http.NewRequest(method, c.ep.APIBase+path, rdr)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
			continue
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if out != nil && len(respBody) > 0 {
				if err := json.Unmarshal(respBody, out); err != nil {
					return fmt.Errorf("decode %s %s: %w", method, path, err)
				}
			}
			return nil
		}
		apiErr := &APIError{Status: resp.StatusCode, Body: string(respBody), Method: method, Path: path}
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return apiErr // caller error — no retry
		}
		lastErr = apiErr
		time.Sleep(time.Duration(attempt) * 2 * time.Second)
	}
	return lastErr
}

// ListProjects returns the caller's projects (GET /api/projects).
func (c *Client) ListProjects() ([]Project, error) {
	var out []Project
	err := c.do("GET", "/api/projects", nil, &out)
	return out, err
}

// GetProject returns one project (GET /api/projects/{id}).
func (c *Client) GetProject(id string) (*Project, error) {
	var out Project
	if err := c.do("GET", "/api/projects/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// FileTree returns the project's file tree root (GET /api/projects/{id}/files).
func (c *Client) FileTree(id string) (*FileNode, error) {
	var out FileNode
	if err := c.do("GET", "/api/projects/"+id+"/files", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// FileContent returns decoded file text (GET /api/projects/{id}/file?path=).
func (c *Client) FileContent(id, path string) (*FileContent, error) {
	var out FileContent
	u := "/api/projects/" + id + "/file?path=" + url.QueryEscape(path)
	if err := c.do("GET", u, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// BinaryAnalysis fetches the full analysis blob. When the project carries a
// signed CloudFront URL (schema 2.0) it's fetched directly from there;
// otherwise the inline /binary-analysis endpoint is used. Both return the same
// snake_case worker JSON.
func (c *Client) BinaryAnalysis(p *Project) (*Analysis, error) {
	raw, err := c.BinaryAnalysisRaw(p)
	if err != nil {
		return nil, err
	}
	return ParseAnalysis(raw)
}

// BinaryAnalysisRaw fetches the analysis blob's raw JSON bytes (so callers can
// cache them on disk before parsing — the toolbox does, to avoid re-paying the
// CloudFront/API fetch on every session).
func (c *Client) BinaryAnalysisRaw(p *Project) ([]byte, error) {
	if p.AnalysisDownloadURL != "" {
		return c.getSignedBlob(p.AnalysisDownloadURL)
	}
	return c.getRawBody("GET", "/api/projects/"+p.ID+"/binary-analysis")
}

// ListSamples returns a project's EXTRA samples (GET /api/projects/{id}/samples).
// Empty for single-sample projects. Each READY sample carries a short-TTL
// signed URL for its analysis blob — list right before fetching.
func (c *Client) ListSamples(projectID string) ([]Sample, error) {
	var out []Sample
	if err := c.do("GET", "/api/projects/"+projectID+"/samples", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SampleAnalysisRaw fetches one extra sample's analysis blob — signed URL
// when present, else the inline /samples/{sid}/binary-analysis endpoint. Same
// snake_case worker JSON as the project's primary blob.
func (c *Client) SampleAnalysisRaw(projectID string, s *Sample) ([]byte, error) {
	if s.AnalysisDownloadURL != "" {
		return c.getSignedBlob(s.AnalysisDownloadURL)
	}
	return c.getRawBody("GET", "/api/projects/"+projectID+"/samples/"+s.ID+"/binary-analysis")
}

// ParseAnalysis decodes a raw worker JSON blob, surfacing a worker-side error
// marker as an error.
func ParseAnalysis(raw []byte) (*Analysis, error) {
	var a Analysis
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, fmt.Errorf("decode analysis blob: %w", err)
	}
	if a.Error != "" {
		return nil, fmt.Errorf("worker analysis error: %s", abbreviate(a.Error, 300))
	}
	return &a, nil
}

// getRawBody GETs a backend path with auth and returns the raw response body
// (for endpoints that return non-JSON-DTO bodies like the analysis blob).
func (c *Client) getRawBody(method, path string) ([]byte, error) {
	token, err := c.toks.Token()
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}
	req, _ := http.NewRequest(method, c.ep.APIBase+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{Status: resp.StatusCode, Body: string(b), Method: method, Path: path}
	}
	return b, nil
}

// getSignedBlob GETs a pre-signed CloudFront URL WITHOUT attaching our bearer
// token (the signature is the auth; sending Authorization can break it).
func (c *Client) getSignedBlob(signedURL string) ([]byte, error) {
	resp, err := c.http.Get(signedURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{Status: resp.StatusCode, Body: string(b), Method: "GET", Path: "<signed-analysis-url>"}
	}
	return b, nil
}

// Highlights returns the project's Highlights board.
func (c *Client) Highlights(id string) ([]Highlight, error) {
	var out []Highlight
	err := c.do("GET", "/api/projects/"+id+"/highlights", nil, &out)
	return out, err
}

// CreateHighlight adds a Highlights-board card (POST .../highlights).
func (c *Client) CreateHighlight(id string, req CreateHighlightRequest) (*Highlight, error) {
	var out Highlight
	if err := c.do("POST", "/api/projects/"+id+"/highlights", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetReport get-or-creates the project report (GET .../report).
func (c *Client) GetReport(id string) (*Report, error) {
	var out Report
	if err := c.do("GET", "/api/projects/"+id+"/report", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateReport writes the report title + sections (PUT .../report).
func (c *Client) UpdateReport(id string, req UpdateReportRequest) (*Report, error) {
	var out Report
	if err := c.do("PUT", "/api/projects/"+id+"/report", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ManualRename creates+applies a rename in one shot (POST .../renames/manual).
func (c *Client) ManualRename(id string, req ManualRenameRequest) (*Rename, error) {
	var out Rename
	if err := c.do("POST", "/api/projects/"+id+"/renames/manual", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func abbreviate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// Abbrev truncates s to max runes with an ellipsis (exported for callers that
// render tool activity).
func Abbrev(s string, max int) string { return abbreviate(s, max) }
