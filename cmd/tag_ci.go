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
		Long: `为 CI 准备代码、确定制品版本，并可选执行构建。无需 LLM 或 API Key。

--version-order 按从左到右的顺序尝试版本来源，命中后立即停止：
  webhook-tag  provider 提供的 webhook tag；非空即使用，优先于已有环境变量。
  env          --version-env 指定的环境变量；为空时继续。
  history      根据已提交的 Git 历史计算；失败或为空时继续后续来源。
  timestamp    使用 --timezone 时区的 YYYYMMDDHHmmss 时间戳。
默认顺序：webhook-tag,env,history,timestamp。来源可以省略、重排，不能重复。
所有来源耗尽后报错；未列出 timestamp 时不会自动使用时间戳兜底。
非法输入、tag 检出失败和构建失败始终终止；取消操作不会触发兜底。

--provider bk-ci 读取 BK_CI_START_TYPE、BK_CI_REPO_GIT_WEBHOOK_EVENT_TYPE、
BK_CI_REPO_GIT_WEBHOOK_TAG_NAME。--version-env 默认 CATMIT_VERSION；业务变量
如 ZHIYAN_PKG_VERSION 需要显式指定。普通 catmit tag 不读取这些 CI 输入。

--checkout 仅在 webhook tag 非空时从 --remote 解析该 tag 并按 commit SHA
检出 detached HEAD，不信任同名本地 tag。不强制覆盖工作区，不改非 tag 触发的 HEAD。
只有进入 history 来源时才检查完整历史；浅克隆会自动补全，远程操作受 --timeout 限制。
没有 --checkout 时保留当前代码；请确保流水线已检出正确的触发提交。

-- 后的命令直接执行，不做隐式 Shell 求值。构建前一次性确定版本，并注入
CATMIT_VERSION 和 --version-env 指定的变量；构建失败返回非零（保留命令退出码）。
不提供命令则只准备代码和版本。不会创建或推送 Git tag，也不会调用 setEnv。

输出：进度、构建 stdout/stderr、错误和中文摘要全部进入 stderr，并实时输出。
成功时 stdout 只有最终版本及换行；构建或准备失败时 stdout 为空。
版本不被保留，不保证并发唯一；后续步骤应复用本次结果，勿重新计算。
外部版本不要求 SemVer，仅校验非空、无首尾空白、单行且无控制字符；
用于 Docker tag 等制品命名时仍需按目标格式自行校验。`,
		Example: `  # 仅准备代码和版本：webhook tag > 上游变量 > Git 历史 > 时间戳
  version=$(catmit tag ci --provider bk-ci \
    --version-order webhook-tag,env,history,timestamp \
    --version-env ZHIYAN_PKG_VERSION --checkout)
  setEnv "ZHIYAN_PKG_VERSION" "$version"

  # 同一步构建；构建日志实时写入 stderr
  version=$(catmit tag ci --provider bk-ci \
    --version-order webhook-tag,env,history,timestamp \
    --version-env ZHIYAN_PKG_VERSION --checkout -- sh build.sh)

  # 禁止时间戳兜底，历史计算失败即终止
  catmit tag ci --version-order webhook-tag,env,history \
    --version-env ZHIYAN_PKG_VERSION --checkout`,
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
	flags.BoolVar(&debug, "debug", false, "enable Git debug output on stderr")
	flags.IntVarP(&options.Version.Timeout, "timeout", "t", 30, "timeout per remote Git operation in seconds")
	flags.StringVarP(&options.Version.Remote, "remote", "r", "origin", "remote for tag checkout and version history")
	flags.StringVar(&options.Version.Bump, "bump", "auto", "history source core bump (auto/patch/minor/major)")
	flags.StringVar(&options.Version.InitialVersion, "initial", "v0.1.0", "history source initial core version")
	flags.StringVar(&options.Version.Pre, "pre", "", "history source prerelease channel (default dev)")
	flags.BoolVar(&options.Version.Release, "release", false, "history source generates a stable version")
	flags.BoolVar(&options.Version.NoTimestamp, "notimestamp", false, "omit history source prerelease timestamp; keep counter")
	flags.StringVar(&options.Version.Timezone, "timezone", "Asia/Shanghai", "timezone for history and timestamp sources")
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
