package api

import (
	"net/http"
	"sort"
	"strings"

	"github.com/dhawalhost/flagura/pkg/domain"
)

// Middleware wraps an http.HandlerFunc with pre- or post-processing logic.
type Middleware func(http.HandlerFunc) http.HandlerFunc

// ChainMiddlewares wraps a handler with a sequence of middlewares in left-to-right order:
// ChainMiddlewares(handler, m1, m2) executes m1 -> m2 -> handler.
func ChainMiddlewares(handler http.HandlerFunc, middlewares ...Middleware) http.HandlerFunc {
	h := handler
	for i := len(middlewares) - 1; i >= 0; i-- {
		if middlewares[i] != nil {
			h = middlewares[i](h)
		}
	}
	return h
}

// MethodRouter dispatches HTTP requests based on the HTTP request method.
// If an unhandled method is received, it automatically returns HTTP 405 Method Not Allowed
// along with the standard RFC Allow header listing all supported methods.
type MethodRouter struct {
	routes map[string]http.HandlerFunc
}

// NewMethodRouter creates a new MethodRouter instance.
func NewMethodRouter() *MethodRouter {
	return &MethodRouter{
		routes: make(map[string]http.HandlerFunc),
	}
}

// Handle registers a handler for a specific HTTP method (e.g. GET, POST, PUT, DELETE).
func (mr *MethodRouter) Handle(method string, handler http.HandlerFunc) *MethodRouter {
	mr.routes[method] = handler
	return mr
}

// ServeHTTP implements http.Handler for MethodRouter.
func (mr *MethodRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h, ok := mr.routes[r.Method]; ok {
		h(w, r)
		return
	}

	methods := make([]string, 0, len(mr.routes))
	for m := range mr.routes {
		methods = append(methods, m)
	}
	sort.Strings(methods)

	w.Header().Set("Allow", strings.Join(methods, ", "))
	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

// Handler returns the MethodRouter wrapped as a standard http.HandlerFunc.
func (mr *MethodRouter) Handler() http.HandlerFunc {
	return mr.ServeHTTP
}

// handle registers a route pattern with an optional chain of middlewares on the server mux.
// Middlewares are applied in declaration order: handle(pattern, handler, m1, m2) executes m1 -> m2 -> handler.
func (s *Server) handle(pattern string, handler http.HandlerFunc, middlewares ...Middleware) {
	s.mux.HandleFunc(pattern, ChainMiddlewares(handler, middlewares...))
}

// handleMethods registers a route pattern that dispatches based on HTTP method, with optional middlewares.
func (s *Server) handleMethods(pattern string, methodHandlers map[string]http.HandlerFunc, middlewares ...Middleware) {
	mr := NewMethodRouter()
	for method, h := range methodHandlers {
		mr.Handle(method, h)
	}
	s.handle(pattern, mr.Handler(), middlewares...)
}

// RequireAdmin is a convenience middleware wrapping RequireRole with domain.RoleAdmin.
func (s *Server) RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return s.RequireRole(domain.RoleAdmin, next)
}
