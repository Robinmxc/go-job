package utils

import (
	"errors"
	"os/user"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockUserLooker struct {
	lookupFunc   func(username string) (*user.User, error)
	lookupIdFunc func(uid string) (*user.User, error)
}

func (m *mockUserLooker) Lookup(username string) (*user.User, error) {
	return m.lookupFunc(username)
}

func (m *mockUserLooker) LookupId(uid string) (*user.User, error) {
	return m.lookupIdFunc(uid)
}

func TestLookupUser(t *testing.T) {
	tests := []struct {
		name         string
		username     string
		mockLookup   func(string) (*user.User, error)
		mockLookupID func(string) (*user.User, error)
		expected     *userInfo
		expectError  bool
	}{
		{
			name:     "empty username",
			username: "",
			mockLookup: func(_ string) (*user.User, error) {
				return nil, nil
			},
			expectError: true,
		},
		{
			name:     "successful lookup by username",
			username: "test-user",
			mockLookup: func(_ string) (*user.User, error) {
				return &user.User{
					Uid:      "1000",
					Gid:      "1000",
					Username: "test-user",
					HomeDir:  "/home/test-user",
				}, nil
			},
			expected: &userInfo{
				Uid:  "1000",
				Gid:  "1000",
				Name: "test-user",
				Home: "/home/test-user",
			},
			expectError: false,
		},
		{
			name:     "failed lookup by username, successful by uid",
			username: "1000",
			mockLookup: func(_ string) (*user.User, error) {
				return nil, errors.New("user not found")
			},
			mockLookupID: func(_ string) (*user.User, error) {
				return &user.User{
					Uid:      "1000",
					Gid:      "1000",
					Username: "test-user",
					HomeDir:  "/home/test-user",
				}, nil
			},
			expected: &userInfo{
				Uid:  "1000",
				Gid:  "1000",
				Name: "test-user",
				Home: "/home/test-user",
			},
			expectError: false,
		},
		{
			name:     "both lookups fail",
			username: "nonexistent",
			mockLookup: func(_ string) (*user.User, error) {
				return nil, errors.New("user not found")
			},
			mockLookupID: func(_ string) (*user.User, error) {
				return nil, errors.New("user not found")
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockUserLooker{
				lookupFunc:   tt.mockLookup,
				lookupIdFunc: tt.mockLookupID,
			}

			result, err := lookupUser(tt.username, mock)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestIsNumeric(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"123", true},
		{"0", true},
		{"-123", false},
		{"abc", false},
		{"123a", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			require.Equal(t, tt.expected, isNumeric(tt.input))
		})
	}
}
