package ui

import (
	"testing"
)


// -------------------------------------------------------


func TestLoadingModel_Success(t *testing.T) {
	t.Skip("LoadingModel is no longer a standalone tea.Model")
	// col := mockCollector{diff: "diff", commits: []string{"feat: a"}}
	// lm := NewLoadingModel(context.Background(), col, mockPrompt{}, mockClient{msg: "feat: ok"}, "", "en", 20*time.Second)

	// finalModel, err := runModel(lm)
	// require.NoError(t, err)

	// if m, ok := finalModel.(*LoadingModel); ok {
	// 	msg, err := m.IsDone()
	// 	require.NoError(t, err)
	// 	require.Equal(t, "feat: ok", msg)
	// } else {
	// 	t.Fatalf("unexpected model type")
	// }
}

func TestLoadingModel_Error(t *testing.T) {
	t.Skip("LoadingModel is no longer a standalone tea.Model")
	// col := mockCollector{err: context.Canceled}
	// lm := NewLoadingModel(context.Background(), col, mockPrompt{}, mockClient{}, "", "en", 20*time.Second)

	// finalModel, err := runModel(lm)
	// require.NoError(t, err)
	// if m, ok := finalModel.(*LoadingModel); ok {
	// 	_, e := m.IsDone()
	// 	require.ErrorIs(t, e, context.Canceled)
	// } else {
	// 	t.Fatalf("unexpected model type")
	// }
}
