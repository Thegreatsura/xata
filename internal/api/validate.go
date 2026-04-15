package api

import (
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// echoValidator provides validation support to the API handlers.
type echoValidator validator.Validate

func newEchoValidator() *echoValidator {
	v := validator.New()

	// register function to get tag name from json tags.
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})

	return (*echoValidator)(v)
}

// Validate checks if i is valid. If i implements the customValidator interface,
// we will call `Validate` directly.
func (v *echoValidator) Validate(i any) (err error) {
	if i == nil {
		return nil
	}

	// the go-playground/validator project eventually fails on maps or arrays.
	// In order to allow us to use non-struct types as body in our API we
	// have to wrap the project and implement maps and array support at the top-level
	// by ourselves :(
	//
	// Note: The gin project also wraps the validator with custom logic for non-struct spec.
	//       See https://github.com/gin-gonic/gin/blob/master/binding/default_validator.go

	value := reflect.ValueOf(i)
	switch value.Kind() {
	case reflect.Pointer:
		err = v.Validate(value.Elem().Interface())
	case reflect.Struct:
		err = (*validator.Validate)(v).Struct(i)
	case reflect.Map:
		for iter := value.MapRange(); iter.Next(); {
			if err = v.Validate(iter.Value().Interface()); err != nil {
				err = UserError(err)
				err = BadRequestErrf("'%s': %w", iter.Key(), err)
			}
		}
	case reflect.Array, reflect.Slice:
		count := value.Len()
		for i := range count {
			if err = v.Validate(value.Index(i).Interface()); err != nil {
				err = UserError(err)
				err = BadRequestErrf("index %v: %w", i, err)
				break
			}
		}
	}

	return err
}
