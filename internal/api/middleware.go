package api

import (
	"github.com/labstack/echo/v4"

	"xata/internal/idgen"
)

func requestIDMiddleware(headerName string, idLimit int) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			res := c.Response()
			rid := req.Header.Get(headerName)
			if rid == "" {
				rid = idgen.GenerateWithPrefix("req")
			}
			if len(rid) > idLimit {
				rid = rid[:idLimit]
			}
			res.Header().Set(headerName, rid)

			return next(c)
		}
	}
}
