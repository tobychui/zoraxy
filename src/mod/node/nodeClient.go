package node

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/access"
	"imuslab.com/zoraxy/mod/tlscert"
)

type ProxyConfigSnapshot struct {
	ConfigVersion       string            `json:"config_version"`
	PrimaryVersion      string            `json:"primary_version,omitempty"`
	RequireVersionMatch bool              `json:"require_version_match"`
	RootConfig          json.RawMessage   `json:"root_config,omitempty"`
	ProxyConfigs        []json.RawMessage `json:"proxy_configs"`
}

type AccessSnapshot struct {
	ConfigVersion  string                `json:"config_version"`
	AccessRules    []*access.AccessRule  `json:"access_rules"`
	TrustedProxies []access.TrustedProxy `json:"trusted_proxies"`
}

type CertificateSnapshot struct {
	ConfigVersion string                      `json:"config_version"`
	Certificates  []tlscert.StoredCertificate `json:"certificates"`
}

type SystemSnapshot struct {
	ConfigVersion  string            `json:"config_version"`
	ServiceEnabled bool              `json:"service_enabled"`
	DatabaseBackup json.RawMessage   `json:"database_backup"`
	StreamConfigs  []json.RawMessage `json:"stream_configs,omitempty"`
	RedirectRules  []json.RawMessage `json:"redirect_rules,omitempty"`
}

type NodeClient struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

func NewNodeClient(baseURL string, token string, timeout time.Duration) *NodeClient {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &NodeClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		Client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
}

func (c *NodeClient) resolveURL(path string) (string, error) {
	return url.JoinPath(c.BaseURL, strings.TrimPrefix(path, "/"))
}

func (c *NodeClient) newRequest(method string, path string, body io.Reader, contentType string) (*http.Request, error) {
	targetURL, err := c.resolveURL(path)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, targetURL, body)
	if err != nil {
		return nil, err
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "zoraxy-node-sync")

	return req, nil
}

func (c *NodeClient) doJSON(method string, path string, body io.Reader, contentType string, target interface{}) error {
	req, err := c.newRequest(method, path, body, contentType)
	if err != nil {
		return err
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("node request failed: %s", strings.TrimSpace(string(respBody)))
	}

	if target == nil {
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

func (c *NodeClient) UpdateNodeInfo(path string, hostname string, nodeName string, nodeIP string, managementPort string, zoraxyVersion string, configVersion string, localOverride bool, streamRuntime map[string]*StreamProxyRuntime) error {
	form := url.Values{}
	form.Set("hostname", hostname)
	form.Set("node_name", nodeName)
	if strings.TrimSpace(nodeIP) != "" {
		form.Set("node_ip", strings.TrimSpace(nodeIP))
	}
	form.Set("management_port", managementPort)
	form.Set("zoraxy_version", zoraxyVersion)
	form.Set("config_version", configVersion)
	form.Set("local_override", fmt.Sprintf("%t", localOverride))
	if streamRuntime != nil {
		payload, err := json.Marshal(streamRuntime)
		if err != nil {
			return err
		}
		form.Set("stream_proxy_runtime", string(payload))
	}

	return c.doJSON(http.MethodPost, path, strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", nil)
}

func (c *NodeClient) FetchProxyConfigs(path string) (*ProxyConfigSnapshot, error) {
	results := &ProxyConfigSnapshot{}
	if err := c.doJSON(http.MethodGet, path, nil, "", results); err != nil {
		return nil, err
	}
	return results, nil
}

func (c *NodeClient) FetchAccessRules(path string) (*AccessSnapshot, error) {
	results := &AccessSnapshot{}
	if err := c.doJSON(http.MethodGet, path, nil, "", results); err != nil {
		return nil, err
	}
	return results, nil
}

func (c *NodeClient) FetchCertificates(path string) (*CertificateSnapshot, error) {
	results := &CertificateSnapshot{}
	if err := c.doJSON(http.MethodGet, path, nil, "", results); err != nil {
		return nil, err
	}
	return results, nil
}

func (c *NodeClient) FetchSystemData(path string) (*SystemSnapshot, error) {
	results := &SystemSnapshot{}
	if err := c.doJSON(http.MethodGet, path, nil, "", results); err != nil {
		return nil, err
	}
	return results, nil
}

func (c *NodeClient) SendTelemetry(path string, telemetry *TelemetrySnapshot) error {
	if telemetry == nil {
		return nil
	}

	payload, err := json.Marshal(telemetry)
	if err != nil {
		return err
	}

	return c.doJSON(http.MethodPost, path, strings.NewReader(string(payload)), "application/json", nil)
}
