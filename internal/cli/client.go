package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
)

const defaultSocketPath = "/var/run/apod.sock"

type Client struct {
	http       *http.Client
	baseURL    string
	apiKey     string
	socketPath string
}

func NewClient(remote, apiKey string) *Client {
	if remote != "" {
		return &Client{
			http:    http.DefaultClient,
			baseURL: remote,
			apiKey:  apiKey,
		}
	}

	return &Client{
		http: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", defaultSocketPath)
				},
			},
		},
		baseURL:    "http://apod",
		socketPath: defaultSocketPath,
	}
}

type apiResponse struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

func (c *Client) do(method, path string, body interface{}) (*apiResponse, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed (is apod server running?): %w", err)
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if !apiResp.OK {
		return nil, fmt.Errorf("%s", apiResp.Error)
	}

	return &apiResp, nil
}

func (c *Client) Get(path string) (*apiResponse, error) {
	return c.do("GET", path, nil)
}

func (c *Client) Post(path string, body interface{}) (*apiResponse, error) {
	return c.do("POST", path, body)
}

func (c *Client) Delete(path string) (*apiResponse, error) {
	return c.do("DELETE", path, nil)
}
