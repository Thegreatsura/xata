package api

import (
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// xataEchoContext overwrites the echo default JSON encoding.
// The `pretty` query parameter provided by the echo Framework is ignored,
// xata uses `_pretty` instead.
type xataEchoContext struct {
	echo.Context
	tracer trace.Tracer
}

func (j *xataEchoContext) JSON(status int, value any) error {
	ctx := j.Context.Request().Context()
	_, span := j.tracer.Start(ctx, "Encode")
	defer span.End()

	var err error
	if j.Context.QueryParams().Has("_pretty") {
		err = j.JSONPretty(status, value, "  ")
	} else {
		err = j.Context.JSON(status, value)
	}

	if err != nil {
		span.RecordError(err)
	}
	return err
}

func xataEchoMiddleware(
	tracerProvider trace.TracerProvider,
) echo.MiddlewareFunc {
	if tracerProvider == nil {
		tracerProvider = noop.NewTracerProvider()
	}
	tracer := tracerProvider.Tracer("xata.api.json")

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// default content type to JSON if content type header is missing:
			req := c.Request()
			if req.Header.Get(echo.HeaderContentType) == "" {
				req.Header.Add(echo.HeaderContentType, echo.MIMEApplicationJSON)
			}

			// adapt echo.Context to Xata defaults
			return next(&xataEchoContext{c, tracer})
		}
	}
}
