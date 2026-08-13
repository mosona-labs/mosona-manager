package apublic

import "github.com/labstack/echo/v5"

func Router(e *echo.Group) {
	previewByDomain := e.Group("/preview")
	previewByDomain.GET("/bootstrap", limitPublicDataRequests(resolvePublicPageByDomain(bootstrap)))
	previewByDomain.GET("/sse", limitPublicDataRequests(resolvePublicPageByDomain(sse)))

	previewByName := e.Group("/preview/:name")
	previewByName.GET("/bootstrap", limitPublicDataRequests(resolvePublicPageByName(bootstrap)))
	previewByName.GET("/sse", limitPublicDataRequests(resolvePublicPageByName(sse)))
}
