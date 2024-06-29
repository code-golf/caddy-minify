package caddyminify

import (
	"bytes"
	"net/http"
	"sync"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/json"
	"github.com/tdewolff/minify/v2/svg"
)

// Interface guards
var (
	_ caddy.Module                = (*Middleware)(nil)
	_ caddy.Provisioner           = (*Middleware)(nil)
	_ caddyhttp.MiddlewareHandler = (*Middleware)(nil)
	_ caddyfile.Unmarshaler       = (*Middleware)(nil)
)

type Middleware struct{ minify *minify.M }

func init() {
	caddy.RegisterModule(new(Middleware))
	httpcaddyfile.RegisterHandlerDirective("minify", setup)
}

func setup(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	m := new(Middleware)
	return m, m.UnmarshalCaddyfile(h.Dispenser)
}

func (Middleware) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.minify",
		New: func() caddy.Module { return new(Middleware) },
	}
}

func (m *Middleware) Provision(caddy.Context) error {
	m.minify = minify.New()

	m.minify.AddFunc("text/html", html.Minify)
	m.minify.Add("application/json", &json.Minifier{KeepNumbers: true})
	m.minify.AddFunc("image/svg+xml", svg.Minify)

	return nil
}

var pool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

func (m Middleware) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	// Get a buffer to hold the response body.
	buf := pool.Get().(*bytes.Buffer)
	buf.Reset()
	defer pool.Put(buf)

	// Set up the response recorder.
	rec := caddyhttp.NewResponseRecorder(w, buf,
		func(int, http.Header) bool { return true })

	// Collect the response from upstream.
	if err := next.ServeHTTP(rec, r); err != nil {
		return err
	}

	// Early-exit if the content type isn't a match.
	_, params, minifier := m.minify.Match(rec.Header().Get("Content-Type"))
	if minifier == nil {
		return rec.WriteResponse()
	}

	// Minify the body.
	w.Header().Del("Content-Length")
	w.WriteHeader(rec.Status())
	return minifier(m.minify, w, buf, params)
}

func (Middleware) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	d.Next() // Consume directive name.

	// There should be no more arguments.
	if d.NextArg() {
		return d.ArgErr()
	}

	return nil
}
