package ginertia

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
)

// A resolver is a plain value or one of these funcs, evaluated only when
// the prop is actually sent:
//
//	func() any
//	func() (any, error)
//	func(*gin.Context) any
//	func(*gin.Context) (any, error)
//
// Typed funcs like func() []User are accepted too (via reflection).

// OptionalProp is never sent on the first visit; only on a partial reload
// that asks for it (router.reload({ only: ["key"] })).
type OptionalProp struct{ value any }

// Optional wraps a resolver that is only evaluated when explicitly requested.
func Optional(resolver any) OptionalProp { return OptionalProp{resolver} }

// Lazy is an alias of Optional (Inertia v1 naming).
func Lazy(resolver any) OptionalProp { return Optional(resolver) }

// AlwaysProp is always sent, even on partial reloads that don't ask for it.
type AlwaysProp struct{ value any }

// Always wraps a value that is included in every response.
func Always(value any) AlwaysProp { return AlwaysProp{value} }

// DeferProp is excluded from the first response and fetched by the client
// right after the page renders, grouped so related props load together.
type DeferProp struct {
	value  any
	group  string
	merge  bool
	deep   bool
	onceOn bool
}

// Defer wraps a resolver loaded after the initial render. Optional group
// name (default "default"); props in the same group load in one request.
func Defer(resolver any, group ...string) DeferProp {
	g := "default"
	if len(group) > 0 && group[0] != "" {
		g = group[0]
	}
	return DeferProp{value: resolver, group: g}
}

// Merge makes the deferred prop merge into existing client data.
func (p DeferProp) Merge() DeferProp { p.merge = true; return p }

// DeepMerge makes the deferred prop deep-merge into existing client data.
func (p DeferProp) DeepMerge() DeferProp { p.merge, p.deep = true, true; return p }

// MergeProp is appended to (instead of replacing) the client's existing
// value on partial reloads — handy for "load more" pagination.
type MergeProp struct {
	value   any
	deep    bool
	prepend bool
	matchOn []string
}

// Merge appends the value to the existing array / shallow-merges the object.
func Merge(value any) MergeProp { return MergeProp{value: value} }

// DeepMerge deep-merges the value into the existing client data.
func DeepMerge(value any) MergeProp { return MergeProp{value: value, deep: true} }

// Prepend puts new items before the existing ones instead of after.
func (p MergeProp) Prepend() MergeProp { p.prepend = true; return p }

// MatchOn de-duplicates merged array items by these fields (e.g. "id").
func (p MergeProp) MatchOn(fields ...string) MergeProp { p.matchOn = fields; return p }

// OnceProp is resolved once and cached by the client; later visits skip it
// until it expires or is explicitly reloaded.
type OnceProp struct {
	value any
	ttl   time.Duration
}

// Once wraps a resolver the client keeps across visits.
func Once(resolver any) OnceProp { return OnceProp{value: resolver} }

// Until sets how long the client keeps the value.
func (p OnceProp) Until(ttl time.Duration) OnceProp { p.ttl = ttl; return p }

// ScrollProp powers the client's <InfiniteScroll> component: the items
// array (under the wrapper key, default "data") is appended or prepended
// depending on which direction the client is loading.
type ScrollProp struct {
	value   any
	meta    ScrollMeta
	wrapper string
	matchOn []string
}

// ScrollMeta describes the pagination state. Page identifiers are usually
// ints, but cursor strings work too; nil means "no more pages".
type ScrollMeta struct {
	PageName     string `json:"pageName"` // query param, default "page"
	PreviousPage any    `json:"previousPage"`
	NextPage     any    `json:"nextPage"`
	CurrentPage  any    `json:"currentPage"`
}

// Scroll wraps a value that already contains the items array under "data"
// (e.g. gin.H{"data": posts, "total": n}) together with its pagination meta.
func Scroll(value any, meta ScrollMeta) ScrollProp {
	if meta.PageName == "" {
		meta.PageName = "page"
	}
	return ScrollProp{value: value, meta: meta, wrapper: "data"}
}

// ScrollPage is the common page-number case: items for `page`, and whether
// another page exists after it (fetch perPage+1 rows to know).
//
//	ginertia.ScrollPage(posts, page, hasMore)
func ScrollPage(items any, page int, hasMore bool) ScrollProp {
	meta := ScrollMeta{PageName: "page", CurrentPage: page}
	if page > 1 {
		meta.PreviousPage = page - 1
	}
	if hasMore {
		meta.NextPage = page + 1
	}
	return Scroll(map[string]any{"data": items}, meta)
}

// Wrapper changes the key holding the items array (default "data").
func (p ScrollProp) Wrapper(key string) ScrollProp { p.wrapper = key; return p }

// PageName changes the query parameter name (default "page").
func (p ScrollProp) PageName(name string) ScrollProp { p.meta.PageName = name; return p }

// MatchOn de-duplicates items by these fields when merging (e.g. "id").
func (p ScrollProp) MatchOn(fields ...string) ScrollProp { p.matchOn = fields; return p }

// resolve evaluates a resolver func, or returns plain values unchanged.
func resolve(c *gin.Context, v any) (any, error) {
	switch f := v.(type) {
	case nil:
		return nil, nil
	case func() any:
		return f(), nil
	case func() (any, error):
		return f()
	case func(*gin.Context) any:
		return f(c), nil
	case func(*gin.Context) (any, error):
		return f(c)
	case OptionalProp:
		return resolve(c, f.value)
	case AlwaysProp:
		return resolve(c, f.value)
	case DeferProp:
		return resolve(c, f.value)
	case MergeProp:
		return resolve(c, f.value)
	case OnceProp:
		return resolve(c, f.value)
	case ScrollProp:
		return resolve(c, f.value)
	}
	return resolveReflect(c, v)
}

func unknownResolver(v any) error {
	return fmt.Errorf("ginertia: unsupported resolver %T", v)
}
