# PR Template Feature

## Overview

The catmit PR creation feature (`--pr` flag) now supports automatic PR template management with LLM-based content generation.

## Features

### 1. Automatic PR Template Creation
- Creates `~/.config/catmit/pr-template.md` automatically if it doesn't exist
- Uses a comprehensive default template with sections for change type, summary, motivation, implementation details, test plan, and more

### 2. LLM-Based PR Title Generation
- Analyzes commits ahead of the target branch (fork-aware)
- Falls back to branch name analysis if no commits found
- Generates titles in format: `[Type] Brief description`
- Types: Feature, Fix, Refactor, Docs, Chore

### 3. LLM-Based Template Filling
- Intelligently fills PR template sections based on:
  - Commit messages
  - Changed files
  - Diff statistics
- Auto-checks relevant checklist items
- Detects test and documentation changes

## Usage

```bash
# Create PR with auto-generated title and body
catmit --pr

# Create PR to a specific remote/branch
catmit --pr --pr-remote upstream --pr-base main

# Create draft PR
catmit --pr --pr-draft

# Disable template usage (use simple PR)
catmit --pr --pr-template=false
```

## Fork Workflow Support

The PR creation intelligently handles fork workflows:

1. **Upstream First**: Tries to analyze commits against upstream/main
2. **Origin Fallback**: Falls back to origin/main if upstream not available
3. **Branch Name Fallback**: Uses branch name for generation if no base found

## Example Output

For a branch `feature/user-auth` with commits adding authentication:

**Generated Title**: `[Feature] Add user authentication system`

**Generated Body**:
```markdown
### 📝 Change Type
- [x] Feature
- [ ] Bug Fix
- [ ] Refactor
- [ ] Documentation
- [ ] Chore / CI
- [ ] Other (please specify):

### 📌 Summary
Add user authentication system with email validation and secure password handling

### 🎯 Motivation
Enables users to securely register and log in to the application, supporting the core user management functionality

### 🔍 Implementation Details
- Introduced `auth/register.go` for user registration endpoint
- Added `auth/validator.go` for email format validation
- Implemented secure password hashing with bcrypt
- Created middleware for JWT token validation

### ✅ Test Plan
- [x] Added unit tests for registration flow
- [x] Added integration tests for auth endpoints
- [x] Manual testing with Postman
- [x] CI passed

### 🔄 Backward Compatibility & Rollback
- [x] Backward compatible
- [x] Safe to rollback by removing auth endpoints

### 🔗 Related Issues / PRs
Closes #42

### 📋 Checklist
- [x] Code passes local build and tests
- [x] Unit/Integration tests added or updated
- [ ] Documentation updated (API docs, README, etc.)
- [x] Follows coding conventions and style guide
- [x] No sensitive data or secrets in diff
```

## Configuration

The PR template can be customized by editing `~/.config/catmit/pr-template.md`. The LLM will intelligently fill in the sections based on your custom template structure.