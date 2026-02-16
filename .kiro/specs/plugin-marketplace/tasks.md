# Implementation Plan: Plugin Marketplace

## Overview

This implementation plan breaks down the plugin marketplace feature into incremental coding tasks. Each task builds on previous work, starting with core data models and registry functionality, then adding the marketplace service layer, REST API endpoints, validation, and finally the pre-built bundles. Testing tasks are integrated throughout to catch errors early.

## Tasks

- [x] 1. Create core data models and registry interface
  - Create `internal/marketplace/types.go` with PluginManifest, Dependency, ConfigField, UpdateInfo, SearchResult structs
  - Create `internal/marketplace/registry.go` with Registry interface definition
  - Set up basic package structure and imports
  - _Requirements: 1.1, 2.1, 2.2, 2.3_

- [ ]* 1.1 Write property test for manifest structure
  - **Property 1: Manifest round-trip preservation**
  - **Validates: Requirements 1.1, 2.4**

- [ ] 2. Implement LocalRegistry for file-based storage
  - [x] 2.1 Implement LocalRegistry struct with file system operations
    - Create `internal/marketplace/local_registry.go`
    - Implement List(), Get(), Add(), Remove(), Refresh() methods
    - Read/write JSON manifest files from `configs/marketplace/` directory
    - _Requirements: 1.1, 1.3, 1.4_
  
  - [ ]* 2.2 Write property test for registry query completeness
    - **Property 3: Query completeness**
    - **Validates: Requirements 1.4**
  
  - [x] 2.3 Implement version history tracking
    - Modify LocalRegistry to store multiple versions per plugin
    - Update Get() to accept optional version parameter
    - _Requirements: 1.5_
  
  - [ ]* 2.4 Write property test for version history
    - **Property 4: Version history preservation**
    - **Validates: Requirements 1.5**

- [ ] 3. Implement manifest validator
  - [x] 3.1 Create Validator struct and validation methods
    - Create `internal/marketplace/validator.go`
    - Implement ValidateManifest(), ValidateName(), ValidateVersion(), ValidateCommand()
    - Add regex patterns for name validation (alphanumeric, hyphens, underscores)
    - Add semantic version parsing and validation
    - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.5_
  
  - [ ]* 3.2 Write property tests for validation rules
    - **Property 5: Required field validation**
    - **Property 8: Plugin name format validation**
    - **Property 9: Semantic version validation**
    - **Property 10: Command injection prevention**
    - **Validates: Requirements 2.1, 7.1, 7.2, 7.3, 7.4**

- [ ] 4. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 5. Implement marketplace service
  - [x] 5.1 Create Service struct with core methods
    - Create `internal/marketplace/service.go`
    - Implement NewService(), ListAvailable(), GetManifest()
    - Wire together Registry, Plugin Manager, and Validator
    - _Requirements: 1.4, 2.5_
  
  - [x] 5.2 Implement search functionality
    - Implement Search() method with text query and tag filtering
    - Add relevance scoring algorithm (exact match, contains, tag match)
    - Sort results by relevance score
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5_
  
  - [ ]* 5.3 Write property tests for search behavior
    - **Property 11: Text search matches name or description**
    - **Property 12: Tag filter returns only matching plugins**
    - **Property 13: Multiple tag filters use AND logic**
    - **Property 15: Results sorted by relevance score**
    - **Validates: Requirements 3.1, 3.2, 3.3, 3.5**

- [ ] 6. Implement plugin installation from marketplace
  - [x] 6.1 Implement Install() method with dependency checking
    - Add dependency resolution logic
    - Check if dependencies are installed before proceeding
    - Convert PluginManifest to plugin.Plugin and call Plugin Manager
    - Handle configuration schema and required fields
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 5.4_
  
  - [ ]* 6.2 Write property tests for installation
    - **Property 16: Successful installation enables plugin**
    - **Property 17: Dependency validation before installation**
    - **Property 18: Installation atomicity on failure**
    - **Property 21: Required configuration validation**
    - **Validates: Requirements 4.1, 4.2, 4.3, 4.4, 4.5, 5.4**

- [ ] 7. Implement update mechanism
  - [x] 7.1 Implement CheckUpdates() and Update() methods
    - Compare installed plugin versions with registry versions
    - Implement semantic version comparison logic
    - Handle update with rollback on failure
    - Preserve user configuration during updates
    - _Requirements: 6.1, 6.2, 6.3, 6.4, 6.5_
  
  - [ ]* 7.2 Write property tests for updates
    - **Property 23: Update detection**
    - **Property 24: Update replaces version**
    - **Property 25: Update rollback on failure**
    - **Property 26: Configuration preservation during update**
    - **Validates: Requirements 6.1, 6.2, 6.3, 6.4, 6.5**

- [ ] 8. Implement recommendations
  - [x] 8.1 Implement GetRecommendations() method
    - Add logic for starter bundle recommendations (when no plugins installed)
    - Limit results to maximum 5 plugins
    - _Requirements: 10.4, 10.5_
  
  - [ ]* 8.2 Write property test for recommendation limits
    - **Property 28: Recommendation count limit**
    - **Validates: Requirements 10.5**

- [ ] 9. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 10. Create REST API handlers
  - [x] 10.1 Create marketplace handler file and wire to router
    - Create `internal/gateway/cc_marketplace_handler.go`
    - Add marketplace service to server struct
    - Register routes in `internal/gateway/router.go`
    - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5_
  
  - [x] 10.2 Implement GET /v1/cc/marketplace/plugins endpoint
    - List all available plugins from registry
    - Return JSON array with plugin manifests
    - Include installed status for each plugin
    - _Requirements: 8.1_
  
  - [x] 10.3 Implement GET /v1/cc/marketplace/plugins/{name} endpoint
    - Retrieve specific plugin manifest by name
    - Return 404 if not found in registry
    - _Requirements: 8.2_
  
  - [x] 10.4 Implement POST /v1/cc/marketplace/plugins/{name}/install endpoint
    - Accept optional configuration in request body
    - Call marketplace service Install() method
    - Return installed plugin details or error
    - Emit "plugin.installed" event
    - _Requirements: 8.3_
  
  - [x] 10.5 Implement GET /v1/cc/marketplace/search endpoint
    - Accept query and tags query parameters
    - Call marketplace service Search() method
    - Return search results with relevance scores
    - _Requirements: 8.4_
  
  - [x] 10.6 Implement GET /v1/cc/marketplace/updates endpoint
    - Call marketplace service CheckUpdates() method
    - Return list of available updates
    - _Requirements: 8.5_
  
  - [x] 10.7 Implement GET /v1/cc/marketplace/recommendations endpoint
    - Call marketplace service GetRecommendations() method
    - Return recommended plugins
    - _Requirements: 10.1, 10.2, 10.3, 10.4, 10.5_

- [ ]* 10.8 Write unit tests for API endpoints
  - Test each endpoint with valid and invalid inputs
  - Test error responses and status codes
  - Test integration with marketplace service
  - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5_

- [ ] 11. Create pre-built plugin bundles
  - [x] 11.1 Create GLM Local bundle manifest
    - Create `configs/marketplace/glm-local.json`
    - Define skills, hooks, and MCP servers for GLM Local integration
    - Add configuration schema for required settings
    - _Requirements: 5.1, 5.2, 5.3, 5.5_
  
  - [x] 11.2 Create OpenAI Proxy bundle manifest
    - Create `configs/marketplace/openai-proxy.json`
    - Define proxy configuration and upstream settings
    - _Requirements: 5.1, 5.2, 5.3, 5.5_
  
  - [x] 11.3 Create Anthropic Proxy bundle manifest
    - Create `configs/marketplace/anthropic-proxy.json`
    - Define proxy configuration and upstream settings
    - _Requirements: 5.1, 5.2, 5.3, 5.5_
  
  - [x] 11.4 Create Search Tools bundle manifest
    - Create `configs/marketplace/search-tools.json`
    - Define search-related skills and MCP servers
    - _Requirements: 5.1, 5.2, 5.3, 5.5_
  
  - [x] 11.5 Create File Tools bundle manifest
    - Create `configs/marketplace/file-tools.json`
    - Define file operation skills and MCP servers
    - _Requirements: 5.1, 5.2, 5.3, 5.5_

- [ ]* 11.6 Write unit test for pre-built bundles
  - Test that all 5 required bundles exist in registry
  - Test that each bundle has valid structure
  - Test that bundles can be installed successfully
  - _Requirements: 5.1_

- [ ] 12. Initialize marketplace service in gateway server
  - [x] 12.1 Wire marketplace service into server initialization
    - Modify `internal/gateway/server.go` to create marketplace service
    - Initialize LocalRegistry with configs/marketplace/ path
    - Connect marketplace service to existing plugin manager
    - Add marketplace service to server struct
    - _Requirements: 1.3, 1.4_

- [ ] 13. Final checkpoint - Integration testing
  - [x] 13.1 Test complete marketplace flow
    - Start gateway server
    - List available plugins via API
    - Search for plugins
    - Install a plugin from marketplace
    - Check for updates
    - Update a plugin
    - Verify all operations work end-to-end
  
  - [x] 13.2 Verify error handling
    - Test installation with missing dependencies
    - Test installation of non-existent plugin
    - Test invalid manifest handling
    - Verify descriptive error messages
    - _Requirements: 2.5, 4.5, 7.5_

- [ ] 14. Documentation
  - [x] 14.1 Update API documentation
    - Document all new marketplace endpoints
    - Add example requests and responses
    - Document error codes and messages
  
  - [x] 14.2 Create marketplace usage guide
    - Document how to browse marketplace
    - Document how to install plugins
    - Document how to create custom plugin manifests
    - Document pre-built bundles and their purposes

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate universal correctness properties across random inputs
- Unit tests validate specific examples, edge cases, and API contracts
- Pre-built bundles provide immediate value to users
- The implementation builds on existing plugin system without breaking changes
