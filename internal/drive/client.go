package drive

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// Scope is the only permission the client ever asks for.
	Scope = "https://www.googleapis.com/auth/drive.readonly"

	defaultBase = "https://www.googleapis.com/drive/v3"
	attempts    = 3 // how many times a request for metadata is tried
)

// Client talks to Google Drive on behalf of a Service Account.
type Client struct {
	key  *Key
	http *http.Client
	base string

	// Replaced in tests.
	now   func() time.Time
	sleep func(context.Context, time.Duration)

	mu      sync.Mutex
	token   string
	expires time.Time
}

// New returns a client for the account of the key.
func New(key *Key) *Client {
	return &Client{
		key:  key,
		http: &http.Client{},
		base: defaultBase,
		now:  time.Now,
		sleep: func(ctx context.Context, d time.Duration) {
			select {
			case <-ctx.Done():
			case <-time.After(d):
			}
		},
	}
}

// Email is the address a Drive folder has to be shared with.
func (c *Client) Email() string { return c.key.Email }

// APIError is a refusal from Google: a status and, where given, the reason
// Drive names for it, such as "fileNotDownloadable".
type APIError struct {
	Status  int
	Reason  string
	Message string
}

func (e *APIError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("Google Drive: %s (%d %s)", e.Message, e.Status, e.Reason)
	}
	return fmt.Sprintf("Google Drive: %s (%d)", e.Message, e.Status)
}

// temporary tells whether the same request may succeed if tried again.
func (e *APIError) temporary() bool {
	return e.Status == http.StatusTooManyRequests || e.Status >= 500 ||
		e.Reason == "rateLimitExceeded" || e.Reason == "userRateLimitExceeded"
}

// accessToken returns a token that is good for at least another minute,
// signing in again when the current one is about to run out.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && c.now().Before(c.expires.Add(-time.Minute)) {
		return c.token, nil
	}

	// The sign-in is a short-lived statement "I am this account and I want
	// read-only access", signed with the account's private key.
	enc := base64.RawURLEncoding.EncodeToString
	now := c.now().Unix()
	claims, _ := json.Marshal(map[string]any{
		"iss": c.key.Email, "scope": Scope, "aud": c.key.tokenURI, "iat": now, "exp": now + 3600,
	})
	unsigned := enc([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + enc(claims)
	sum := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, c.key.private, crypto.SHA256, sum[:])
	if err != nil {
		return "", errors.New("signing in: the key could not sign the request")
	}
	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {unsigned + "." + enc(sig)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.key.tokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("signing in: %w", err)
	}
	defer resp.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	json.Unmarshal(body, &out)
	if resp.StatusCode != http.StatusOK || out.AccessToken == "" {
		message := out.Description
		if message == "" {
			message = "sign-in was refused"
		}
		return "", &APIError{Status: resp.StatusCode, Reason: out.Error, Message: message}
	}
	c.token = out.AccessToken
	c.expires = c.now().Add(time.Duration(out.ExpiresIn) * time.Second)
	return c.token, nil
}

// open sends one request and returns the response if Drive accepted it.
// The caller closes the body.
func (c *Client) open(ctx context.Context, path string, query url.Values, header http.Header) (*http.Response, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	u := c.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	for name, values := range header {
		req.Header[name] = values
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent {
		return resp, nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return nil, apiError(resp.StatusCode, body)
}

// apiError reads the error Drive sent: its message and the reason code.
func apiError(status int, body []byte) *APIError {
	var raw struct {
		Error struct {
			Message string `json:"message"`
			Errors  []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"error"`
	}
	json.Unmarshal(body, &raw)
	e := &APIError{Status: status, Message: raw.Error.Message}
	if len(raw.Error.Errors) > 0 {
		e.Reason = raw.Error.Errors[0].Reason
	}
	if e.Message == "" {
		e.Message = http.StatusText(status)
	}
	return e
}

// getJSON asks for metadata and decodes the answer. Such requests are small
// and safe to repeat, so a failure that looks temporary is tried again with
// a growing pause.
func (c *Client) getJSON(ctx context.Context, path string, query url.Values, into any) error {
	var last error
	for try := range attempts {
		if try > 0 {
			c.sleep(ctx, time.Duration(try*try)*time.Second)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		resp, err := c.open(ctx, path, query, nil)
		if err == nil {
			err = json.NewDecoder(resp.Body).Decode(into)
			resp.Body.Close()
			if err == nil {
				return nil
			}
		}
		last = err
		var refused *APIError
		if errors.As(err, &refused) && !refused.temporary() {
			return err
		}
	}
	return last
}
