# Marketplace API Documentation

## Overview

The Marketplace API provides endpoints for discovering, searching, and installing plugins from the CC Gateway plugin marketplace. All endpoints are prefixed with `/v1/cc/marketplace`.

## Authentication

Marketplace endpoints do not require authentication for read operations (list, search, get). Installation operations use the same authentication as other CC Gateway endpoints.

## Endpoints

### List Available Plugins

**GET** `/v1/cc/marketplace/plugins`

Returns all available plugins in the marketplace.

**Response:**
```json
{
  "data": [
    {
      "name": "glm-local",
      "version": "1.0.0",
      "description": "GLM Local model integration",
      "author": "CC Gateway Team",
      "tags": ["model", "glm", "local"],
      "homepage": "https://github.com/hhhaiai/apis/blob/main/docs/MARKETPLACE_GUIDE.md#1-glm-local",
      "license": "MIT"
    }
  ],
  "count": 5
}
```

### Get Plugin Manifest

**GET** `/v1/cc/marketplace/plugins/{name}`

Retrieves detailed information about a specific plugin.

**Parameters:**
- `name` (path): Plugin name

**Response:**
```json
{
  "name": "glm-local",
  "version": "1.0.0",
  "description": "GLM Local model integration",
  "author": "CC Gateway Team",
  "tags": ["model", "glm", "local"],
  "dependencies": [],
  "skills": [],
  "hooks": [],
  "mcp_servers": [],
  "config_schema": {
    "glm5_url": {
      "type": "url",
      "description": "Base URL for GLM-5 model endpoint",
      "required": true,
      "default": "http://127.0.0.1:5025"
    }
  }
}
```

**Error Responses:**
- `404 Not Found`: Plugin not found in marketplace

### Install Plugin

**POST** `/v1/cc/marketplace/plugins/{name}/install`

Installs a plugin from the marketplace.

**Parameters:**
- `name` (path): Plugin name

**Request Body:**
```json
{
  "config": {
    "glm5_url": "http://127.0.0.1:5025",
    "api_key": "your-api-key"
  }
}
```

**Response:**
```json
{
  "success": true,
  "message": "plugin installed successfully"
}
```

**Error Responses:**
- `404 Not Found`: Plugin not found in marketplace
- `409 Conflict`: Plugin already installed
- `400 Bad Request`: Missing dependencies or invalid configuration
  - `dependency_error`: Missing required dependencies
  - `invalid_request_error`: Validation or configuration error

### Search Plugins

**GET** `/v1/cc/marketplace/search`

Searches for plugins matching query and tag filters.

**Query Parameters:**
- `q` (optional): Text query to search in name and description
- `tags` (optional): Comma-separated list of tags (AND logic)

**Examples:**
- `/v1/cc/marketplace/search?q=proxy`
- `/v1/cc/marketplace/search?tags=tools,search`
- `/v1/cc/marketplace/search?q=openai&tags=proxy`

**Response:**
```json
{
  "data": [
    {
      "manifest": {
        "name": "openai-proxy",
        "version": "1.0.0",
        "description": "OpenAI API proxy configuration",
        "author": "CC Gateway Team",
        "tags": ["proxy", "openai"]
      },
      "relevance_score": 100.0,
      "installed": false,
      "installed_version": ""
    }
  ],
  "count": 2
}
```

### Check for Updates

**GET** `/v1/cc/marketplace/updates`

Checks for available updates to installed plugins.

**Response:**
```json
{
  "data": [
    {
      "plugin_name": "glm-local",
      "current_version": "1.0.0",
      "available_version": "1.1.0",
      "update_available": true
    }
  ],
  "count": 1
}
```

### Get Recommendations

**GET** `/v1/cc/marketplace/recommendations`

Returns recommended plugins based on installed plugins or starter bundles.

**Response:**
```json
{
  "data": [
    {
      "name": "glm-local",
      "version": "1.0.0",
      "description": "GLM Local model integration",
      "author": "CC Gateway Team",
      "tags": ["model", "glm", "local"]
    }
  ],
  "count": 5
}
```

**Note:** Maximum 5 recommendations are returned.

### Uninstall Plugin

**POST** `/v1/cc/marketplace/plugins/{name}/uninstall`

**DELETE** `/v1/cc/marketplace/plugins/{name}/uninstall`

Uninstalls a plugin and records uninstallation statistics.

**Parameters:**
- `name` (path): Plugin name

**Response:**
```json
{
  "success": true,
  "message": "plugin uninstalled successfully"
}
```

**Error Responses:**
- `404 Not Found`: Plugin not found or not installed

### Update Plugin

**POST** `/v1/cc/marketplace/plugins/{name}/update`

Updates an installed plugin to the latest version available in the marketplace.

**Parameters:**
- `name` (path): Plugin name

**Response:**
```json
{
  "success": true,
  "message": "plugin updated successfully"
}
```

**Error Responses:**
- `404 Not Found`: Plugin not found or not installed
- `409 Conflict`: Plugin is already at the latest version

### Get Plugin Statistics

**GET** `/v1/cc/marketplace/stats/{name}`

Retrieves usage statistics for a specific plugin.

**Parameters:**
- `name` (path): Plugin name

**Response:**
```json
{
  "plugin_name": "glm-local",
  "install_count": 42,
  "uninstall_count": 5,
  "last_installed": "2026-02-15T10:30:00Z",
  "last_uninstalled": "2026-02-14T15:20:00Z",
  "active_installs": 37
}
```

**Error Responses:**
- `404 Not Found`: No statistics found for plugin

### Get Popular Plugins

**GET** `/v1/cc/marketplace/popular`

Returns the most popular plugins based on active installations.

**Query Parameters:**
- `limit` (optional): Maximum number of results (default: 10)

**Response:**
```json
{
  "data": [
    {
      "plugin_name": "openai-proxy",
      "install_count": 150,
      "uninstall_count": 10,
      "last_installed": "2026-02-15T12:00:00Z",
      "last_uninstalled": "2026-02-10T09:00:00Z",
      "active_installs": 140
    },
    {
      "plugin_name": "glm-local",
      "install_count": 42,
      "uninstall_count": 5,
      "active_installs": 37
    }
  ],
  "count": 2
}
```

## Error Codes

All error responses follow this format:

```json
{
  "type": "error",
  "error": {
    "type": "error_type",
    "message": "Error description"
  }
}
```

**Error Types:**
- `not_found_error`: Resource not found
- `conflict`: Resource already exists
- `dependency_error`: Missing dependencies
- `invalid_request_error`: Invalid request or validation error
- `api_error`: Internal server error

## Pre-Built Bundles

The marketplace includes these pre-built plugin bundles:

1. **glm-local**: GLM Local model integration (GLM-5, GLM-4.7)
2. **openai-proxy**: OpenAI API proxy configuration
3. **anthropic-proxy**: Anthropic Claude API proxy configuration
4. **search-tools**: Web search and information retrieval tools
5. **file-tools**: File system operation tools

## Example Usage

### List all plugins
```bash
curl http://127.0.0.1:8080/v1/cc/marketplace/plugins
```

### Search for proxy plugins
```bash
curl 'http://127.0.0.1:8080/v1/cc/marketplace/search?tags=proxy'
```

### Get plugin details
```bash
curl http://127.0.0.1:8080/v1/cc/marketplace/plugins/glm-local
```

### Install a plugin
```bash
curl -X POST http://127.0.0.1:8080/v1/cc/marketplace/plugins/glm-local/install \
  -H 'Content-Type: application/json' \
  -d '{
    "config": {
      "glm5_url": "http://127.0.0.1:5025",
      "glm47_url": "http://127.0.0.1:5022"
    }
  }'
```

### Check for updates
```bash
curl http://127.0.0.1:8080/v1/cc/marketplace/updates
```

### Get recommendations
```bash
curl http://127.0.0.1:8080/v1/cc/marketplace/recommendations
```

### Uninstall a plugin
```bash
curl -X POST http://127.0.0.1:8080/v1/cc/marketplace/plugins/glm-local/uninstall
```

### Update a plugin
```bash
curl -X POST http://127.0.0.1:8080/v1/cc/marketplace/plugins/glm-local/update
```

### Get plugin statistics
```bash
curl http://127.0.0.1:8080/v1/cc/marketplace/stats/glm-local
```

### Get popular plugins
```bash
curl 'http://127.0.0.1:8080/v1/cc/marketplace/popular?limit=5'
```
