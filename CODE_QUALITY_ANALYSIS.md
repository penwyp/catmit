# Code Quality Analysis Report

Generated: 2025-07-16T15:12:26.421228
Commit: 977bd11e488f829934d3029fd189a16295e058ca

## Code Analysis Results

### Cyclomatic Complexity (>15)
```
50 cmd run cmd/root.go:366:1
26 ui (*MainModel).Update ui/main_model.go:125:1
24 ui (*MainModel).renderCommitContent ui/main_model.go:505:1
18 ui (*ReviewModel).Update ui/review.go:72:1
16 collector (*Collector).AnalyzeChanges collector/collector.go:894:1
```

### Duplicate Constants
```
ui/review.go:104:8:1 other occurrence(s) of "left" found in: ui/main_model.go:300:7
ui/main_model.go:300:7:1 other occurrence(s) of "left" found in: ui/review.go:104:8
cmd/root.go:439:46:1 other occurrence(s) of "PR URL: %s\n" found in: cmd/root.go:446:45
cmd/root.go:446:45:1 other occurrence(s) of "PR URL: %s\n" found in: cmd/root.go:439:46
ui/commit.go:239:68:1 other occurrence(s) of "Pushing to remote..." found in: ui/main_model.go:528:81
ui/main_model.go:528:81:1 other occurrence(s) of "Pushing to remote..." found in: ui/commit.go:239:68
collector/collector.go:608:30:1 other occurrence(s) of "## " found in: collector/collector.go:609:43
collector/collector.go:609:43:1 other occurrence(s) of "## " found in: collector/collector.go:608:30
collector/collector.go:731:49:1 other occurrence(s) of "--others" found in: collector/collector.go:1008:59
collector/collector.go:1008:59:1 other occurrence(s) of "--others" found in: collector/collector.go:731:49
collector/collector.go:767:51:1 other occurrence(s) of "--porcelain" found in: collector/collector.go:885:90
collector/collector.go:885:90:1 other occurrence(s) of "--porcelain" found in: collector/collector.go:767:51
collector/collector.go:330:36:1 other occurrence(s) of "permission denied" found in: collector/collector.go:463:30
collector/collector.go:463:30:1 other occurrence(s) of "permission denied" found in: collector/collector.go:330:36
collector/collector.go:803:14:1 other occurrence(s) of ".py" found in: collector/collector.go:1228:14
collector/collector.go:1228:14:1 other occurrence(s) of ".py" found in: collector/collector.go:803:14
collector/collector.go:803:21:1 other occurrence(s) of ".js" found in: collector/collector.go:1228:21
collector/collector.go:1228:21:1 other occurrence(s) of ".js" found in: collector/collector.go:803:21
collector/collector.go:909:26:1 other occurrence(s) of "failed to get untracked files: %w" found in: collector/collector.go:1114:25
collector/collector.go:1114:25:1 other occurrence(s) of "failed to get untracked files: %w" found in: collector/collector.go:909:26
collector/collector.go:806:7:1 other occurrence(s) of ".md" found in: collector/collector.go:1232:7
collector/collector.go:1232:7:1 other occurrence(s) of ".md" found in: collector/collector.go:806:7
ui/commit.go:204:83:1 other occurrence(s) of " (%s)" found in: ui/review.go:243:82
ui/review.go:243:82:1 other occurrence(s) of " (%s)" found in: ui/commit.go:204:83
ui/components.go:207:26:1 other occurrence(s) of "─" found in: ui/commit.go:209:74
ui/commit.go:209:74:1 other occurrence(s) of "─" found in: ui/components.go:207:26
cmd/root.go:40:21:1 other occurrence(s) of "pull request already exists: %s" found in: ui/main_model.go:712:21
ui/main_model.go:712:21:1 other occurrence(s) of "pull request already exists: %s" found in: cmd/root.go:40:21
collector/collector.go:726:37:1 other occurrence(s) of "diff" found in: collector/collector.go:856:81
collector/collector.go:856:81:1 other occurrence(s) of "diff" found in: collector/collector.go:726:37
cmd/root.go:163:41:1 other occurrence(s) of "add" found in: cmd/root.go:622:41
cmd/root.go:622:41:1 other occurrence(s) of "add" found in: cmd/root.go:163:41
collector/collector.go:731:61:1 other occurrence(s) of "--exclude-standard" found in: collector/collector.go:1008:71
collector/collector.go:1008:71:1 other occurrence(s) of "--exclude-standard" found in: collector/collector.go:731:61
ui/components.go:24:26:1 other occurrence(s) of "245" found in: ui/commit.go:170:28
ui/commit.go:170:28:1 other occurrence(s) of "245" found in: ui/components.go:24:26
collector/collector.go:726:45:1 other occurrence(s) of "--cached" found in: collector/collector.go:856:89
collector/collector.go:856:89:1 other occurrence(s) of "--cached" found in: collector/collector.go:726:45
prompt/prompt.go:176:10:1 other occurrence(s) of "No changes detected." found in: prompt/prompt.go:239:10
prompt/prompt.go:239:10:1 other occurrence(s) of "No changes detected." found in: prompt/prompt.go:176:10
ui/commit.go:200:29:1 other occurrence(s) of "│" found in: ui/commit.go:200:102
ui/commit.go:200:102:1 other occurrence(s) of "│" found in: ui/commit.go:200:29
collector/collector.go:667:34:1 other occurrence(s) of "git" found in: collector/collector.go:706:34
collector/collector.go:706:34:1 other occurrence(s) of "git" found in: collector/collector.go:667:34
collector/collector.go:809:16:1 other occurrence(s) of ".css" found in: collector/collector.go:1234:16
collector/collector.go:1234:16:1 other occurrence(s) of ".css" found in: collector/collector.go:809:16
ui/components.go:28:26:1 other occurrence(s) of "196" found in: ui/review.go:178:28
ui/review.go:178:28:1 other occurrence(s) of "196" found in: ui/components.go:28:26
collector/collector.go:806:40:1 other occurrence(s) of ".yml" found in: collector/collector.go:1230:25
collector/collector.go:1230:25:1 other occurrence(s) of ".yml" found in: collector/collector.go:806:40
collector/collector.go:803:50:1 other occurrence(s) of ".cpp" found in: collector/collector.go:1228:50
collector/collector.go:1228:50:1 other occurrence(s) of ".cpp" found in: collector/collector.go:803:50
ui/commit.go:204:33:1 other occurrence(s) of "Commit Progress" found in: ui/main_model.go:638:10
ui/main_model.go:638:10:1 other occurrence(s) of "Commit Progress" found in: ui/commit.go:204:33
collector/collector.go:1188:10:1 other occurrence(s) of "chore" found in: collector/collector.go:1199:9
collector/collector.go:1199:9:1 other occurrence(s) of "chore" found in: collector/collector.go:1188:10
cmd/root.go:141:41:1 other occurrence(s) of "push" found in: cmd/root.go:356:38
cmd/root.go:356:38:1 other occurrence(s) of "push" found in: cmd/root.go:141:41
ui/commit.go:227:69:1 other occurrence(s) of "Committing changes..." found in: ui/main_model.go:520:79
ui/main_model.go:520:79:1 other occurrence(s) of "Committing changes..." found in: ui/commit.go:227:69
ui/commit.go:209:31:1 other occurrence(s) of "┌" found in: ui/review.go:248:31
ui/review.go:248:31:1 other occurrence(s) of "┌" found in: ui/commit.go:209:31
ui/components.go:29:26:1 other occurrence(s) of "255" found in: ui/commit.go:174:28
ui/commit.go:174:28:1 other occurrence(s) of "255" found in: ui/components.go:29:26
ui/commit.go:99:22:1 other occurrence(s) of "ctrl+c" found in: ui/loading.go:102:8
ui/loading.go:102:8:1 other occurrence(s) of "ctrl+c" found in: ui/commit.go:99:22
collector/collector.go:818:35:1 other occurrence(s) of "test" found in: collector/collector.go:1222:28
collector/collector.go:1222:28:1 other occurrence(s) of "test" found in: collector/collector.go:818:35
ui/review.go:92:9:1 other occurrence(s) of "enter" found in: ui/review.go:127:8
ui/review.go:127:8:1 other occurrence(s) of "enter" found in: ui/review.go:92:9
ui/loading.go:189:12:1 other occurrence(s) of "Processing…" found in: ui/main_model.go:439:12
ui/main_model.go:439:12:1 other occurrence(s) of "Processing…" found in: ui/loading.go:189:12
client/client.go:238:13:1 other occurrence(s) of "system_prompt_length" found in: client/client.go:244:13
client/client.go:244:13:1 other occurrence(s) of "system_prompt_length" found in: client/client.go:238:13
collector/collector.go:803:7:1 other occurrence(s) of ".go" found in: collector/collector.go:1228:7
collector/collector.go:1228:7:1 other occurrence(s) of ".go" found in: collector/collector.go:803:7
ui/commit.go:245:47:1 other occurrence(s) of "Pushed successfully" found in: ui/main_model.go:546:60
ui/main_model.go:546:60:1 other occurrence(s) of "Pushed successfully" found in: ui/commit.go:245:47
collector/collector.go:806:14:1 other occurrence(s) of ".txt" found in: collector/collector.go:1232:14
collector/collector.go:1232:14:1 other occurrence(s) of ".txt" found in: collector/collector.go:806:14
collector/collector.go:856:101:1 other occurrence(s) of "--no-ext-diff" found in: collector/collector.go:863:74
collector/collector.go:863:74:1 other occurrence(s) of "--no-ext-diff" found in: collector/collector.go:856:101
ui/loading.go:183:12:1 other occurrence(s) of "Crafting prompt…" found in: ui/main_model.go:433:12
ui/main_model.go:433:12:1 other occurrence(s) of "Crafting prompt…" found in: ui/loading.go:183:12
collector/collector.go:803:58:1 other occurrence(s) of ".rs" found in: collector/collector.go:1228:58
collector/collector.go:1228:58:1 other occurrence(s) of ".rs" found in: collector/collector.go:803:58
ui/loading.go:180:12:1 other occurrence(s) of "Preprocessing files…" found in: ui/main_model.go:430:12
ui/main_model.go:430:12:1 other occurrence(s) of "Preprocessing files…" found in: ui/loading.go:180:12
client/client.go:237:16:1 other occurrence(s) of "user_prompt_preview" found in: client/client.go:243:16
client/client.go:243:16:1 other occurrence(s) of "user_prompt_preview" found in: client/client.go:237:16
collector/collector.go:565:33:1 other occurrence(s) of ".tmp" found in: collector/collector.go:569:69
collector/collector.go:569:69:1 other occurrence(s) of ".tmp" found in: collector/collector.go:565:33
collector/collector.go:806:31:1 other occurrence(s) of ".yaml" found in: collector/collector.go:1230:16
collector/collector.go:1230:16:1 other occurrence(s) of ".yaml" found in: collector/collector.go:806:31
collector/collector.go:809:24:1 other occurrence(s) of ".scss" found in: collector/collector.go:1234:24
collector/collector.go:1234:24:1 other occurrence(s) of ".scss" found in: collector/collector.go:809:24
ui/main_model.go:556:81:1 other occurrence(s) of "Creating pull request..." found in: cmd/root.go:433:61
cmd/root.go:433:61:1 other occurrence(s) of "Creating pull request..." found in: ui/main_model.go:556:81
cmd/root.go:226:69:1 other occurrence(s) of "/pull/" found in: cmd/root.go:230:79
cmd/root.go:230:79:1 other occurrence(s) of "/pull/" found in: cmd/root.go:226:69
ui/components.go:31:26:1 other occurrence(s) of "208" found in: ui/loading.go:181:63
ui/loading.go:181:63:1 other occurrence(s) of "208" found in: ui/components.go:31:26
client/client.go:239:13:1 other occurrence(s) of "user_prompt_length" found in: client/client.go:245:13
client/client.go:245:13:1 other occurrence(s) of "user_prompt_length" found in: client/client.go:239:13
collector/collector.go:803:65:1 other occurrence(s) of ".rb" found in: collector/collector.go:1228:65
collector/collector.go:1228:65:1 other occurrence(s) of ".rb" found in: collector/collector.go:803:65
cmd/root.go:170:61:1 other occurrence(s) of "--quiet" found in: cmd/root.go:630:61
cmd/root.go:630:61:1 other occurrence(s) of "--quiet" found in: cmd/root.go:170:61
client/client.go:236:16:1 other occurrence(s) of "system_prompt_preview" found in: client/client.go:242:16
client/client.go:242:16:1 other occurrence(s) of "system_prompt_preview" found in: client/client.go:236:16
collector/collector.go:806:48:1 other occurrence(s) of ".xml" found in: collector/collector.go:1230:33
collector/collector.go:1230:33:1 other occurrence(s) of ".xml" found in: collector/collector.go:806:48
ui/components.go:27:26:1 other occurrence(s) of "220" found in: ui/commit.go:173:28
ui/commit.go:173:28:1 other occurrence(s) of "220" found in: ui/components.go:27:26
ui/review.go:95:9:1 other occurrence(s) of "esc" found in: ui/review.go:122:28
ui/review.go:122:28:1 other occurrence(s) of "esc" found in: ui/review.go:95:9
ui/commit.go:230:47:1 other occurrence(s) of "Committed successfully" found in: ui/commit.go:237:47
ui/commit.go:237:47:1 other occurrence(s) of "Committed successfully" found in: ui/commit.go:230:47
collector/collector.go:1194:10:1 other occurrence(s) of "fix" found in: collector/collector.go:1310:10
collector/collector.go:1310:10:1 other occurrence(s) of "fix" found in: collector/collector.go:1194:10
cmd/root.go:438:63:1 other occurrence(s) of "Pull request already exists" found in: cmd/root.go:546:62
cmd/root.go:546:62:1 other occurrence(s) of "Pull request already exists" found in: cmd/root.go:438:63
collector/collector.go:767:41:1 other occurrence(s) of "status" found in: collector/collector.go:885:80
collector/collector.go:885:80:1 other occurrence(s) of "status" found in: collector/collector.go:767:41
ui/commit.go:233:69:1 other occurrence(s) of "Preparing to push..." found in: ui/main_model.go:524:82
ui/main_model.go:524:82:1 other occurrence(s) of "Preparing to push..." found in: ui/commit.go:233:69
collector/collector.go:1191:10:1 other occurrence(s) of "refactor" found in: collector/collector.go:1306:10
collector/collector.go:1306:10:1 other occurrence(s) of "refactor" found in: collector/collector.go:1191:10
cmd/root.go:450:44:1 other occurrence(s) of "Nothing to commit." found in: cmd/root.go:469:45
cmd/root.go:469:45:1 other occurrence(s) of "Nothing to commit." found in: cmd/root.go:450:44
collector/collector.go:803:28:1 other occurrence(s) of ".ts" found in: collector/collector.go:1228:28
collector/collector.go:1228:28:1 other occurrence(s) of ".ts" found in: collector/collector.go:803:28
ui/review.go:109:22:1 other occurrence(s) of "down" found in: ui/main_model.go:305:21
ui/main_model.go:305:21:1 other occurrence(s) of "down" found in: ui/review.go:109:22
collector/collector.go:706:54:1 other occurrence(s) of "--abbrev-ref" found in: cmd/root.go:240:54
cmd/root.go:240:54:1 other occurrence(s) of "--abbrev-ref" found in: collector/collector.go:706:54
prompt/prompt.go:197:25:1 other occurrence(s) of "failed to get file status summary: %w" found in: collector/collector.go:897:26
collector/collector.go:897:26:1 other occurrence(s) of "failed to get file status summary: %w" found in: prompt/prompt.go:197:25
collector/collector.go:706:41:1 other occurrence(s) of "rev-parse" found in: cmd/root.go:240:41
cmd/root.go:240:41:1 other occurrence(s) of "rev-parse" found in: collector/collector.go:706:41
collector/collector.go:451:30:1 other occurrence(s) of "timeout" found in: cmd/root.go:352:40
cmd/root.go:352:40:1 other occurrence(s) of "timeout" found in: collector/collector.go:451:30
collector/collector.go:809:7:1 other occurrence(s) of ".html" found in: collector/collector.go:1234:7
collector/collector.go:1234:7:1 other occurrence(s) of ".html" found in: collector/collector.go:809:7
ui/commit.go:221:71:1 other occurrence(s) of "Message: " found in: ui/main_model.go:515:50
ui/main_model.go:515:50:1 other occurrence(s) of "Message: " found in: ui/commit.go:221:71
ui/commit.go:211:22:1 other occurrence(s) of "┐" found in: ui/review.go:250:22
ui/review.go:250:22:1 other occurrence(s) of "┐" found in: ui/commit.go:211:22
collector/collector.go:809:33:1 other occurrence(s) of ".less" found in: collector/collector.go:1234:33
collector/collector.go:1234:33:1 other occurrence(s) of ".less" found in: collector/collector.go:809:33
ui/loading.go:177:12:1 other occurrence(s) of "Collecting diff…" found in: ui/main_model.go:427:12
ui/main_model.go:427:12:1 other occurrence(s) of "Collecting diff…" found in: ui/loading.go:177:12
collector/collector.go:806:22:1 other occurrence(s) of ".json" found in: collector/collector.go:1230:7
collector/collector.go:1230:7:1 other occurrence(s) of ".json" found in: collector/collector.go:806:22
cmd/root.go:116:15:1 other occurrence(s) of "output" found in: cmd/root.go:149:16
cmd/root.go:149:16:1 other occurrence(s) of "output" found in: cmd/root.go:116:15
collector/collector.go:731:37:1 other occurrence(s) of "ls-files" found in: collector/collector.go:1008:47
collector/collector.go:1008:47:1 other occurrence(s) of "ls-files" found in: collector/collector.go:731:37
cmd/root.go:442:25:1 other occurrence(s) of "failed to create pull request: %w" found in: cmd/root.go:550:24
cmd/root.go:550:24:1 other occurrence(s) of "failed to create pull request: %w" found in: cmd/root.go:442:25
collector/collector.go:1185:10:1 other occurrence(s) of "feat" found in: collector/collector.go:1197:10
collector/collector.go:1197:10:1 other occurrence(s) of "feat" found in: collector/collector.go:1185:10
ui/review.go:243:33:1 other occurrence(s) of "Commit Preview" found in: ui/main_model.go:636:10
ui/main_model.go:636:10:1 other occurrence(s) of "Commit Preview" found in: ui/review.go:243:33
ui/main_model.go:580:60:1 other occurrence(s) of "Pull request created successfully" found in: cmd/root.go:444:61
cmd/root.go:444:61:1 other occurrence(s) of "Pull request created successfully" found in: ui/main_model.go:580:60
ui/review.go:109:8:1 other occurrence(s) of "right" found in: ui/main_model.go:305:7
ui/main_model.go:305:7:1 other occurrence(s) of "right" found in: ui/review.go:109:8
collector/collector.go:818:76:1 other occurrence(s) of "spec" found in: collector/collector.go:1222:62
collector/collector.go:1222:62:1 other occurrence(s) of "spec" found in: collector/collector.go:818:76
ui/loading.go:186:12:1 other occurrence(s) of "Generating commit message…" found in: ui/main_model.go:436:12
ui/main_model.go:436:12:1 other occurrence(s) of "Generating commit message…" found in: ui/loading.go:186:12
collector/collector.go:803:35:1 other occurrence(s) of ".java" found in: collector/collector.go:1228:35
collector/collector.go:1228:35:1 other occurrence(s) of ".java" found in: collector/collector.go:803:35
```

### Large Files
```
total (3366 lines) [TEST FILE]
./collector/collector.go (1440 lines) [SOURCE FILE]
total (4796 lines) [SOURCE FILE]
```

### Static Analysis Issues
```
collector/collector.go:579:6: func filterFiles is unused (U1000)
ui/components.go:186:12: hStyle.Copy is deprecated: to copy just use assignment (i.e. a := b). All methods also return a new style.  (SA1019)
ui/components.go:187:12: tStyle.Copy is deprecated: to copy just use assignment (i.e. a := b). All methods also return a new style.  (SA1019)
ui/review.go:232:13: hStyle.Copy is deprecated: to copy just use assignment (i.e. a := b). All methods also return a new style.  (SA1019)
ui/review.go:233:13: tStyle.Copy is deprecated: to copy just use assignment (i.e. a := b). All methods also return a new style.  (SA1019)
```


## AI Analysis:
## 🤖 AI Code Review Summary (Fallback Analysis)

**Priority**: MEDIUM

### Identified Issues:
🔄 **High**: Complex functions found - consider refactoring
📋 **Medium**: Duplicate constants - extract as named constants
📁 **Medium**: Large files detected - consider splitting
🔍 **High**: Static analysis issues - check staticcheck warnings

### Recommended Actions:
1. Fix test failures first (highest priority)
2. Address staticcheck warnings
3. Refactor complex functions
4. Extract duplicate constants
5. Split large files

*Configure GEMINI_API_KEY for detailed AI insights.*

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
