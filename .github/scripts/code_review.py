#!/usr/bin/env python3
# .github/scripts/code_review.py

import os
import sys
import json
import requests
from datetime import datetime

class GitHubAPI:
    def __init__(self, token, repo):
        self.token = token
        self.repo = repo
        self.base_url = "https://api.github.com"
        self.headers = {
            "Authorization": f"Bearer {token}",
            "Accept": "application/vnd.github.v3+json",
            "Content-Type": "application/json"
        }
    
    def request(self, method, endpoint, data=None):
        url = f"{self.base_url}/repos/{self.repo}{endpoint}"
        response = requests.request(method, url, headers=self.headers, json=data)
        
        if not response.ok:
            print(f"GitHub API error: {response.status_code} {response.text}")
            return None
        
        return response.json()

class CodeReviewBot:
    def __init__(self):
        # Get configuration from environment variables
        self.github_token = os.getenv('GITHUB_TOKEN')
        self.gemini_api_key = os.getenv('GEMINI_API_KEY')
        self.repo = os.getenv('GITHUB_REPOSITORY')
        self.sha = os.getenv('GITHUB_SHA')
        self.test_exit_code = os.getenv('TEST_EXIT_CODE')
        
        # Issue flags
        self.has_complexity = os.getenv('HAS_COMPLEXITY') == 'true'
        self.has_const_issues = os.getenv('HAS_CONST_ISSUES') == 'true'
        self.has_large_files = os.getenv('HAS_LARGE_FILES') == 'true'
        self.has_static_issues = os.getenv('HAS_STATIC_ISSUES') == 'true'
        
        # GitHub API client
        self.github = GitHubAPI(self.github_token, self.repo)
    
    def read_file(self, filename):
        """Read file contents"""
        try:
            with open(filename, 'r', encoding='utf-8') as f:
                return f.read()
        except FileNotFoundError:
            print(f"File {filename} does not exist")
            return ""
        except Exception as e:
            print(f"Error reading file {filename}: {e}")
            return ""
    
    def call_gemini_api(self, prompt):
        """Call Gemini API for AI analysis"""
        if not self.gemini_api_key:
            print("GEMINI_API_KEY not found, using fallback analysis")
            return None
        
        url = f"https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key={self.gemini_api_key}"
        
        payload = {
            "contents": [{"parts": [{"text": prompt}]}],
            "generationConfig": {
                "temperature": 0.3,
                "maxOutputTokens": 2048
            }
        }
        
        try:
            response = requests.post(url, json=payload)
            
            if not response.ok:
                print(f"Gemini API error: {response.status_code}")
                return None
            
            data = response.json()
            if data.get('candidates') and data['candidates'][0].get('content'):
                return data['candidates'][0]['content']['parts'][0]['text']
            
            return None
        except Exception as e:
            print(f"Error calling Gemini API: {e}")
            return None
    
    def generate_ai_analysis(self):
        """Generate AI analysis report"""
        test_results = self.read_file('test_results.txt')
        analysis_data = self.read_file('analysis_report.md')
        
        prompt = f"""You are an experienced Go developer and code reviewer. Please analyze the following code quality report and provide actionable recommendations.

Project Context: This is a Go project called "catmit" (AI-powered commit message generator).

## Test Results:
{test_results or 'No test failures detected.'}

## Static Analysis Report:
{analysis_data or 'No static analysis issues detected.'}

Please provide:
1. **Priority Assessment**: Sort issues by urgency (Critical/High/Medium/Low)
2. **Root Cause Analysis**: What could be causing these issues?
3. **Specific Actions**: Concrete steps to fix each issue
4. **Implementation Order**: Best sequence for solving improvements
5. **Prevention Recommendations**: How to avoid these issues in the future

Focus on practical, actionable advice suitable for Go projects. Be concise but comprehensive.
Format your response using clear markdown headings and bullet points."""
        
        ai_report = self.call_gemini_api(prompt)
        
        if not ai_report:
            print("Using fallback analysis engine")
            insights = []
            priority = 'medium'
            
            if 'FAIL' in test_results:
                priority = 'critical'
                insights.append('🚨 **Critical**: Test failures detected - fix immediately')
            
            if self.has_complexity:
                insights.append('🔄 **High**: Complex functions found - consider refactoring')
            
            if self.has_const_issues:
                insights.append('📋 **Medium**: Duplicate constants - extract as named constants')
            
            if self.has_large_files:
                insights.append('📁 **Medium**: Large files detected - consider splitting')
            
            if self.has_static_issues:
                insights.append('🔍 **High**: Static analysis issues - check staticcheck warnings')
            
            ai_report = f"""## 🤖 AI Code Review Summary (Fallback Analysis)

**Priority**: {priority.upper()}

### Identified Issues:
{chr(10).join(insights)}

### Recommended Actions:
1. Fix test failures first (highest priority)
2. Address staticcheck warnings
3. Refactor complex functions
4. Extract duplicate constants
5. Split large files

*Configure GEMINI_API_KEY for detailed AI insights.*"""
        
        return ai_report
    
    def create_test_failure_issue(self):
        """Create test failure issue"""
        test_results = self.read_file('test_results.txt')
        ai_report = self.generate_ai_analysis()
        
        issue_data = {
            "title": f"🚨 Test Failure ({self.sha[:7]})",
            "body": f"""## Test Failure Detected

Commit: {self.sha}

{ai_report}

### Test Results:
<details>
<summary>Click to view detailed test output</summary>

```
{test_results}
```
</details>

---
*This issue was automatically created by AI-powered code analysis.*""",
            "labels": ["bug", "tests", "automated", "ai-reviewed"]
        }
        
        result = self.github.request('POST', '/issues', issue_data)
        if result:
            print(f"Created issue #{result['number']}")
            return result['number']
        return None
    
    def create_code_quality_pr(self):
        """Create code quality improvement PR"""
        analysis_report = self.read_file('analysis_report.md')
        ai_report = self.generate_ai_analysis()
        branch_name = f"code-quality-fixes-{self.sha[:7]}"
        
        try:
            # Create new branch
            ref_data = {
                "ref": f"refs/heads/{branch_name}",
                "sha": self.sha
            }
            self.github.request('POST', '/git/refs', ref_data)
            
            # Create analysis file content
            analysis_content = f"""# Code Quality Analysis Report

Generated: {datetime.now().isoformat()}
Commit: {self.sha}

{analysis_report}

## AI Analysis:
{ai_report}

## Recommended Actions:
1. **Reduce Complexity**: Break down complex functions into smaller ones
2. **Extract Constants**: Define constants for repeated literal values
3. **Split Large Files**: Consider splitting large files into smaller modules
4. **Fix Static Issues**: Address issues found by staticcheck

## Next Steps:
- Review the above findings
- Apply suggested refactoring
- Ensure all tests still pass
- Update this file when complete
"""
            
            # Create file
            import base64
            file_data = {
                "message": "Add code quality analysis report",
                "content": base64.b64encode(analysis_content.encode()).decode(),
                "branch": branch_name
            }
            self.github.request('PUT', '/contents/CODE_QUALITY_ANALYSIS.md', file_data)
            
            # Create PR
            pr_data = {
                "title": f"🔧 Code Quality Improvements ({self.sha[:7]})",
                "head": branch_name,
                "base": "main",
                "body": f"""## Automated Code Quality Review

This PR was automatically created after detecting code quality issues in commit {self.sha}.

{ai_report}

### Detailed Analysis:
<details>
<summary>Click to view technical details</summary>

{analysis_report}
</details>

### This PR contains:
- Added `CODE_QUALITY_ANALYSIS.md` with detailed findings
- AI-generated recommendations and action plan

### Required Actions:
- [ ] Review the AI analysis above
- [ ] Follow the recommended implementation order
- [ ] Apply suggested refactoring
- [ ] Run tests to ensure changes don't break functionality
- [ ] Update or remove analysis file after addressing issues

**Note**: This is an automated PR with AI-assisted analysis. Please review carefully before merging.""",
                "draft": True
            }
            
            pr_result = self.github.request('POST', '/pulls', pr_data)
            if not pr_result:
                return None
            
            # Add labels
            labels_data = {
                "labels": ["automated", "code-quality", "refactoring", "ai-reviewed"]
            }
            self.github.request('POST', f"/issues/{pr_result['number']}/labels", labels_data)
            
            # Add comment
            comment_data = {
                "body": """## 🤖 Code Quality Analysis

I've analyzed the code and found some areas for improvement. Please review the `CODE_QUALITY_ANALYSIS.md` file added in this PR for detailed findings.

If you have any questions about the recommendations, feel free to ask! You can also push new commits to this branch to run additional analysis."""
            }
            self.github.request('POST', f"/issues/{pr_result['number']}/comments", comment_data)
            
            print(f"Created PR #{pr_result['number']}: {pr_result['html_url']}")
            return pr_result['number']
            
        except Exception as e:
            print(f"Failed to create PR: {e}")
            return None
    
    def run(self):
        """Run code review process"""
        print("Starting code review process...")
        print(f"Test exit code: {self.test_exit_code}")
        print(f"Complexity issues: {self.has_complexity}")
        print(f"Constant issues: {self.has_const_issues}")
        print(f"Large files: {self.has_large_files}")
        print(f"Static issues: {self.has_static_issues}")
        
        # If tests fail, create issue
        if self.test_exit_code != '0':
            print("Test failure detected, creating issue...")
            self.create_test_failure_issue()
        
        # If there are code quality issues and tests pass, create PR
        if (self.test_exit_code == '0' and 
            (self.has_complexity or self.has_const_issues or 
             self.has_large_files or self.has_static_issues)):
            print("Code quality issues detected, creating PR...")
            self.create_code_quality_pr()
        
        print("Code review process completed.")

if __name__ == "__main__":
    try:
        bot = CodeReviewBot()
        bot.run()
    except Exception as e:
        print(f"Script execution failed: {e}")
        sys.exit(1)