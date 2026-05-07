package server

import (
	"encoding/json"
	"net/http"
)

func openAPISpec(apiBaseURL string) map[string]any {
	server := apiBaseURL + "/management-api"
	cacheEntry := map[string]any{
		"type":     "object",
		"required": []string{"id", "key", "version", "scope", "repoId", "updatedAt", "locationId"},
		"properties": map[string]any{
			"id": map[string]any{"type": "string"}, "key": map[string]any{"type": "string"},
			"version": map[string]any{"type": "string"}, "scope": map[string]any{"type": "string"},
			"repoId": map[string]any{"type": "string"}, "updatedAt": map[string]any{"type": "integer"},
			"locationId": map[string]any{"type": "string"},
		},
	}
	storageLocation := map[string]any{
		"type":     "object",
		"required": []string{"id", "folderName", "partCount"},
		"properties": map[string]any{
			"id": map[string]any{"type": "string"}, "folderName": map[string]any{"type": "string"},
			"partCount":        map[string]any{"type": "integer"},
			"mergeStartedAt":   map[string]any{"type": "integer", "nullable": true},
			"mergedAt":         map[string]any{"type": "integer", "nullable": true},
			"partsDeletedAt":   map[string]any{"type": "integer", "nullable": true},
			"lastDownloadedAt": map[string]any{"type": "integer", "nullable": true},
		},
	}
	return map[string]any{
		"openapi":  "3.1.0",
		"info":     map[string]any{"title": "Cache Server Management API", "version": "1.0.0"},
		"servers":  []map[string]any{{"url": server}},
		"security": []map[string]any{{"apiKeyHeader": []any{}}},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"apiKeyHeader": map[string]any{"type": "apiKey", "in": "header", "name": "X-Api-Key"},
			},
			"schemas": map[string]any{
				"CacheEntry":      cacheEntry,
				"StorageLocation": storageLocation,
			},
		},
		"paths": map[string]any{
			"/cache-entries":          pathListDelete("CacheEntry"),
			"/cache-entries/{id}":     pathGetDelete("CacheEntry"),
			"/cache-entries/match":    pathMatch(),
			"/storage-locations/{id}": pathGetDelete("StorageLocation"),
		},
	}
}

func pathGetDelete(schema string) map[string]any {
	idParam := []any{map[string]any{
		"name": "id", "in": "path", "required": true,
		"schema": map[string]any{"type": "string"},
	}}
	ref := map[string]any{"$ref": "#/components/schemas/" + schema}
	jsonContent := map[string]any{"application/json": map[string]any{"schema": ref}}
	return map[string]any{
		"get": map[string]any{
			"summary":    "Get " + schema,
			"parameters": idParam,
			"responses":  map[string]any{"200": map[string]any{"description": "OK", "content": jsonContent}},
		},
		"delete": map[string]any{
			"summary":    "Delete " + schema,
			"parameters": idParam,
			"responses":  map[string]any{"204": map[string]any{"description": "Deleted"}},
		},
	}
}

func pathListDelete(schema string) map[string]any {
	filterParams := []any{
		queryParam("key"), queryParam("version"), queryParam("scope"), queryParam("repoId"),
	}
	ref := map[string]any{"$ref": "#/components/schemas/" + schema}
	return map[string]any{
		"get": map[string]any{
			"summary": "List " + schema + "s",
			"parameters": append(filterParams,
				map[string]any{"name": "page", "in": "query", "schema": map[string]any{"type": "integer", "default": 1}},
				map[string]any{"name": "itemsPerPage", "in": "query", "schema": map[string]any{"type": "integer", "default": 20}},
			),
			"responses": map[string]any{"200": map[string]any{
				"description": "OK",
				"content": map[string]any{"application/json": map[string]any{"schema": map[string]any{
					"type": "object", "required": []string{"total", "items"},
					"properties": map[string]any{
						"total": map[string]any{"type": "integer"},
						"items": map[string]any{"type": "array", "items": ref},
					},
				}}},
			}},
		},
		"delete": map[string]any{
			"summary":    "Delete many " + schema + "s",
			"parameters": filterParams,
			"responses":  map[string]any{"200": map[string]any{"description": "OK"}},
		},
	}
}

func pathMatch() map[string]any {
	return map[string]any{"get": map[string]any{
		"summary": "Match cache entry",
		"parameters": []any{
			map[string]any{"name": "primaryKey", "in": "query", "required": true, "schema": map[string]any{"type": "string"}},
			map[string]any{"name": "version", "in": "query", "required": true, "schema": map[string]any{"type": "string"}},
			map[string]any{"name": "repoId", "in": "query", "required": true, "schema": map[string]any{"type": "string"}},
			map[string]any{"name": "scopes", "in": "query", "required": true,
				"schema": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
			map[string]any{"name": "restoreKeys", "in": "query",
				"schema": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}},
		},
		"responses": map[string]any{"200": map[string]any{"description": "OK"}},
	}}
}

func queryParam(name string) map[string]any {
	return map[string]any{"name": name, "in": "query", "schema": map[string]any{"type": "string"}}
}

const swaggerUIHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Cache Server Management API</title>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
</head><body><div id="swagger-ui"></div>
<script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>window.onload=()=>SwaggerUIBundle({url:"/management-api/_docs/spec.json",dom_id:"#swagger-ui"})</script>
</body></html>`

func mgmtDocsHTML(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerUIHTML))
}

func mgmtDocsSpec(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openAPISpec(d.Cfg.APIBaseURL))
	}
}
