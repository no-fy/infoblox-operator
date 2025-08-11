package infoblox

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a minimal Infoblox WAPI client for managing record:host resources.
type Client struct {
	baseURL   string
	username  string
	password  string
	http      *http.Client
	userAgent string
}

// NewClient constructs a new Client.
func NewClient(baseURL, username, password string, insecureTLS bool) *Client {
	transport := &http.Transport{}
	if insecureTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // nolint:gosec
	}
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/") + "/",
		username: username,
		password: password,
		http: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
		userAgent: "infoblox-operator/0.1",
	}
}

// RecordHost represents the minimal fields we care about from Infoblox record:host.
type RecordHost struct {
	Ref       string              `json:"_ref,omitempty"`
	Name      string              `json:"name,omitempty"`
	View      string              `json:"view,omitempty"`
	TTL       *int                `json:"ttl,omitempty"`
	IPv4Addrs []IPv4Addr          `json:"ipv4addrs,omitempty"`
	ExtAttrs  map[string]EAObject `json:"extattrs,omitempty"`
}

type IPv4Addr struct {
	IPv4Addr string `json:"ipv4addr,omitempty"`
}

// EAObject matches Infoblox EA value envelope.
type EAObject struct {
	Value string `json:"value"`
}

// GetHostRecord fetches a host record by name and view. Returns (nil, nil) if not found.
func (c *Client) GetHostRecord(ctx context.Context, fqdn, view string) (*RecordHost, error) {
	// Example: GET /record:host?name=host.example.com&view=default
	q := url.Values{}
	q.Set("name", fqdn)
	if view != "" {
		q.Set("view", view)
	}
	endpoint := c.baseURL + "record:host?" + q.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	c.applyAuthHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("infoblox get host failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	var list []RecordHost
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	// Infoblox may return multiple if duplicates exist; pick the first.
	return &list[0], nil
}

// CreateHostRecord creates a host record with either a fixed IP or next available IP allocation.
// If nextAvailable is non-empty, it must be of the form "func:nextavailableip:<target>", where <target>
// can be a network CIDR (and optional view) or a network reference. When ip is provided, nextAvailable is ignored.
func (c *Client) CreateHostRecord(ctx context.Context, name, view string, ttl *int, ip, nextAvailable string, extattrs map[string]string) (string, string, error) {
	payload := map[string]any{
		"name": name,
	}
	if view != "" {
		payload["view"] = view
	}
	if ttl != nil {
		payload["ttl"] = *ttl
	}
	// extattrs conversion
	if len(extattrs) > 0 {
		ea := make(map[string]EAObject, len(extattrs))
		for k, v := range extattrs {
			ea[k] = EAObject{Value: v}
		}
		payload["extattrs"] = ea
	}
	if ip != "" {
		payload["ipv4addrs"] = []IPv4Addr{{IPv4Addr: ip}}
	} else if nextAvailable != "" {
		if !strings.HasPrefix(nextAvailable, "func:nextavailableip:") {
			nextAvailable = "func:nextavailableip:" + nextAvailable
		}
		payload["ipv4addrs"] = []IPv4Addr{{IPv4Addr: nextAvailable}}
	} else {
		return "", "", errors.New("either ip or nextAvailable must be provided")
	}

	endpoint := c.baseURL + "record:host"
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(b)))
	c.applyAuthHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("infoblox create host failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	// Response is the object reference string
	var ref string
	if err := json.NewDecoder(resp.Body).Decode(&ref); err != nil {
		return "", "", err
	}

	// Fetch to get the allocated IP
	obj, err := c.GetByRef(ctx, ref)
	if err != nil {
		return ref, "", nil // return ref anyway
	}
	ipAddr := ""
	if obj != nil && len(obj.IPv4Addrs) > 0 {
		ipAddr = obj.IPv4Addrs[0].IPv4Addr
	}
	return ref, ipAddr, nil
}

// GetByRef fetches an object by Infoblox reference.
func (c *Client) GetByRef(ctx context.Context, ref string) (*RecordHost, error) {
	// Infoblox WAPI expects the object reference as a raw path segment, not URL-escaped as a whole.
	// The ref already contains the object type and ID separated by '/', e.g., "record:host/XYZ".
	// Escaping the '/' results in an invalid path like "record:host%2FXYZ" and a 400 error.
	endpoint := c.baseURL + ref
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	c.applyAuthHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("infoblox get by ref failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	var obj RecordHost
	if err := json.NewDecoder(resp.Body).Decode(&obj); err != nil {
		return nil, err
	}
	return &obj, nil
}

// DeleteByRef deletes an object by reference.
func (c *Client) DeleteByRef(ctx context.Context, ref string) error {
	// See note in GetByRef: do not URL-escape the entire reference
	endpoint := c.baseURL + ref
	req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	c.applyAuthHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("infoblox delete failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	return nil
}

func (c *Client) applyAuthHeaders(req *http.Request) {
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
}
