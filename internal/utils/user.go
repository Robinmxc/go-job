package utils

import (
	"fmt"
	"os/user"
	"strconv"
)

type userLooker interface {
	Lookup(username string) (*user.User, error)
	LookupId(uid string) (*user.User, error)
}

type defaultUserLooker struct{}

func (d *defaultUserLooker) Lookup(username string) (*user.User, error) {
	return user.Lookup(username)
}

func (d *defaultUserLooker) LookupId(uid string) (*user.User, error) {
	return user.LookupId(uid)
}

// userInfo holds user information (simplified version of os/user.User)
type userInfo struct {
	Uid  string
	Gid  string
	Home string
	Name string
}

// lookupUser retrieves user information from the system by username
func lookupUser(username string, looker userLooker) (*userInfo, error) {
	// Handle empty username case
	if username == "" {
		return nil, fmt.Errorf("username cannot be empty")
	}

	// Set defaultUserLooker
	if looker == nil {
		looker = &defaultUserLooker{}
	}

	// Try standard lookup first
	u, err := looker.Lookup(username)
	if err == nil {
		return &userInfo{
			Uid:  u.Uid,
			Gid:  u.Gid,
			Home: u.HomeDir,
			Name: u.Username,
		}, nil
	}

	// Fallback: Try by ID if username looks like a numeric ID
	if isNumeric(username) {
		u, err = looker.LookupId(username)
		if err == nil {
			return &userInfo{
				Uid:  u.Uid,
				Gid:  u.Gid,
				Home: u.HomeDir,
				Name: u.Username,
			}, nil
		}
	}

	return nil, fmt.Errorf("user lookup failed: %w", err)
}

// isNumeric checks if a string represents a numeric value
func isNumeric(s string) bool {
	out, err := strconv.Atoi(s)
	if err != nil {
		return false
	} else if out < 0 {
		return false
	}
	return true
}
