// Package ci translates pipeline environment variables into platform-independent
// trigger information. Version selection belongs to the CI workflow.
package ci

import "fmt"

type Context struct {
	Tag     string
	Trigger string
}

func Read(provider string, getenv func(string) string) (Context, error) {
	if provider != "bk-ci" {
		return Context{}, fmt.Errorf("unsupported CI provider %q; supported: bk-ci", provider)
	}
	info := Context{Tag: getenv("BK_CI_REPO_GIT_WEBHOOK_TAG_NAME")}
	switch start := getenv("BK_CI_START_TYPE"); start {
	case "WEB_HOOK":
		switch event := getenv("BK_CI_REPO_GIT_WEBHOOK_EVENT_TYPE"); event {
		case "TAG_PUSH":
			info.Trigger = "webhook(TAG_PUSH)  TAG 推送"
		case "PUSH":
			info.Trigger = "webhook(PUSH)  分支推送"
		case "MERGE_REQUEST", "MERGE_REQUEST_ACCEPT":
			info.Trigger = "webhook(MR)  MR 触发"
		case "":
			info.Trigger = "webhook  事件触发"
		default:
			info.Trigger = fmt.Sprintf("webhook(%s)  事件触发", event)
		}
	case "MANUAL":
		info.Trigger = "manual  手动触发"
	case "TIME_TRIGGER":
		info.Trigger = "schedule  定时触发"
	case "PIPELINE":
		info.Trigger = "pipeline  子流水线"
	case "SERVICE":
		info.Trigger = "service  服务调用"
	case "REMOTE":
		info.Trigger = "remote  远程触发"
	default:
		info.Trigger = "manual  手动触发"
	}
	return info, nil
}
