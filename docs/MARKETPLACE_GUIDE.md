# Plugin Marketplace Usage Guide

## Introduction

The CC Gateway Plugin Marketplace provides a centralized catalog for discovering and installing pre-configured plugin bundles. This guide covers how to browse, search, install, and manage plugins from the marketplace.

## Quick Start

### 1. Browse Available Plugins

List all available plugins in the marketplace:

```bash
curl http://127.0.0.1:8080/v1/cc/marketplace/plugins
```

You'll see 5 pre-built bundles:
- **glm-local**: Local GLM model integration
- **openai-proxy**: OpenAI API proxy
- **anthropic-proxy**: Anthropic Claude API proxy
- **search-tools**: Web search tools
- **file-tools**: File operation tools

### 2. Get Plugin Details

View detailed information about a specific plugin:

```bash
curl http://127.0.0.1:8080/v1/cc/marketplace/plugins/glm-local
```

This shows:
- Plugin description and metadata
- Required and optional configuration fields
- Skills, hooks, and MCP servers included
- Dependencies (if any)

### 3. Install a Plugin

Install a plugin with configuration:

```bash
curl -X POST http://127.0.0.1:8080/v1/cc/marketplace/plugins/glm-local/install \
  -H 'Content-Type: application/json' \
  -d '{
    "config": {
      "glm5_url": "http://127.0.0.1:5025",
      "glm47_url": "http://127.0.0.1:5022",
      "api_key": "free"
    }
  }'
```

## Searching for Plugins

### Search by Text Query

Find plugins by name or description:

```bash
# Search for "proxy" in name or description
curl 'http://127.0.0.1:8080/v1/cc/marketplace/search?q=proxy'

# Search for "tools"
curl 'http://127.0.0.1:8080/v1/cc/marketplace/search?q=tools'
```

### Search by Tags

Filter plugins by tags:

```bash
# Find all proxy plugins
curl 'http://127.0.0.1:8080/v1/cc/marketplace/search?tags=proxy'

# Find all tool plugins
curl 'http://127.0.0.1:8080/v1/cc/marketplace/search?tags=tools'
```

### Combined Search

Use both query and tags:

```bash
curl 'http://127.0.0.1:8080/v1/cc/marketplace/search?q=openai&tags=proxy'
```

## Plugin Recommendations

Get personalized plugin recommendations:

```bash
curl http://127.0.0.1:8080/v1/cc/marketplace/recommendations
```

Recommendations are based on:
- **No plugins installed**: Returns starter bundles (glm-local, openai-proxy, etc.)
- **Some plugins installed**: Returns complementary plugins
- **Maximum 5 recommendations** are returned

## Checking for Updates

Check if newer versions of installed plugins are available:

```bash
curl http://127.0.0.1:8080/v1/cc/marketplace/updates
```

Response shows:
- Current installed version
- Available version in marketplace
- Whether an update is available

## Pre-Built Plugin Bundles

### 1. GLM Local

**Purpose**: Run GLM-5 and GLM-4.7 models locally

**Configuration:**
```json
{
  "glm5_url": "http://127.0.0.1:5025",
  "glm47_url": "http://127.0.0.1:5022",
  "api_key": "free"
}
```

**Use Case**: Local model deployment without external API dependencies

### 2. OpenAI Proxy

**Purpose**: Route requests to OpenAI's GPT models

**Configuration:**
```json
{
  "api_key": "sk-...",
  "base_url": "https://api.openai.com/v1",
  "default_model": "gpt-4"
}
```

**Use Case**: OpenAI API integration with automatic retry and failover

### 3. Anthropic Proxy

**Purpose**: Route requests to Claude models

**Configuration:**
```json
{
  "api_key": "sk-ant-...",
  "base_url": "https://api.anthropic.com",
  "default_model": "claude-3-5-sonnet-20241022"
}
```

**Use Case**: Anthropic Claude integration with streaming support

### 4. Search Tools

**Purpose**: Web search and information retrieval

**Configuration:**
```json
{
  "search_api_url": "https://api.search.example.com"
}
```

**Includes:**
- `web_search`: Search the web for information
- `image_search`: Find images
- `image_recognition`: Analyze images

### 5. File Tools

**Purpose**: File system operations

**Configuration:**
```json
{
  "allowed_paths": "/tmp,/var/tmp",
  "max_file_size": "10485760"
}
```

**Includes:**
- `file_read`: Read file contents
- `file_write`: Write to files
- `file_list`: List directory contents

## Creating Custom Plugin Manifests

You can create your own plugin manifests and add them to the marketplace.

### Manifest Structure

Create a JSON file in `configs/marketplace/`:

```json
{
  "name": "my-plugin",
  "version": "1.0.0",
  "description": "My custom plugin",
  "author": "Your Name",
  "source": "community.custom",
  "verified": false,
  "tags": ["custom", "integration"],
  "license": "AGPL-3.0-or-later",
  "dependencies": [],
  "skills": [
    {
      "name": "my-skill",
      "description": "Custom skill",
      "template": "Do something with: {{input}}"
    }
  ],
  "hooks": [],
  "mcp_servers": [],
  "config_schema": {
    "api_key": {
      "type": "string",
      "description": "API key for service",
      "required": true
    },
    "endpoint": {
      "type": "url",
      "description": "Service endpoint URL",
      "required": false,
      "default": "https://api.example.com"
    }
  }
}
```

### Required Fields

- `name`: Plugin identifier (alphanumeric, hyphens, underscores only)
- `version`: Semantic version (major.minor.patch)
- `description`: Brief description of plugin functionality
- `author`: Plugin author name

### Optional Fields

- `tags`: Array of categorization tags
- `dependencies`: Array of required plugins
- `license`: License identifier (e.g., "MIT", "Apache-2.0")
- `source`: Metadata source label (e.g., `builtin.ccgateway`, `community.custom`, `cloud.vendor`)
- `verified`: Whether the manifest is curated/verified by trusted maintainers
- `homepage`: Plugin homepage URL (optional; if set must be a real non-placeholder URL)
- `icon_url`: URL to plugin icon
- `skills`: Array of skill configurations
- `hooks`: Array of hook configurations
- `mcp_servers`: Array of MCP server configurations
- `config_schema`: Configuration field definitions

### Configuration Schema

Define required and optional configuration fields:

```json
"config_schema": {
  "field_name": {
    "type": "string|int|bool|url",
    "description": "Field description",
    "required": true|false,
    "default": "default_value"
  }
}
```

### Validation Rules

Plugin manifests must follow these rules:

1. **Name**: Only alphanumeric characters, hyphens, and underscores
2. **Version**: Must follow semantic versioning (e.g., "1.0.0")
3. **Commands**: MCP server commands are validated for shell injection risks
4. **Dependencies**: Must reference existing plugins

## Troubleshooting

### Plugin Not Found

**Error**: `plugin "xyz" not found in marketplace`

**Solution**: 
- Check plugin name spelling
- Verify plugin manifest exists in `configs/marketplace/`
- Restart gateway to reload marketplace registry

### Missing Dependencies

**Error**: `missing required dependencies: [dep1, dep2]`

**Solution**: Install dependencies first:
```bash
curl -X POST http://127.0.0.1:8080/v1/cc/marketplace/plugins/dep1/install
curl -X POST http://127.0.0.1:8080/v1/cc/marketplace/plugins/dep2/install
```

### Already Installed

**Error**: `plugin "xyz" is already installed`

**Solution**: 
- Uninstall existing plugin first
- Or use the update mechanism if available

### Invalid Configuration

**Error**: `missing required configuration field: api_key`

**Solution**: Provide all required configuration fields:
```bash
curl -X POST http://127.0.0.1:8080/v1/cc/marketplace/plugins/xyz/install \
  -H 'Content-Type: application/json' \
  -d '{"config": {"api_key": "your-key"}}'
```

## Best Practices

1. **Review plugin details** before installation to understand configuration requirements
2. **Check dependencies** to ensure all required plugins are available
3. **Use search and tags** to discover relevant plugins efficiently
4. **Check for updates** regularly to benefit from bug fixes and new features
5. **Test in development** before deploying plugins to production
6. **Document custom plugins** with clear descriptions and configuration examples

## Next Steps

- Explore the [Marketplace API Documentation](MARKETPLACE_API.md) for detailed API reference
- Learn about [Plugin Development](PLUGIN_DEVELOPMENT.md) to create custom plugins
- Check the [Admin Console](ADMIN_CONSOLE_FEATURES.md) for web-based plugin management
