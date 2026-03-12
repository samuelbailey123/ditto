package server

import (
	"net/http"
	"strings"

	"github.com/samuelbailey123/ditto/internal/config"
)

// routeKind classifies how specific a route path match is.
// Higher values indicate greater specificity.
type routeKind int

const (
	kindWildcard     routeKind = 1
	kindParameterised routeKind = 2
	kindExact         routeKind = 3
)

// Router holds a set of routes and finds the best match for an incoming request.
type Router struct {
	routes []config.Route
}

// NewRouter creates a Router backed by the provided routes.
func NewRouter(routes []config.Route) *Router {
	return &Router{routes: routes}
}

// Match finds the most specific route that satisfies r and returns it together
// with any extracted path parameters. It returns (nil, nil) when no route
// matches.
//
// Specificity order (highest wins): exact path > parameterised path > wildcard.
// When specificity is equal the first declared route wins.
func (ro *Router) Match(r *http.Request) (*config.Route, map[string]string) {
	var (
		bestRoute  *config.Route
		bestParams map[string]string
		bestKind   routeKind
	)

	for i := range ro.routes {
		route := &ro.routes[i]

		if !strings.EqualFold(route.Method, r.Method) {
			continue
		}

		params, kind, ok := matchPath(route.Path, r.URL.Path)
		if !ok {
			continue
		}

		// Check optional RequestMatch constraints.
		if route.Match != nil {
			if !satisfiesMatch(route.Match, r, nil) {
				continue
			}
		}

		if kind > bestKind {
			bestKind = kind
			bestRoute = route
			bestParams = params
		}
	}

	return bestRoute, bestParams
}

// MatchWithBody is like Match but also evaluates body-based RequestMatch rules.
// It is used by the server after reading the request body.
func (ro *Router) MatchWithBody(r *http.Request, body []byte) (*config.Route, map[string]string) {
	var (
		bestRoute  *config.Route
		bestParams map[string]string
		bestKind   routeKind
	)

	for i := range ro.routes {
		route := &ro.routes[i]

		if !strings.EqualFold(route.Method, r.Method) {
			continue
		}

		params, kind, ok := matchPath(route.Path, r.URL.Path)
		if !ok {
			continue
		}

		if route.Match != nil {
			if !satisfiesMatch(route.Match, r, body) {
				continue
			}
		}

		if kind > bestKind {
			bestKind = kind
			bestRoute = route
			bestParams = params
		}
	}

	return bestRoute, bestParams
}

// matchPath attempts to match routePath against requestPath.
// It returns the extracted parameters, the route kind, and whether the match
// succeeded.
func matchPath(routePath, requestPath string) (map[string]string, routeKind, bool) {
	// Wildcard suffix: "/static/*" matches "/static/css/style.css".
	if strings.HasSuffix(routePath, "*") {
		prefix := strings.TrimSuffix(routePath, "*")
		if strings.HasPrefix(requestPath, prefix) {
			return map[string]string{}, kindWildcard, true
		}
		return nil, 0, false
	}

	routeSegs := splitPath(routePath)
	reqSegs := splitPath(requestPath)

	if len(routeSegs) != len(reqSegs) {
		return nil, 0, false
	}

	params := make(map[string]string)
	isParameterised := false

	for i, rseg := range routeSegs {
		if strings.HasPrefix(rseg, "{") && strings.HasSuffix(rseg, "}") {
			name := rseg[1 : len(rseg)-1]
			params[name] = reqSegs[i]
			isParameterised = true
			continue
		}
		if rseg != reqSegs[i] {
			return nil, 0, false
		}
	}

	if isParameterised {
		return params, kindParameterised, true
	}
	return params, kindExact, true
}

// satisfiesMatch checks the optional query, header, and body constraints.
// body may be nil when body matching is not required.
func satisfiesMatch(m *config.RequestMatch, r *http.Request, body []byte) bool {
	if len(m.Query) > 0 && !matchQuery(m.Query, r.URL.Query()) {
		return false
	}
	if len(m.Headers) > 0 && !matchHeaders(m.Headers, r.Header) {
		return false
	}
	if len(m.Body) > 0 && !matchBody(m.Body, body) {
		return false
	}
	return true
}

// splitPath splits a URL path into non-empty segments.
func splitPath(p string) []string {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	// Filter empty segments that result from a root path "/".
	out := parts[:0]
	for _, s := range parts {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
