package template

// defaultPRTemplate is the default PR template content
const defaultPRTemplate = `### 📝 Change Type
<!-- Select one or more -->
- [ ] Feature
- [ ] Bug Fix
- [ ] Refactor
- [ ] Documentation
- [ ] Chore / CI
- [ ] Other (please specify):

### 📌 Summary
<!-- A brief description of what this PR does -->
<!-- Keep it concise and informative -->
e.g., "Add user registration API with email validation and error handling"

### 🎯 Motivation
<!-- Why is this change necessary? What problem does it solve? -->
e.g., "Enables new users to sign up via email, supporting product onboarding flow"

### 🔍 Implementation Details
<!-- Describe key changes, design decisions, modules touched, and edge cases handled -->
- Introduced ` + "`RegisterHandler`" + ` for new route
- Added ` + "`email_validator.go`" + ` for format & uniqueness check
- Updated ` + "`routes.go`" + ` with new endpoint
- Handles duplicate emails and invalid formats

### ✅ Test Plan
<!-- Describe what you tested, how, and test coverage -->
- [x] Added unit tests: normal registration, duplicate email, invalid format
- [x] Manual test via Postman and cURL
- [x] CI passed (link to workflow run if possible)

### 🔄 Backward Compatibility & Rollback
- [x] Backward compatible
- [x] Safe to rollback by removing new handler and route

### 🔗 Related Issues / PRs
<!-- e.g., Closes #123, Depends on #456 -->
Closes #42

### 📋 Checklist
- [x] Code passes local build and tests
- [x] Unit/Integration tests added or updated
- [x] Documentation updated (API docs, README, etc.)
- [x] Follows coding conventions and style guide
- [x] No sensitive data or secrets in diff`