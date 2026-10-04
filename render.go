package ginertia

import (
	"bytes"
	"html/template"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Page is the Inertia page object sent to the client.
type Page struct {
	Component      string                   `json:"component"`
	Props          Props                    `json:"props"`
	URL            string                   `json:"url"`
	Version        string                   `json:"version"`
	EncryptHistory bool                     `json:"encryptHistory"`
	ClearHistory   bool                     `json:"clearHistory"`
	Flash          map[string]any           `json:"flash"`
	SharedProps    []string                 `json:"sharedProps,omitempty"`
	MergeProps     []string                 `json:"mergeProps,omitempty"`
	PrependProps   []string                 `json:"prependProps,omitempty"`
	DeepMergeProps []string                 `json:"deepMergeProps,omitempty"`
	MatchPropsOn   []string                 `json:"matchPropsOn,omitempty"`
	DeferredProps  map[string][]string      `json:"deferredProps,omitempty"`
	OnceProps      map[string]OnceMeta      `json:"onceProps,omitempty"`
	ScrollProps    map[string]ScrollMetaOut `json:"scrollProps,omitempty"`
}

// ScrollMetaOut is ScrollMeta plus the reset flag, as sent to the client.
type ScrollMetaOut struct {
	ScrollMeta
	Reset bool `json:"reset"`
}

// HeaderMergeIntent tells whether the client is loading the next
// ("append") or previous ("prepend") page of an infinite scroll.
const HeaderMergeIntent = "X-Inertia-Infinite-Scroll-Merge-Intent"

// OnceMeta describes a once prop to the client.
type OnceMeta struct {
	Prop      string `json:"prop"`
	ExpiresAt *int64 `json:"expiresAt"`
}

// Render renders a page component with props. On the first visit it returns
// the full HTML document; on Inertia visits it returns the page as JSON.
func Render(c *gin.Context, component string, props map[string]any) {
	getState(c).e.Render(c, component, props)
}

// Render is the Engine method behind the package-level Render.
func (e *Engine) Render(c *gin.Context, component string, props map[string]any) {
	page, err := e.BuildPage(c, component, props)
	if err != nil {
		e.cfg.ErrorHandler(c, err)
		return
	}
	body, err := e.cfg.Marshal(page)
	if err != nil {
		e.cfg.ErrorHandler(c, err)
		return
	}

	if IsInertia(c) {
		c.Header(HeaderInertia, "true")
		c.Data(http.StatusOK, "application/json; charset=utf-8", body)
		return
	}

	html, err := e.renderHTML(c, page, body)
	if err != nil {
		e.cfg.ErrorHandler(c, err)
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", html)
}

// BuildPage resolves props and builds the page object without writing a
// response. Useful for tests or custom responses.
func (e *Engine) BuildPage(c *gin.Context, component string, props map[string]any) (*Page, error) {
	st := getState(c)

	all := e.sharedProps()
	maps.Copy(all, st.shared)
	sharedKeys := slices.Sorted(maps.Keys(all))
	maps.Copy(all, props)

	// Validation errors and flash data: what the previous request stored
	// plus anything set during this request (consumed now, not persisted).
	errs := map[string]any{}
	flash := map[string]any{}
	clearHistory := st.clearHistory
	for _, f := range []*FlashData{st.incoming, st.outgoing} {
		if f == nil {
			continue
		}
		maps.Copy(errs, f.Errors)
		maps.Copy(flash, f.Flash)
		clearHistory = clearHistory || f.ClearHistory
	}
	if st.outgoing != nil {
		st.outgoing = nil
		_ = e.cfg.Flash.Write(c, nil)
	}
	if _, ok := all["errors"]; !ok {
		all["errors"] = Always(errs)
	}

	page := &Page{
		Component:      component,
		URL:            requestURI(c.Request),
		Version:        e.Version(),
		EncryptHistory: e.cfg.EncryptHistory,
		ClearHistory:   clearHistory,
		Flash:          flash,
		SharedProps:    sharedKeys,
	}
	if st.encryptHistory != nil {
		page.EncryptHistory = *st.encryptHistory
	}

	partial := IsInertia(c) && c.GetHeader(HeaderPartialComponent) == component
	only := headerSet(c, HeaderPartialData)
	except := headerSet(c, HeaderPartialExcept)
	reset := headerSet(c, HeaderReset)
	exceptOnce := headerSet(c, HeaderExceptOnceProps)

	out := make(Props, len(all))
	for _, key := range slices.Sorted(maps.Keys(all)) {
		v := all[key]
		if _, always := v.(AlwaysProp); partial && !always {
			if len(only) > 0 && !only[key] {
				continue
			}
			if except[key] {
				continue
			}
		}

		switch p := v.(type) {
		case OptionalProp:
			if !partial {
				continue
			}
		case DeferProp:
			if !partial {
				if page.DeferredProps == nil {
					page.DeferredProps = map[string][]string{}
				}
				page.DeferredProps[p.group] = append(page.DeferredProps[p.group], key)
				continue
			}
			if p.merge && !reset[key] {
				if p.deep {
					page.DeepMergeProps = append(page.DeepMergeProps, key)
				} else {
					page.MergeProps = append(page.MergeProps, key)
				}
			}
		case MergeProp:
			if !reset[key] {
				switch {
				case p.deep:
					page.DeepMergeProps = append(page.DeepMergeProps, key)
				case p.prepend:
					page.PrependProps = append(page.PrependProps, key)
				default:
					page.MergeProps = append(page.MergeProps, key)
				}
				for _, f := range p.matchOn {
					page.MatchPropsOn = append(page.MatchPropsOn, key+"."+f)
				}
			}
		case ScrollProp:
			if page.ScrollProps == nil {
				page.ScrollProps = map[string]ScrollMetaOut{}
			}
			page.ScrollProps[key] = ScrollMetaOut{ScrollMeta: p.meta, Reset: reset[key]}
			if !reset[key] {
				path := key + "." + p.wrapper
				if c.GetHeader(HeaderMergeIntent) == "prepend" {
					page.PrependProps = append(page.PrependProps, path)
				} else {
					page.MergeProps = append(page.MergeProps, path)
				}
				for _, f := range p.matchOn {
					page.MatchPropsOn = append(page.MatchPropsOn, path+"."+f)
				}
			}
		case OnceProp:
			if page.OnceProps == nil {
				page.OnceProps = map[string]OnceMeta{}
			}
			meta := OnceMeta{Prop: key}
			if p.ttl > 0 {
				exp := time.Now().Add(p.ttl).UnixMilli()
				meta.ExpiresAt = &exp
			}
			page.OnceProps[key] = meta
			// The client already holds it: skip unless explicitly asked.
			if exceptOnce[key] && !(partial && only[key]) {
				continue
			}
		}

		val, err := resolve(c, v)
		if err != nil {
			return nil, err
		}
		out[key] = val
	}
	page.Props = out
	return page, nil
}

func (e *Engine) renderHTML(c *gin.Context, page *Page, pageJSON []byte) ([]byte, error) {
	st := getState(c)
	id := template.HTMLEscapeString(e.cfg.ContainerID)

	var inertia, head template.HTML
	if e.cfg.SSR != nil {
		if res, err := e.cfg.SSR.render(c.Request.Context(), pageJSON); err == nil {
			inertia = template.HTML(res.Body)
			head = template.HTML(strings.Join(res.Head, "\n"))
		} else if e.cfg.SSR.OnError != nil {
			e.cfg.SSR.OnError(c, err)
		}
	}
	if inertia == "" {
		// "<" only appears inside JSON strings, so escaping it keeps the
		// payload from closing the <script> element (custom marshalers
		// may not escape HTML like encoding/json does).
		safe := bytes.ReplaceAll(pageJSON, []byte("<"), []byte(`\u003c`))
		inertia = template.HTML(`<script data-page="` + id + `" type="application/json">` +
			string(safe) + `</script><div id="` + id + `"></div>`)
	}

	data := map[string]any{}
	maps.Copy(data, st.view)
	data["page"] = page
	data["inertia"] = inertia
	data["inertiaHead"] = head

	t, err := e.rootTemplate()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func headerSet(c *gin.Context, name string) map[string]bool {
	h := c.GetHeader(name)
	if h == "" {
		return nil
	}
	set := map[string]bool{}
	for _, k := range strings.Split(h, ",") {
		if k = strings.TrimSpace(k); k != "" {
			set[k] = true
		}
	}
	return set
}

var (
	ctxType = reflect.TypeOf((*gin.Context)(nil))
	errType = reflect.TypeOf((*error)(nil)).Elem()
)

// resolveReflect handles typed resolver funcs like func() []User or
// func(*gin.Context) (*User, error). Non-func values pass through.
func resolveReflect(c *gin.Context, v any) (any, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Func {
		return v, nil
	}
	t := rv.Type()
	var args []reflect.Value
	switch {
	case t.NumIn() == 0:
	case t.NumIn() == 1 && t.In(0) == ctxType:
		args = []reflect.Value{reflect.ValueOf(c)}
	default:
		return nil, unknownResolver(v)
	}
	switch {
	case t.NumOut() == 1:
		return rv.Call(args)[0].Interface(), nil
	case t.NumOut() == 2 && t.Out(1) == errType:
		res := rv.Call(args)
		if err, _ := res[1].Interface().(error); err != nil {
			return nil, err
		}
		return res[0].Interface(), nil
	}
	return nil, unknownResolver(v)
}
