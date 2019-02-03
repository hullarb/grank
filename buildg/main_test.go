package main

import (
	"path/filepath"
	"testing"
)

func TestValidSrc(t *testing.T) {
	for _, tc := range []struct {
		src   string
		valid bool
	}{
		{},
		{filepath.Join("github.com", "pkg1"), true},
		{filepath.Join("github.com", "pkg1", "sp1"), true},
		{filepath.Join("github.com", "pkg1", "vendor", "sp1"), false},
		{filepath.Join("github.com", "pkg1", "_vendor", "sp1"), false},
		{filepath.Join("github.com", "pkg1", "Godeps", "sp1"), false},
		{filepath.Join("github.com", "pkg1", "workspace", "sp1"), false},
		{filepath.Join("github.com", "pkg1", "_workspace", "sp1"), false},
	} {
		if tc.valid != validSrc(tc.src) {
			t.Error(tc.src)
		}
	}
}
