package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jjbank/skill-market/internal/model"
	"github.com/jjbank/skill-market/internal/service"
)

// GitHubHandler exposes public, read-only discovery and archive endpoints.
type GitHubHandler struct {
	service *service.GitHubService
}

func NewGitHubHandler(githubService *service.GitHubService) *GitHubHandler {
	return &GitHubHandler{service: githubService}
}

func (h *GitHubHandler) Status(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.HostStatus())
}

func (h *GitHubHandler) SearchSkills(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "12"))
	request := model.GitHubSkillSearchRequest{Query: c.Query("query"), Page: page, PageSize: pageSize}
	result, err := h.service.Search(c.Request.Context(), &request)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *GitHubHandler) PreviewSkill(c *gin.Context) {
	preview, err := h.service.PreviewSkill(c.Request.Context(), c.Query("repo"), c.Query("ref"), c.Query("path"))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.Header("Cache-Control", "private, max-age=300")
	c.JSON(http.StatusOK, preview)
}

func (h *GitHubHandler) DownloadSkill(c *gin.Context) {
	filename, archive, err := h.service.BuildArchive(c.Request.Context(), c.Query("repo"), c.Query("ref"), c.Query("path"))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	filename = strings.ReplaceAll(filename, `"`, "")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Cache-Control", "private, max-age=300")
	c.Data(http.StatusOK, "application/zip", archive)
}
