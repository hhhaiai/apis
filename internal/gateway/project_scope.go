package gateway

import (
	"context"
	"net/http"
	"strings"

	"ccgateway/internal/mcpregistry"
	"ccgateway/internal/requestctx"
)

const (
	scopeProject = "project"
	scopeGlobal  = "global"
)

type scopeSelection struct {
	Scope     string
	ProjectID string
}

func withProjectContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectID := projectIDFromRequest(r)
		ctx := requestctx.WithProjectID(r.Context(), projectID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func projectIDFromRequest(r *http.Request) string {
	if r == nil {
		return requestctx.DefaultProjectID
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("project_id")); raw != "" {
		return requestctx.NormalizeProjectID(raw)
	}
	if raw := strings.TrimSpace(r.Header.Get("x-project-id")); raw != "" {
		return requestctx.NormalizeProjectID(raw)
	}
	return requestctx.ProjectID(r.Context())
}

func projectIDFromContext(ctx context.Context) string {
	return requestctx.ProjectID(ctx)
}

func resolveScopeSelection(r *http.Request) scopeSelection {
	scope := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("scope")))
	if scope == "" {
		scope = strings.ToLower(strings.TrimSpace(r.Header.Get("x-scope")))
	}
	switch scope {
	case scopeGlobal:
		return scopeSelection{
			Scope:     scopeGlobal,
			ProjectID: requestctx.DefaultProjectID,
		}
	default:
		return scopeSelection{
			Scope:     scopeProject,
			ProjectID: projectIDFromRequest(r),
		}
	}
}

func pluginScopePrefix(projectID string) string {
	projectID = requestctx.NormalizeProjectID(projectID)
	if projectID == requestctx.DefaultProjectID {
		return ""
	}
	return "prj_" + projectID + "::"
}

func pluginStorageName(projectID, displayName string) string {
	projectID = requestctx.NormalizeProjectID(projectID)
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return ""
	}
	if projectID == requestctx.DefaultProjectID {
		return displayName
	}
	prefix := pluginScopePrefix(projectID)
	if strings.HasPrefix(displayName, prefix) {
		return displayName
	}
	return prefix + displayName
}

func pluginDisplayName(projectID, storageName string) string {
	projectID = requestctx.NormalizeProjectID(projectID)
	storageName = strings.TrimSpace(storageName)
	if storageName == "" {
		return ""
	}
	if projectID == requestctx.DefaultProjectID {
		return storageName
	}
	prefix := pluginScopePrefix(projectID)
	return strings.TrimPrefix(storageName, prefix)
}

func pluginBelongsToProject(projectID, storageName string) bool {
	projectID = requestctx.NormalizeProjectID(projectID)
	storageName = strings.TrimSpace(storageName)
	if storageName == "" {
		return false
	}
	if projectID == requestctx.DefaultProjectID {
		// Legacy/global plugins are unprefixed and belong to default project.
		return !strings.HasPrefix(storageName, "prj_")
	}
	return strings.HasPrefix(storageName, pluginScopePrefix(projectID))
}

func mcpScopePrefix(projectID string) string {
	projectID = requestctx.NormalizeProjectID(projectID)
	if projectID == requestctx.DefaultProjectID {
		return ""
	}
	return "prj_" + projectID + "__"
}

func mcpStorageID(projectID, displayID string) string {
	projectID = requestctx.NormalizeProjectID(projectID)
	displayID = strings.TrimSpace(displayID)
	if projectID == requestctx.DefaultProjectID || displayID == "" {
		return displayID
	}
	prefix := mcpScopePrefix(projectID)
	if strings.HasPrefix(displayID, prefix) {
		return displayID
	}
	return prefix + displayID
}

func mcpDisplayID(projectID, storageID string) string {
	projectID = requestctx.NormalizeProjectID(projectID)
	storageID = strings.TrimSpace(storageID)
	if projectID == requestctx.DefaultProjectID {
		return storageID
	}
	return strings.TrimPrefix(storageID, mcpScopePrefix(projectID))
}

func mcpServerProjectID(server mcpregistry.Server) string {
	if server.Metadata != nil {
		if raw, ok := server.Metadata["project_id"]; ok {
			if s, ok := raw.(string); ok {
				return requestctx.NormalizeProjectID(s)
			}
		}
	}
	trimmedID := strings.TrimSpace(server.ID)
	if strings.HasPrefix(trimmedID, "prj_") {
		rest := strings.TrimPrefix(trimmedID, "prj_")
		if idx := strings.Index(rest, "__"); idx > 0 {
			return requestctx.NormalizeProjectID(rest[:idx])
		}
	}
	return requestctx.DefaultProjectID
}

func mcpServerBelongsToProject(projectID string, server mcpregistry.Server) bool {
	return requestctx.NormalizeProjectID(projectID) == mcpServerProjectID(server)
}

func mcpServerForProject(projectID string, server mcpregistry.Server) mcpregistry.Server {
	projectID = requestctx.NormalizeProjectID(projectID)
	out := server
	out.ID = mcpDisplayID(projectID, server.ID)
	if out.Metadata == nil {
		out.Metadata = map[string]any{}
	}
	out.Metadata["project_id"] = projectID
	return out
}
