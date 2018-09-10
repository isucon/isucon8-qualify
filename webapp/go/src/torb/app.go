package main

import (
	"net/http"

	"github.com/labstack/echo"
)

func main() {
	e := echo.New()
	e.GET("/", func(ctx echo.Context) error {
		return ctx.String(http.StatusOK, "Hello torb.go\n")
	})
	e.Start(":8080")
}
