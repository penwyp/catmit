package workflow

import (
	"context"
	"strings"
	"time"

	"github.com/penwyp/catmit/internal/app"
	"github.com/penwyp/catmit/internal/errors"
	tagging "github.com/penwyp/catmit/internal/tag"
	"github.com/penwyp/catmit/pkg/gitinfo"
	"go.uber.org/zap"
)

type CommitMessageOptions struct {
	Language string
	Timeout  int
	SeedText string
	Debug    bool
}

func GenerateCommitMessage(ctx context.Context, deps *app.Dependencies, opts CommitMessageOptions) (string, error) {
	col := deps.GetCollector()

	// Use ComprehensiveDiff to include untracked files.
	diffText, err := col.ComprehensiveDiff(ctx)
	if err != nil {
		if errors.Is(err, gitinfo.ErrNoDiff) {
			if opts.Debug && deps.Logger != nil {
				deps.Logger.Debug("No staged, unstaged, or untracked changes detected")
			}
			return "", err
		}
		return "", errors.Wrap(errors.ErrTypeGit, "failed to collect git diff", err)
	}

	commits, err := col.RecentCommits(ctx, 10)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeGit, "failed to process diff", err)
	}

	builder := deps.GetPromptBuilder(opts.Language)
	systemPrompt := builder.BuildSystemPrompt()

	userPrompt, err := builder.BuildUserPromptWithBudget(ctx, col, opts.SeedText)
	if err != nil {
		if opts.Debug && deps.Logger != nil {
			deps.Logger.Debug("Smart prompt building failed, falling back to traditional method", zap.Error(err))
		}
		branch, _ := col.BranchName(ctx)
		files, _ := col.ChangedFiles(ctx)
		userPrompt = builder.BuildUserPrompt(opts.SeedText, diffText, commits, branch, files)
	}

	client := deps.GetClient()
	apiCtx, apiCancel := context.WithTimeout(ctx, time.Duration(opts.Timeout)*time.Second)
	defer apiCancel()

	contentChan, errChan := client.GetCommitMessageStream(apiCtx, systemPrompt, userPrompt)

	var fullMessage strings.Builder
	for {
		select {
		case content, ok := <-contentChan:
			if !ok {
				raw := strings.TrimSpace(fullMessage.String())
				if err := tagging.ValidateConventionalCommit(raw); err != nil {
					repaired, repairErr := tagging.TryRepairCommitMessage(raw)
					if repairErr != nil {
						return "", errors.Wrap(errors.ErrTypeLLM, "LLM 输出的提交信息不符合 Conventional Commits 格式，请重试", err)
					}
					return repaired, nil
				}
				return raw, nil
			}
			fullMessage.WriteString(content)
		case err := <-errChan:
			if err != nil {
				if errors.Is(err, errors.ErrLLMTimeout) {
					return "", errors.Wrap(errors.ErrTypeTimeout, "failed to get commit message from LLM", err)
				}
				return "", errors.Wrap(errors.ErrTypeLLM, "failed to get commit message from LLM", err)
			}
		case <-apiCtx.Done():
			return "", errors.Wrap(errors.ErrTypeTimeout, "API timeout", apiCtx.Err())
		}
	}
}
