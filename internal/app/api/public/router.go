package apublic

import "github.com/labstack/echo/v5"

func Router(e *echo.Group) {
	previewByDomain := e.Group("/preview", resolvePublicPageByDomain)
	previewByDomain.GET("/bootstrap", bootstrap)
	previewByDomain.GET("/sse", sse)

	previewByName := e.Group("/preview/:name", resolvePublicPageByName)
	previewByName.GET("/bootstrap", bootstrap)
	previewByName.GET("/sse", sse)
}
