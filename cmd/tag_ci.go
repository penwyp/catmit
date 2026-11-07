package cmd

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/penwyp/catmit/internal/git"
	"github.com/penwyp/catmit/internal/logger"
	"github.com/penwyp/catmit/internal/workflow"
	"github.com/spf13/cobra"
)

func newTagCICommand() *cobra.Command {
	var options workflow.TagCIOptions
	var debug bool
	cmd := &cobra.Command{
		Use:          "ci [flags] [-- command [args...]]",
		Short:        "Prepare CI code and select an artifact version in explicit source order",
		SilenceUsage: true,
		Long: `为 CI 准备代码、确定制品版本，并可选执行构建。

版本来源
  按 --version-order 从左到右尝试，命中后立即停止。
  默认：webhook-tag > env > history > timestamp

  webhook-tag
    读取蓝盾 webhook tag，非空即使用。
  env
    读取 --version-env 指定的变量，非空即使用。
  history
    根据 Git 历史计算；失败或为空时尝试下一个来源。
  timestamp
    使用指定时区，格式为 YYYYMMDDHHmmss。

  来源可以省略或重排，不能重复。没有 timestamp 就不兜底。
  非法输入、tag 检出失败、构建失败或取消操作均会终止。

代码与构建
  --provider bk-ci 读取以下蓝盾变量：
    BK_CI_START_TYPE
    BK_CI_REPO_GIT_WEBHOOK_EVENT_TYPE
    BK_CI_REPO_GIT_WEBHOOK_TAG_NAME
  业务变量由 --version-env 单独指定。
  --checkout 按远程 tag 的 commit 检出，不信任本地同名 tag。
  不强制覆盖工作区；非 tag 触发保持 HEAD。
  只有进入 history 时才补全浅克隆，其他来源无需完整历史。

  没有尾部命令：仅准备代码和版本。
  有 -- 命令：先确定版本，再注入 CATMIT_VERSION 和业务变量。
  命令直接执行；需要 Shell 语法时显式使用 sh -c。

输出约定
  stdout：成功时只有版本号及换行，失败时为空。
  stderr：实时进度、完整构建日志、错误和一次中文摘要。
  构建失败保留退出码；不创建或推送 Git tag，不调用 LLM。
  通过 setEnv 向后续步骤传递结果；不要重新计算版本。
  不保证并发唯一；外部版本需单行、无首尾空白或控制字符。
  用于 Docker tag 等制品名称时，还需按目标格式校验。`,
		Example: `  # 仅准备代码与版本
  catmit tag ci \
    --provider bk-ci \
    --version-order \
    webhook-tag,env,history,timestamp \
    --version-env ZHIYAN_PKG_VERSION \
    --checkout

  # 准备后构建；日志实时进入 stderr
  catmit tag ci \
    --provider bk-ci \
    --version-order \
    webhook-tag,env,history,timestamp \
    --version-env ZHIYAN_PKG_VERSION \
    --checkout -- sh build.sh`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && cmd.ArgsLenAtDash() != 0 {
				return fmt.Errorf("build commands must follow --, for example: catmit tag ci -- sh build.sh")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := runTagCI(cmd, options, debug, args); err != nil {
				code := 1
				var exitErr *exec.ExitError
				switch {
				case errors.Is(cmd.Context().Err(), context.Canceled):
					code = 130
				case errors.Is(cmd.Context().Err(), context.DeadlineExceeded):
					code = 124
				case errors.As(err, &exitErr) && exitErr.ExitCode() > 0:
					code = exitErr.ExitCode()
				}
				return &ExitError{Code: code, Err: err}
			}
			return nil
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&options.Provider, "provider", "bk-ci", "CI 环境适配器（bk-ci）")
	flags.StringVar(&options.VersionOrder, "version-order", workflow.DefaultVersionOrder, "从左到右选择版本来源，命中即停止")
	flags.StringVar(&options.VersionEnv, "version-env", "CATMIT_VERSION", "env 来源读取的变量名，同时向构建注入该变量")
	flags.BoolVar(&options.Checkout, "checkout", false, "tag 触发时检出远程 tag 对应的 commit")
	flags.BoolVar(&debug, "debug", false, "将 Git 调试日志写入 stderr")
	flags.IntVarP(&options.Version.Timeout, "timeout", "t", 30, "单次远程 Git 操作的超时秒数")
	flags.StringVarP(&options.Version.Remote, "remote", "r", "origin", "检出 tag 和读取版本历史的远程")
	flags.StringVar(&options.Version.Bump, "bump", "auto", "核心版本递增：auto / patch / minor / major")
	flags.StringVar(&options.Version.InitialVersion, "initial", "v0.1.0", "没有版本历史时的初始核心版本")
	flags.StringVar(&options.Version.Pre, "pre", "", "预发布通道，未指定时使用 dev")
	flags.BoolVar(&options.Version.Release, "release", false, "生成正式版本，不附加预发布后缀")
	flags.BoolVar(&options.Version.NoTimestamp, "notimestamp", false, "去掉预发布时间戳，保留递增序号")
	flags.StringVar(&options.Version.Timezone, "timezone", "Asia/Shanghai", "历史版本与时间戳来源共用的时区")
	cmd.InitDefaultHelpFlag()
	cmd.Flags().Lookup("help").Usage = "显示帮助"
	cmd.SetHelpFunc(renderTagCIHelp)
	cmd.MarkFlagsMutuallyExclusive("pre", "release")
	return cmd
}

func runTagCI(cmd *cobra.Command, options workflow.TagCIOptions, debug bool, args []string) error {
	if err := options.Validate(); err != nil {
		return err
	}
	appLogger, err := logger.NewStderr(debug)
	if err != nil {
		return err
	}
	defer func() { _ = appLogger.Sync() }()
	var runner git.Runner
	if testDeps != nil {
		runner = testDeps.GetGitRunner()
	} else {
		runner = git.NewRunnerWithLogger(debug, appLogger)
	}
	return workflow.NewTagCIWorkflow(runner, options, cmd.OutOrStdout(), cmd.ErrOrStderr(), cmd.InOrStdin()).Run(cmd.Context(), args)
}
