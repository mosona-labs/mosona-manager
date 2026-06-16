package ateam

import (
	"errors"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"
	"mosona-manager/internal/db"
	"mosona-manager/internal/siteaccess"
	"mosona-manager/internal/utils"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/lib/pq"
)

func getPublicPage(c *echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	if tid == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid team data",
		})
	}

	page, err := db.GetTeamPublicPage(tid)
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Success",
		Data: buildPublicPageResponse(c, page),
	})
}

func setPublicPage(c *echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	if tid == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid team data",
		})
	}

	var req publicPageUpdateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, _type.H{
			Code: "invalid",
			Msg:  "Invalid request format",
		})
	}

	name, err := normalizePublicPageName(req.Name)
	if err != nil {
		return c.JSON(400, _type.H{
			Code: "invalid",
			Msg:  "Invalid public page name",
		})
	}
	domain, err := normalizePublicPageDomain(req.Domain)
	if err != nil {
		return c.JSON(400, _type.H{
			Code: "invalid",
			Msg:  "Invalid public page domain",
		})
	}
	title, err := normalizePublicPageTitle(req.Title)
	if err != nil {
		return c.JSON(400, _type.H{
			Code: "invalid",
			Msg:  "Invalid public page title",
		})
	}
	description := normalizePublicPageDescription(req.Description)
	customCSS := normalizePublicPageCustomCSS(req.CustomCSS)
	if req.Enabled && name == nil && domain == nil {
		return c.JSON(400, _type.H{
			Code: "invalid",
			Msg:  "At least one of name or domain is required when public page is enabled",
		})
	}
	if domain != nil {
		baseDomain := normalizeConfiguredBaseDomain(config.ReadDynamicConf().Domain)
		if baseDomain != "" && *domain == baseDomain {
			return c.JSON(400, _type.H{
				Code: "invalid",
				Msg:  "Public page domain cannot match the application base domain",
			})
		}
	}

	err = db.UpsertTeamPublicPage(tid, req.Enabled, name, domain, title, description, customCSS)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			switch strings.ToLower(pqErr.Constraint) {
			case "team_public_pages_name_unique":
				return c.JSON(409, _type.H{
					Code: "conflict",
					Msg:  "Public page name is already in use",
				})
			case "team_public_pages_domain_unique":
				return c.JSON(409, _type.H{
					Code: "conflict",
					Msg:  "Public page domain is already in use",
				})
			}
		}
		return utils.ErrorHandler(c, err, "Database error")
	}

	if err := siteaccess.Refresh(); err != nil {
		return utils.ErrorHandler(c, err, "Failed to refresh site access cache")
	}

	page, err := db.GetTeamPublicPage(tid)
	if err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Public page settings updated",
		Data: buildPublicPageResponse(c, page),
	})
}

func buildPublicPageResponse(c *echo.Context, page _type.TeamPublicPage) publicPageResponse {
	var urlByName *string
	if page.Name != nil {
		value := publicAppBaseURL(c) + "/preview/" + *page.Name
		urlByName = &value
	}

	var urlByDomain *string
	if page.Domain != nil {
		value := requestScheme(c) + "://" + *page.Domain
		urlByDomain = &value
	}

	return publicPageResponse{
		Enabled:     page.Enabled,
		Name:        page.Name,
		Domain:      page.Domain,
		Title:       page.Title,
		Description: page.Description,
		CustomCSS:   page.CustomCSS,
		URLByName:   urlByName,
		URLByDomain: urlByDomain,
	}
}
