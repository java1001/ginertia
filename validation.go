package ginertia

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// MessageFunc builds the message for one failed rule. field is the
// form/json name of the field (e.g. "email" or "address.city").
type MessageFunc func(fe validator.FieldError, field string) string

// DefaultMessage is used by ValidationErrors when no MessageFunc is passed.
// Replace it to translate messages app-wide.
var DefaultMessage MessageFunc = func(fe validator.FieldError, field string) string {
	label := strings.ReplaceAll(field, "_", " ")
	switch fe.Tag() {
	case "required":
		return fmt.Sprintf("The %s field is required.", label)
	case "email":
		return fmt.Sprintf("The %s must be a valid email address.", label)
	case "url", "http_url":
		return fmt.Sprintf("The %s must be a valid URL.", label)
	case "min", "gte":
		if fe.Kind() == reflect.String || fe.Kind() == reflect.Slice {
			return fmt.Sprintf("The %s must be at least %s characters.", label, fe.Param())
		}
		return fmt.Sprintf("The %s must be at least %s.", label, fe.Param())
	case "max", "lte":
		if fe.Kind() == reflect.String || fe.Kind() == reflect.Slice {
			return fmt.Sprintf("The %s may not be greater than %s characters.", label, fe.Param())
		}
		return fmt.Sprintf("The %s may not be greater than %s.", label, fe.Param())
	case "len":
		return fmt.Sprintf("The %s must be %s characters.", label, fe.Param())
	case "oneof":
		return fmt.Sprintf("The %s must be one of: %s.", label, fe.Param())
	case "eqfield":
		return fmt.Sprintf("The %s confirmation does not match.", label)
	}
	return fmt.Sprintf("The %s is invalid.", label)
}

// ValidationErrors converts the error from c.ShouldBind(&obj) into a
// field → message map ready for WithErrors. Field names use the struct's
// form / json tags so they match the keys of useForm on the client.
// Non-validation errors (malformed JSON, ...) return a generic message
// under the "_" key; the raw parser error is never sent to the client.
func ValidationErrors(err error, obj any, msg ...MessageFunc) map[string]string {
	if err == nil {
		return nil
	}
	mf := DefaultMessage
	if len(msg) > 0 && msg[0] != nil {
		mf = msg[0]
	}
	var ves validator.ValidationErrors
	if !errors.As(err, &ves) {
		return map[string]string{"_": "The request could not be read."}
	}
	t := reflect.TypeOf(obj)
	out := make(map[string]string, len(ves))
	for _, fe := range ves {
		field := fieldPath(t, fe.StructNamespace())
		if _, exists := out[field]; !exists {
			out[field] = mf(fe, field)
		}
	}
	return out
}

// fieldPath maps "CreateUser.Address.City" or "Form.Items[2].Name" to the
// tag-based path "address.city" / "items.2.name".
func fieldPath(t reflect.Type, ns string) string {
	parts := strings.Split(ns, ".")
	if len(parts) > 1 {
		parts = parts[1:] // drop the root struct name
	}
	var out []string
	for _, p := range parts {
		name, index := p, ""
		if i := strings.IndexByte(p, '['); i >= 0 {
			name, index = p[:i], strings.Trim(p[i:], "[]")
		}
		for t != nil && (t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map) {
			t = t.Elem()
		}
		tagName := name
		if t != nil && t.Kind() == reflect.Struct {
			if sf, ok := t.FieldByName(name); ok {
				tagName = tagOf(sf)
				t = sf.Type
			} else {
				t = nil
			}
		}
		out = append(out, tagName)
		if index != "" {
			out = append(out, index)
		}
	}
	return strings.Join(out, ".")
}

func tagOf(sf reflect.StructField) string {
	for _, key := range []string{"form", "json"} {
		if tag := strings.Split(sf.Tag.Get(key), ",")[0]; tag != "" && tag != "-" {
			return tag
		}
	}
	return sf.Name
}
