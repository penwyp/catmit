package pr

import (
	"context"
	"testing"

	"github.com/penwyp/catmit/internal/provider"
	"github.com/stretchr/testify/assert"
)

func TestCommandBuilder_BuildGongfengMRCommand(t *testing.T) {
	tests := []struct {
		name          string
		options       PROptions
		expectedArgs  []string
		expectedError bool
	}{
		{
			name: "title and body skip quick mode",
			options: PROptions{
				Title:       "feat: add chatroom",
				Body:        "details",
				BaseBranch:  "master",
				TargetOwner: "storage-hub",
				TargetRepo:  "storage-aiops",
			},
			expectedArgs: []string{
				"mr", "create", "-R", "storage-hub/storage-aiops", "-T", "master",
				"-t", "feat: add chatroom", "-d", "details",
			},
		},
		{
			name:         "missing body falls back to quick mode",
			options:      PROptions{Title: "feat: x", BaseBranch: "master"},
			expectedArgs: []string{"mr", "create", "-T", "master", "-t", "feat: x", "-q"},
		},
		{
			name:         "fill uses quick mode",
			options:      PROptions{Fill: true, BaseBranch: "master"},
			expectedArgs: []string{"mr", "create", "-T", "master", "-q"},
		},
		{
			name: "reviewers",
			options: PROptions{
				Title: "t", Body: "b", BaseBranch: "master", Reviewers: []string{"a", "b"},
			},
			expectedArgs: []string{"mr", "create", "-T", "master", "-t", "t", "-d", "b", "-r", "a,b"},
		},
		{
			name:          "base branch required",
			options:       PROptions{Title: "t", Body: "b"},
			expectedError: true,
		},
		{
			name:          "draft unsupported",
			options:       PROptions{Title: "t", Body: "b", BaseBranch: "master", Draft: true},
			expectedError: true,
		},
	}

	builder := NewCommandBuilder()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, args, err := builder.BuildCommand("gongfeng", tt.options)
			if tt.expectedError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, "gf", cmd)
			assert.Equal(t, tt.expectedArgs, args)
		})
	}
}

func TestCommandBuilder_ParseGongfengMROutput(t *testing.T) {
	builder := NewCommandBuilder()

	t.Run("strips ADTAG query", func(t *testing.T) {
		url, err := builder.ParseGongfengMROutput("\n\n\x1b[90mOpening https://git.woa.com/storage-hub/storage-aiops/merge_requests/7?ADTAG=gf-cli\x1b[39m")
		assert.NoError(t, err)
		assert.Equal(t, "https://git.woa.com/storage-hub/storage-aiops/merge_requests/7", url)
	})

	t.Run("no URL", func(t *testing.T) {
		_, err := builder.ParseGongfengMROutput("Another open merge request already exists")
		assert.Error(t, err)
	})
}

func TestCreator_checkGongfengMR(t *testing.T) {
	ctx := context.Background()
	remoteInfo := provider.RemoteInfo{
		Provider: "gongfeng",
		Host:     "git.woa.com",
		Owner:    "storage-hub",
		Repo:     "storage-aiops",
	}
	showOutput := "Open•[!3] feat: chatroom\nfeat/chatroom -> master\n\n\n作者: penwu\n\n\n" +
		"在 https://git.woa.com/storage-hub/storage-aiops/merge_requests/3?ADTAG=gf-cli 链接查看此合并请求"

	t.Run("open MR exists", func(t *testing.T) {
		runner := new(MockCommandRunner)
		creator := &Creator{commandRunner: runner}
		runner.On("Run", ctx, "gf", "mr", "show", "feat/chatroom", "-R", "storage-hub/storage-aiops").
			Return([]byte(showOutput), nil).Once()

		exists, url, err := creator.checkGongfengMRWithBase(ctx, "feat/chatroom", "master", remoteInfo)
		assert.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, "https://git.woa.com/storage-hub/storage-aiops/merge_requests/3", url)
		runner.AssertExpectations(t)
	})

	t.Run("target branch mismatch", func(t *testing.T) {
		runner := new(MockCommandRunner)
		creator := &Creator{commandRunner: runner}
		runner.On("Run", ctx, "gf", "mr", "show", "feat/chatroom", "-R", "storage-hub/storage-aiops").
			Return([]byte(showOutput), nil).Once()

		exists, _, err := creator.checkGongfengMRWithBase(ctx, "feat/chatroom", "develop", remoteInfo)
		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("no MR found", func(t *testing.T) {
		runner := new(MockCommandRunner)
		creator := &Creator{commandRunner: runner}
		runner.On("Run", ctx, "gf", "mr", "show", "feat/chatroom", "-R", "storage-hub/storage-aiops").
			Return([]byte("没有找到源分支为 \"feat/chatroom\" 的合并请求"), nil).Once()

		exists, url, err := creator.checkGongfengMRWithBase(ctx, "feat/chatroom", "master", remoteInfo)
		assert.NoError(t, err)
		assert.False(t, exists)
		assert.Empty(t, url)
	})
}
