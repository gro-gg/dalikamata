package query

import (
	"fmt"
	"sort"
	"time"
)

// Match reports whether the entity represented by fields satisfies filter.
// fields is a map of JSON field names to their Go values (string, int, int64,
// float64, bool, time.Time). A nil filter matches everything.
func Match(filter *Filter, fields map[string]any) (bool, error) {
	if filter == nil {
		return true, nil
	}
	switch filter.Op {
	case OpBool:
		return matchBool(filter, fields)
	case OpTerm:
		return matchTerm(filter, fields)
	case OpTerms:
		return matchTerms(filter, fields)
	case OpRange:
		return matchRange(filter, fields)
	case OpExists:
		_, ok := fields[filter.Field]
		return ok, nil
	default:
		return false, fmt.Errorf("unknown filter op: %q", filter.Op)
	}
}

func matchBool(f *Filter, fields map[string]any) (bool, error) {
	for _, child := range f.Must {
		ok, err := Match(&child, fields)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	for _, child := range f.MustNot {
		ok, err := Match(&child, fields)
		if err != nil {
			return false, err
		}
		if ok {
			return false, nil
		}
	}
	// Should: when Must is empty at least one clause must match.
	if len(f.Must) == 0 && len(f.Should) > 0 {
		for _, child := range f.Should {
			ok, err := Match(&child, fields)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	}
	return true, nil
}

func matchTerm(f *Filter, fields map[string]any) (bool, error) {
	if f.Value == nil {
		return false, fmt.Errorf("term filter on %q: value is nil", f.Field)
	}
	fieldVal, ok := fields[f.Field]
	if !ok {
		return false, nil
	}
	cmp, err := compareFieldValue(fieldVal, *f.Value)
	return err == nil && cmp == 0, err
}

func matchTerms(f *Filter, fields map[string]any) (bool, error) {
	fieldVal, ok := fields[f.Field]
	if !ok {
		return false, nil
	}
	for _, v := range f.Values {
		cmp, err := compareFieldValue(fieldVal, v)
		if err != nil {
			return false, err
		}
		if cmp == 0 {
			return true, nil
		}
	}
	return false, nil
}

func matchRange(f *Filter, fields map[string]any) (bool, error) {
	if f.Range == nil {
		return false, fmt.Errorf("range filter on %q: range is nil", f.Field)
	}
	fieldVal, ok := fields[f.Field]
	if !ok {
		return false, nil
	}
	r := f.Range
	if r.GT != nil {
		cmp, err := compareFieldValue(fieldVal, *r.GT)
		if err != nil {
			return false, err
		}
		if cmp <= 0 {
			return false, nil
		}
	}
	if r.GTE != nil {
		cmp, err := compareFieldValue(fieldVal, *r.GTE)
		if err != nil {
			return false, err
		}
		if cmp < 0 {
			return false, nil
		}
	}
	if r.LT != nil {
		cmp, err := compareFieldValue(fieldVal, *r.LT)
		if err != nil {
			return false, err
		}
		if cmp >= 0 {
			return false, nil
		}
	}
	if r.LTE != nil {
		cmp, err := compareFieldValue(fieldVal, *r.LTE)
		if err != nil {
			return false, err
		}
		if cmp > 0 {
			return false, nil
		}
	}
	return true, nil
}

// Less reports whether entity a should sort before entity b given the sort
// specification. Returns false when all fields compare equal.
func Less(sortFields []SortField, a, b map[string]any) bool {
	for _, s := range sortFields {
		cmp := compareAny(a[s.Field], b[s.Field])
		if cmp == 0 {
			continue
		}
		if s.Order == SortDesc {
			return cmp > 0
		}
		return cmp < 0
	}
	return false
}

// Paginate returns the slice of items starting at from (0-indexed) with at
// most size items. A size of 0 returns all items from the offset. An out-of-
// range from returns an empty slice.
func Paginate[T any](items []T, from, size int) []T {
	if from >= len(items) {
		return items[:0]
	}
	items = items[from:]
	if size > 0 && size < len(items) {
		return items[:size]
	}
	return items
}

// SortBy sorts items in-place using the provided sort specification and
// projection function.
func SortBy[T any](items []T, sortFields []SortField, project func(T) map[string]any) {
	sort.SliceStable(items, func(i, j int) bool {
		return Less(sortFields, project(items[i]), project(items[j]))
	})
}

// compareFieldValue compares a raw map field value against a typed Value.
// Returns -1 (a < b), 0 (equal), 1 (a > b), or an error on type mismatch.
func compareFieldValue(fieldVal any, v Value) (int, error) {
	switch v.Kind {
	case KindString:
		sv, ok := fieldVal.(string)
		if !ok {
			return 0, fmt.Errorf("field value type %T is not a string", fieldVal)
		}
		return cmpOrdered(sv, v.String), nil
	case KindInt:
		var iv int64
		switch fv := fieldVal.(type) {
		case int:
			iv = int64(fv)
		case int64:
			iv = fv
		default:
			return 0, fmt.Errorf("field value type %T is not an integer", fieldVal)
		}
		return cmpOrdered(iv, v.Int), nil
	case KindFloat:
		var fv float64
		switch f := fieldVal.(type) {
		case float64:
			fv = f
		case float32:
			fv = float64(f)
		case int:
			fv = float64(f)
		case int64:
			fv = float64(f)
		default:
			return 0, fmt.Errorf("field value type %T is not numeric", fieldVal)
		}
		return cmpOrdered(fv, v.Float), nil
	case KindBool:
		bv, ok := fieldVal.(bool)
		if !ok {
			return 0, fmt.Errorf("field value type %T is not a bool", fieldVal)
		}
		if bv == v.Bool {
			return 0, nil
		}
		if !bv {
			return -1, nil
		}
		return 1, nil
	case KindTime:
		tv, ok := fieldVal.(time.Time)
		if !ok {
			return 0, fmt.Errorf("field value type %T is not a time.Time", fieldVal)
		}
		if tv.Equal(v.Time) {
			return 0, nil
		}
		if tv.Before(v.Time) {
			return -1, nil
		}
		return 1, nil
	default:
		return 0, fmt.Errorf("unknown value kind: %q", v.Kind)
	}
}

// compareAny compares two arbitrary projection values of the same type.
// Returns -1/0/1; incompatible types or nil values return 0.
func compareAny(a, b any) int {
	if a == nil || b == nil {
		return 0
	}
	switch av := a.(type) {
	case string:
		if bv, ok := b.(string); ok {
			return cmpOrdered(av, bv)
		}
	case int:
		if bv, ok := b.(int); ok {
			return cmpOrdered(av, bv)
		}
	case int64:
		if bv, ok := b.(int64); ok {
			return cmpOrdered(av, bv)
		}
	case float64:
		if bv, ok := b.(float64); ok {
			return cmpOrdered(av, bv)
		}
	case bool:
		if bv, ok := b.(bool); ok {
			if av == bv {
				return 0
			}
			if !av {
				return -1
			}
			return 1
		}
	case time.Time:
		if bv, ok := b.(time.Time); ok {
			if av.Equal(bv) {
				return 0
			}
			if av.Before(bv) {
				return -1
			}
			return 1
		}
	}
	return 0
}

func cmpOrdered[T interface{ ~int | ~int64 | ~float64 | ~string }](a, b T) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
