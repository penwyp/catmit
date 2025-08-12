package llm

// maskAPIKey masks the API key for logging purposes
func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return "****"
	}
	return apiKey[:4] + "***" + apiKey[len(apiKey)-4:]
}

// truncateForLog truncates content for logging with UTF-8 awareness
func truncateForLog(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	// Ensure we don't cut in the middle of a UTF-8 character
	truncated := content[:maxLen]
	for len(truncated) > 0 && !isValidUTF8Start(truncated[len(truncated)-1]) {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated + "..."
}

// isValidUTF8Start checks if a byte can be the start of a UTF-8 character
func isValidUTF8Start(b byte) bool {
	return (b&0x80) == 0 || (b&0xC0) == 0xC0
}