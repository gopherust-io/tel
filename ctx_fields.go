package tel

import (
	"context"

	"github.com/rs/zerolog"
)

type ctxFieldsKey struct{}

// Field kinds for the context bag (no interface{} on the hot path).
const (
	fieldKindStr = iota + 1
	fieldKindInt
	fieldKindBool
)

// Field is a typed context log attribute.
// goalign:ignore // trailing bool padding is unavoidable
type Field struct {
	Key  string
	Str  string
	Int  int64
	Kind uint8
	Bool bool
}

// StrField builds a string field.
func StrField(key, val string) Field {
	return Field{Kind: fieldKindStr, Key: key, Str: val}
}

// IntField builds an int64 field.
func IntField(key string, val int64) Field {
	return Field{Kind: fieldKindInt, Key: key, Int: val}
}

// BoolField builds a bool field.
func BoolField(key string, val bool) Field {
	return Field{Kind: fieldKindBool, Key: key, Bool: val}
}

// WithFields returns a child context that carries additional log fields.
// Nested calls append (last-wins when applied). Parent slice is copied — never mutated.
func WithFields(ctx context.Context, fields ...Field) context.Context {
	if len(fields) == 0 {
		return ctx
	}
	parent, _ := ctx.Value(ctxFieldsKey{}).([]Field)
	n := len(parent) + len(fields)
	out := make([]Field, n)
	copy(out, parent)
	copy(out[len(parent):], fields)

	return context.WithValue(ctx, ctxFieldsKey{}, out)
}

func fieldsFromCtx(ctx context.Context) []Field {
	fields, _ := ctx.Value(ctxFieldsKey{}).([]Field)

	return fields
}

func applyFields(c zerolog.Context, fields []Field) zerolog.Context {
	for i := range fields {
		field := &fields[i]
		switch field.Kind {
		case fieldKindStr:
			c = c.Str(field.Key, field.Str)
		case fieldKindInt:
			c = c.Int64(field.Key, field.Int)
		case fieldKindBool:
			c = c.Bool(field.Key, field.Bool)
		}
	}

	return c
}
