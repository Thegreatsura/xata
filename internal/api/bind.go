package api

import "github.com/labstack/echo/v4"

// ReadBody from echo context into the given variable
func ReadBody(c echo.Context, to any) (err error) {
	if err = c.Bind(to); err == nil {
		err = c.Validate(to)
	}
	return UserError(err)
}
