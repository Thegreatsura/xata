package envcfg

import "reflect"

type ifcConfig struct {
	t    reflect.Type
	call func(v any) error
}

func (c *ifcConfig) match(rv reflect.Value) bool {
	return rv.Type().Implements(c.t)
}

func walkIfcCall(pred func(rv reflect.Value) bool, fn func(v any) error, rv reflect.Value) error {
	for k := rv.Kind(); k == reflect.Pointer || k == reflect.Interface; k = rv.Kind() {
		if !rv.IsNil() {
			rv = rv.Elem()
		} else {
			panic("nil pointer in config structures")
		}
	}

	switch rv.Kind() {
	case reflect.Struct:
		n := rv.NumField()
		for i := range n {
			if !rv.Type().Field(i).IsExported() {
				continue
			}
			if err := walkIfcCall(pred, fn, rv.Field(i)); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		n := rv.Len()
		for i := range n {
			if err := walkIfcCall(pred, fn, rv.Index(i)); err != nil {
				return err
			}
		}
	case reflect.Map:
		r := rv.MapRange()
		for r.Next() {
			if err := walkIfcCall(pred, fn, r.Value()); err != nil {
				return err
			}
		}
	}

	if pred(rv) {
		return fn(rv.Interface())
	}
	if rv.CanAddr() && pred(rv.Addr()) {
		return fn(rv.Addr().Interface())
	}
	return nil
}
