package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestDetector_CheckAuthStatus_GitLab(t *testing.T) {
	tests := []struct {
		name           string
		cliName        string
		authCmd        string
		authArgs       []string
		mockOutput     []byte
		mockError      error
		expectedAuth   bool
		expectedUser   string
		expectedError  bool
	}{
		{
			name:       "glab authenticated",
			cliName:    "glab",
			authCmd:    "auth",
			authArgs:   []string{"status"},
			mockOutput: []byte("✓ Logged in to gitlab.com as testuser"),
			mockError:  nil,
			expectedAuth: true,
			expectedUser: "testuser",
			expectedError: false,
		},
		{
			name:       "glab not authenticated",
			cliName:    "glab",
			authCmd:    "auth",
			authArgs:   []string{"status"},
			mockOutput: []byte("No accounts configured"),
			mockError:  &mockExecError{msg: "exit status 1"},
			expectedAuth: false,
			expectedUser: "",
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRunner := new(MockCommandRunner)
			detector := NewDetector(mockRunner)
			
			mockRunner.On("Run", mock.Anything, tt.cliName, append([]string{tt.authCmd}, tt.authArgs...)).
				Return(tt.mockOutput, tt.mockError)
			
			authenticated, user, err := detector.CheckAuthStatus(context.Background(), tt.cliName, tt.authCmd, tt.authArgs...)
			
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedAuth, authenticated)
				assert.Equal(t, tt.expectedUser, user)
			}
			
			mockRunner.AssertExpectations(t)
		})
	}
}

type mockExecError struct {
	msg string
}

func (e *mockExecError) Error() string {
	return e.msg
}