package drive

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fake is a stand-in for Google: a sign-in address and a Drive address.
type fake struct {
	t       *testing.T
	server  *httptest.Server
	private *rsa.PrivateKey
	signIns atomic.Int32
	files   func(w http.ResponseWriter, r *http.Request) // answers /files and below
}

const fakeToken = "token-for-tests"

func newFake(t *testing.T) *fake {
	t.Helper()
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fake{t: t, private: private}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", f.signIn)
	mux.HandleFunc("/drive/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fakeToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.files(w, r)
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

// signIn checks that the request is a statement signed by the account's key
// and asks for read-only access and nothing more.
func (f *fake) signIn(w http.ResponseWriter, r *http.Request) {
	f.signIns.Add(1)
	r.ParseForm()
	parts := strings.Split(r.Form.Get("assertion"), ".")
	if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" || len(parts) != 3 {
		f.t.Errorf("sign-in: unexpected form %v", r.Form)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&f.private.PublicKey, crypto.SHA256, sum[:], sig); err != nil {
		f.t.Errorf("sign-in: the signature does not match the key")
	}
	var claims map[string]any
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	json.Unmarshal(raw, &claims)
	if claims["scope"] != Scope || claims["iss"] != "robot@example.iam.gserviceaccount.com" || claims["aud"] != f.server.URL+"/token" {
		f.t.Errorf("sign-in: unexpected claims %v", claims)
	}
	fmt.Fprintf(w, `{"access_token":%q,"expires_in":3600}`, fakeToken)
}

// keyFile is the JSON Google Cloud would give for the fake account.
func (f *fake) keyFile() []byte {
	der, _ := x509.MarshalPKCS8PrivateKey(f.private)
	block := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	data, _ := json.Marshal(map[string]string{
		"type": "service_account", "client_email": "robot@example.iam.gserviceaccount.com",
		"private_key": string(block), "token_uri": f.server.URL + "/token",
	})
	return data
}

func (f *fake) client() *Client {
	f.t.Helper()
	key, err := ParseKey(f.keyFile())
	if err != nil {
		f.t.Fatal(err)
	}
	c := New(key)
	c.base = f.server.URL + "/drive"
	c.sleep = func(context.Context, time.Duration) {} // tests do not wait
	return c
}

func TestParseKey(t *testing.T) {
	f := newFake(t)
	key, err := ParseKey(f.keyFile())
	if err != nil || key.Email != "robot@example.iam.gserviceaccount.com" {
		t.Fatalf("ParseKey = %v, %v", key, err)
	}
	bad := map[string]string{
		"not json":             "{",
		"another kind of key":  `{"type":"authorized_user","client_email":"a","token_uri":"b","private_key":"c"}`,
		"no email":             `{"type":"service_account","token_uri":"b","private_key":"c"}`,
		"no private key":       `{"type":"service_account","client_email":"a","token_uri":"b","private_key":"nonsense"}`,
		"a broken private key": `{"type":"service_account","client_email":"a","token_uri":"b","private_key":"-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----\n"}`,
	}
	for name, data := range bad {
		_, err := ParseKey([]byte(data))
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		// An error must not carry the content of the file.
		if strings.Contains(err.Error(), "nonsense") || strings.Contains(err.Error(), "AAAA") {
			t.Errorf("%s: the error quotes the key file: %v", name, err)
		}
	}
}

func TestListFollowsPagesAndSignsInOnce(t *testing.T) {
	f := newFake(t)
	var requests []string
	f.files = func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		requests = append(requests, q.Get("pageToken"))
		if q.Get("q") != "trashed = false" || q.Get("pageSize") != "1000" {
			t.Errorf("unexpected query %v", q)
		}
		switch q.Get("pageToken") {
		case "":
			fmt.Fprint(w, `{"nextPageToken":"second","files":[
				{"id":"root","name":"Syncer","mimeType":"application/vnd.google-apps.folder","createdTime":"2026-01-02T03:04:05.000Z","modifiedTime":"2026-02-03T04:05:06.000Z"},
				{"id":"f1","name":"звіт.pdf","mimeType":"application/pdf","size":"1331514","md5Checksum":"e09e","parents":["root"],"createdTime":"2026-10-21T20:14:08.256Z","modifiedTime":"2026-10-21T20:14:08.256Z","capabilities":{"canDownload":true}}
			]}`)
		case "second":
			fmt.Fprint(w, `{"files":[
				{"id":"d1","name":"Бюджет","mimeType":"application/vnd.google-apps.spreadsheet","size":"7317","parents":["root"]},
				{"id":"s1","name":"ярлик","mimeType":"application/vnd.google-apps.shortcut","parents":["root"]},
				{"id":"f2","name":"закрите.mov","mimeType":"video/quicktime","size":"5","parents":["root"],"capabilities":{"canDownload":false}}
			]}`)
		default:
			t.Errorf("unexpected page %q", q.Get("pageToken"))
		}
	}
	c := f.client()
	files, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 5 || len(requests) != 2 {
		t.Fatalf("%d files in %d requests, want 5 in 2", len(files), len(requests))
	}
	root, pdf, sheet, shortcut, locked := files[0], files[1], files[2], files[3], files[4]
	if !root.IsFolder() || root.Parent != "" || root.Modified.Format("2006-01-02") != "2026-02-03" {
		t.Errorf("the shared folder: %+v", root)
	}
	if pdf.Size != 1331514 || pdf.MD5 != "e09e" || pdf.Parent != "root" || !pdf.CanDownload || pdf.GoogleType() != "" {
		t.Errorf("the ordinary file: %+v", pdf)
	}
	if sheet.GoogleType() != "spreadsheet" || sheet.MD5 != "" || !sheet.CanDownload {
		t.Errorf("the Google document: %+v", sheet)
	}
	if !shortcut.IsShortcut() || shortcut.GoogleType() != "" {
		t.Errorf("the shortcut: %+v", shortcut)
	}
	if locked.CanDownload {
		t.Error("a file with downloading forbidden is reported as downloadable")
	}

	// The token is kept: a second list does not sign in again.
	if _, err := c.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := f.signIns.Load(); n != 1 {
		t.Errorf("signed in %d times, want once", n)
	}
	// It is renewed a minute before it runs out.
	c.now = func() time.Time { return time.Now().Add(59*time.Minute + 30*time.Second) }
	if _, err := c.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := f.signIns.Load(); n != 2 {
		t.Errorf("signed in %d times after the token ran out, want twice", n)
	}
}

func TestTemporaryFailuresAreRetried(t *testing.T) {
	f := newFake(t)
	calls := 0
	f.files = func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, `{"files":[]}`)
	}
	if _, err := f.client().List(context.Background()); err != nil || calls != 3 {
		t.Fatalf("err = %v after %d calls, want success on the third", err, calls)
	}

	// A refusal that will not change is reported at once, with its reason.
	calls = 0
	f.files = func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"message":"Drive API has not been used in this project","errors":[{"reason":"accessNotConfigured"}]}}`)
	}
	_, err := f.client().List(context.Background())
	var refused *APIError
	if !errors.As(err, &refused) || refused.Reason != "accessNotConfigured" || refused.Status != 403 || calls != 1 {
		t.Fatalf("err = %v after %d calls, want accessNotConfigured after one", err, calls)
	}

	// Three temporary failures in a row give up.
	calls = 0
	f.files = func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusTooManyRequests)
	}
	if _, err := f.client().List(context.Background()); err == nil || calls != attempts {
		t.Fatalf("err = %v after %d calls, want a failure after %d", err, calls, attempts)
	}
}

func TestSignInRefusalIsReported(t *testing.T) {
	f := newFake(t)
	key, _ := ParseKey(f.keyFile())
	key.tokenURI = f.server.URL + "/drive/refuse"
	f.files = func(w http.ResponseWriter, r *http.Request) {}
	c := New(key)
	c.http = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		rec.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(rec, `{"error":"invalid_grant","error_description":"Invalid JWT Signature."}`)
		return rec.Result(), nil
	})}
	_, err := c.List(context.Background())
	var refused *APIError
	if !errors.As(err, &refused) || refused.Reason != "invalid_grant" {
		t.Fatalf("err = %v, want invalid_grant", err)
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
