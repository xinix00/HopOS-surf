package browse

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

// cacert.pem is Mozilla's root-CA-bundel (via https://curl.se/ca/cacert.pem,
// MPL-2.0). Bare metal heeft geen certificaatwinkel.
//
//go:embed cacert.pem
var cacertPEM []byte

// Elke browser-Session krijgt een eigen cookie-jar. De concurrency-safe
// transport en zijn connection pool mogen gedeeld worden; cookies en
// daarmee login/consent-status niet.
var sharedNetTransport = netTransport()

func newNetClient() *http.Client {
	return &http.Client{
		Timeout:   20 * time.Second,
		Transport: sharedNetTransport,
		Jar:       newJar(),
	}
}

func newJar() http.CookieJar {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil
	}
	return jar
}

func netTransport() http.RoundTripper {
	t := http.DefaultTransport.(*http.Transport).Clone()
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	pool.AppendCertsFromPEM(cacertPEM)
	t.TLSClientConfig = &tls.Config{RootCAs: pool}
	return t
}

// handlerTransport laat tests de volledige HTTP-keten zonder socket lopen.
type handlerTransport struct{ h http.Handler }

func (t handlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := &recorder{hdr: http.Header{}, code: http.StatusOK}
	t.h.ServeHTTP(rec, req)
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", rec.code, http.StatusText(rec.code)),
		StatusCode:    rec.code,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        rec.hdr,
		Body:          io.NopCloser(bytes.NewReader(rec.buf.Bytes())),
		ContentLength: int64(rec.buf.Len()),
		Request:       req,
	}, nil
}

type recorder struct {
	hdr  http.Header
	code int
	buf  bytes.Buffer
}

func (r *recorder) Header() http.Header         { return r.hdr }
func (r *recorder) WriteHeader(c int)           { r.code = c }
func (r *recorder) Write(b []byte) (int, error) { return r.buf.Write(b) }

// load haalt en parset één HTML-document. final is de URL na redirects.
func (s *Session) load(u *url.URL) (doc *html.Node, final *url.URL, err error) {
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, nil, fmt.Errorf("HTTP %s  %s", resp.Status, u)
	}
	body := io.LimitReader(resp.Body, pageMaxBytes)
	r, err := charset.NewReader(body, resp.Header.Get("Content-Type"))
	if err != nil {
		r = body
	}
	doc, err = html.Parse(r)
	if err != nil {
		return nil, nil, err
	}
	final = u
	if resp.Request != nil && resp.Request.URL != nil {
		final = resp.Request.URL
	}
	return doc, final, nil
}

func (s *Session) get(u string) (*http.Response, error) {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	return s.client.Do(req)
}

type resourceData struct {
	data        []byte
	contentType string
}

// resetResourceCache begrenst de levensduur van subresourcebytes tot één
// document. Navigatie is binnen Session serieel en alle loaders zijn
// synchroon klaar voordat een volgende navigatie kan beginnen.
func (s *Session) resetResourceCache() {
	s.resourceMu.Lock()
	s.resources = nil
	s.resourceWait = nil
	s.resourceMu.Unlock()
}

// fetchResource is de ene begrensde byte-fetcher voor CSS, SVG en beelden.
// Absolute URL's worden per Session gecachet; gelijktijdige vragers delen
// dezelfde request.
func (s *Session) fetchResource(ref string, maxBytes int64) ([]byte, string, error) {
	u, err := s.resolve(ref)
	if err != nil {
		return nil, "", err
	}
	key := u.String()

	s.resourceMu.Lock()
	if hit, ok := s.resources[key]; ok {
		s.resourceMu.Unlock()
		return hit.data, hit.contentType, nil
	}
	if wait := s.resourceWait[key]; wait != nil {
		s.resourceMu.Unlock()
		<-wait
		s.resourceMu.Lock()
		hit, ok := s.resources[key]
		s.resourceMu.Unlock()
		if !ok {
			return nil, "", fmt.Errorf("subresource niet geladen: %s", key)
		}
		return hit.data, hit.contentType, nil
	}
	if s.resourceWait == nil {
		s.resourceWait = map[string]chan struct{}{}
	}
	wait := make(chan struct{})
	s.resourceWait[key] = wait
	s.resourceMu.Unlock()

	var got resourceData
	resp, fetchErr := s.get(key)
	if fetchErr == nil {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			fetchErr = fmt.Errorf("HTTP %s  %s", resp.Status, key)
		} else {
			got.data, fetchErr = io.ReadAll(io.LimitReader(resp.Body, maxBytes))
			got.contentType = resp.Header.Get("Content-Type")
		}
	}

	s.resourceMu.Lock()
	if fetchErr == nil {
		if s.resources == nil {
			s.resources = map[string]resourceData{}
		}
		s.resources[key] = got
	}
	delete(s.resourceWait, key)
	close(wait)
	s.resourceMu.Unlock()
	return got.data, got.contentType, fetchErr
}
