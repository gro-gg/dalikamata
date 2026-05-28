package query

import "time"

// ValueKind discriminates the type held by a Value.
type ValueKind string

const (
	KindString ValueKind = "string"
	KindInt    ValueKind = "int"
	KindFloat  ValueKind = "float"
	KindBool   ValueKind = "bool"
	KindTime   ValueKind = "time"
)

// Value is a tagged union representing a scalar used in filter comparisons.
type Value struct {
	Kind   ValueKind `json:"kind"`
	String string    `json:"string,omitempty"`
	Int    int64     `json:"int,omitempty"`
	Float  float64   `json:"float,omitempty"`
	Bool   bool      `json:"bool,omitempty"`
	Time   time.Time `json:"time,omitempty"`
}

func StringValue(s string) Value  { return Value{Kind: KindString, String: s} }
func IntValue(i int64) Value      { return Value{Kind: KindInt, Int: i} }
func FloatValue(f float64) Value  { return Value{Kind: KindFloat, Float: f} }
func BoolValue(b bool) Value      { return Value{Kind: KindBool, Bool: b} }
func TimeValue(t time.Time) Value { return Value{Kind: KindTime, Time: t} }
