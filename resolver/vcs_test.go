package main

import "testing"

func TestFetchRepoRoot(t *testing.T) {
	for _, tc := range []struct {
		imp string
		exp string
	}{
		{"gopkg.in/alecthomas/kingpin.v2", "https://github.com/alecthomas/kingpin"},
		{"k8s.io/api/v1", "https://github.com/kubernetes/api"},
		{"k8s.io/kubernetes/pkg/apis/core/validation", "https://github.com/kubernetes/kubernetes"},
		{"gonum.org/v1/gonum", "https://github.com/gonum/gonum"},
		{"gonum.org/v1/hdf5", "https://github.com/gonum/hdf5"},
	} {
		rr, err := repoRootForImportDynamic(tc.imp, IgnoreMod)
		if err != nil {
			t.Error(err)
		}
		if rr.Repo != tc.exp {
			t.Error("got: ", rr, "expected: ", tc.exp)
		}
	}

}
