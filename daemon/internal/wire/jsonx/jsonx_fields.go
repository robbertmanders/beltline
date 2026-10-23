package jsonx

import (
	"reflect"
	"slices"
	"strings"
	"unicode"
)

// field is one JSON member a struct writes: its name, where its value sits
// (through embedded structs), and the options its tag asks for.
type field struct {
	name      string
	nameJSON  []byte // the name as a JSON string and a colon
	index     []int  // the field's index path through embedded structs
	typ       reflect.Type
	tagged    bool // the name came from a json tag
	omitEmpty bool
	omitZero  bool
	quoted    bool // the ",string" option
	enc       encodeFunc
}

// value returns the field's value in v, or false when an embedded pointer on
// its path is nil.
func (f *field) value(v reflect.Value) (reflect.Value, bool) {
	for i, x := range f.index {
		if i > 0 && v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return v, false
			}
			v = v.Elem()
		}
		v = v.Field(x)
	}
	return v, true
}

// structFields lists the members encoding/json writes for a struct type: the
// same names, tags, embedding and dominance rules, in the same order. unexported
// reports an unexported embedded field, which keeps nil collections null (see
// encoder.keepNil).
func structFields(t reflect.Type) (fields []field, unexported bool) {
	for i := range t.NumField() {
		if sf := t.Field(i); sf.Anonymous && !sf.IsExported() {
			unexported = true
		}
	}
	type level struct {
		typ   reflect.Type
		index []int
	}
	var (
		all       []field
		current   []level
		next      = []level{{typ: t}}
		count     map[reflect.Type]int
		nextCount map[reflect.Type]int
		visited   = map[reflect.Type]bool{}
	)
	for len(next) > 0 {
		current, next = next, nil
		count, nextCount = nextCount, map[reflect.Type]int{}
		for _, lv := range current {
			if visited[lv.typ] {
				continue
			}
			visited[lv.typ] = true
			for i := range lv.typ.NumField() {
				sf := lv.typ.Field(i)
				if sf.Anonymous {
					ft := sf.Type
					if ft.Kind() == reflect.Pointer {
						ft = ft.Elem()
					}
					if !sf.IsExported() && ft.Kind() != reflect.Struct {
						continue
					}
				} else if !sf.IsExported() {
					continue
				}
				tag := sf.Tag.Get("json")
				if tag == "-" {
					continue
				}
				name, opts, _ := strings.Cut(tag, ",")
				if !isValidTag(name) {
					name = ""
				}
				index := make([]int, len(lv.index)+1)
				copy(index, lv.index)
				index[len(lv.index)] = i
				ft := sf.Type
				if ft.Name() == "" && ft.Kind() == reflect.Pointer {
					ft = ft.Elem()
				}
				if name != "" || !sf.Anonymous || ft.Kind() != reflect.Struct {
					f := field{
						name:      name,
						tagged:    name != "",
						index:     index,
						typ:       sf.Type,
						omitEmpty: hasOption(opts, "omitempty"),
						omitZero:  hasOption(opts, "omitzero"),
						quoted:    hasOption(opts, "string"),
					}
					if f.name == "" {
						f.name = sf.Name
					}
					f.nameJSON = append(appendString(nil, f.name), ':')
					all = append(all, f)
					if count[lv.typ] > 1 {
						// Two instances of the embedded struct: a second copy
						// lets the dominance rule below see the clash.
						all = append(all, all[len(all)-1])
					}
					continue
				}
				nextCount[ft]++
				if nextCount[ft] == 1 {
					next = append(next, level{typ: ft, index: index})
				}
			}
		}
	}
	slices.SortFunc(all, func(a, b field) int {
		if c := strings.Compare(a.name, b.name); c != 0 {
			return c
		}
		if c := len(a.index) - len(b.index); c != 0 {
			return c
		}
		if a.tagged != b.tagged {
			if a.tagged {
				return -1
			}
			return 1
		}
		return slices.Compare(a.index, b.index)
	})
	for i := 0; i < len(all); {
		j := i + 1
		for j < len(all) && all[j].name == all[i].name {
			j++
		}
		if dominant, ok := dominantField(all[i:j]); ok {
			fields = append(fields, dominant)
		}
		i = j
	}
	slices.SortFunc(fields, func(a, b field) int { return slices.Compare(a.index, b.index) })
	return fields, unexported
}

// dominantField picks the field a name stands for among fields of one name,
// sorted shallowest first. A clash at the same depth, with neither tagged,
// leaves the name out.
func dominantField(fields []field) (field, bool) {
	if len(fields) > 1 && len(fields[0].index) == len(fields[1].index) && fields[0].tagged == fields[1].tagged {
		return field{}, false
	}
	return fields[0], true
}

// hasOption reports whether a comma-separated tag option list has name.
func hasOption(opts, name string) bool {
	for opts != "" {
		var o string
		o, opts, _ = strings.Cut(opts, ",")
		if o == name {
			return true
		}
	}
	return false
}

// isValidTag reports whether s can be a JSON member name in a tag, as
// encoding/json allows.
func isValidTag(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		switch {
		case strings.ContainsRune("!#$%&()*+-./:;<=>?@[]^_{|}~ ", c):
		case !unicode.IsLetter(c) && !unicode.IsDigit(c):
			return false
		}
	}
	return true
}

// structEncoder writes the fields of a struct in the order structFields
// lists them. Fields reached through a nil embedded pointer are skipped.
func (b *builder) structEncoder(t reflect.Type) encodeFunc {
	fields, unexported := structFields(t)
	for i := range fields {
		fields[i].enc = b.get(fields[i].typ)
		if fields[i].quoted {
			fields[i].enc = quotedEncoder(fields[i].typ, fields[i].enc)
		}
	}
	return func(e *encoder, v reflect.Value) error {
		saved := e.keepNil
		if unexported {
			e.keepNil = true
		}
		e.reserve(128)
		e.buf = append(e.buf, '{')
		first := true
		for i := range fields {
			f := &fields[i]
			fv, ok := f.value(v)
			if !ok {
				continue
			}
			if (f.omitEmpty && isEmptyValue(fv)) || (f.omitZero && fv.IsZero()) {
				continue
			}
			if !first {
				e.buf = append(e.buf, ',')
			}
			first = false
			e.buf = append(e.buf, f.nameJSON...)
			fieldSaved := e.keepNil
			if (f.omitEmpty || f.omitZero) && fv.IsZero() {
				e.keepNil = true
			}
			err := f.enc(e, fv)
			e.keepNil = fieldSaved
			if err != nil {
				return err
			}
		}
		e.buf = append(e.buf, '}')
		e.keepNil = saved
		return nil
	}
}

// isEmptyValue is encoding/json's test for omitempty.
func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Pointer:
		return v.IsNil()
	}
	return false
}

// appendFloat writes f as encoding/json does: the shortest form that reads
// back exactly, in exponent form only for very small or very large values.

// quotedEncoder writes a field tagged ",string" as encoding/json does: a bool, number or string value
// is encoded, then written again as a JSON string, through one unnamed pointer (nil stays null). The
// option is ignored on any other type, and on a type that marshals itself.
func quotedEncoder(t reflect.Type, enc encodeFunc) encodeFunc {
	pointer := t.Kind() == reflect.Pointer && t.Name() == ""
	base := t
	if pointer {
		base = t.Elem()
	}
	for _, m := range []reflect.Type{marshalerType, textMarshalerType} {
		if base.Implements(m) || reflect.PointerTo(base).Implements(m) {
			return enc
		}
	}
	switch base.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.String:
	default:
		return enc
	}
	return func(e *encoder, v reflect.Value) error {
		if pointer && v.IsNil() {
			e.buf = append(e.buf, "null"...)
			return nil
		}
		start := len(e.buf)
		if err := enc(e, v); err != nil {
			return err
		}
		inner := string(e.buf[start:])
		e.buf = appendString(e.buf[:start], inner)
		return nil
	}
}
