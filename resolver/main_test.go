package main

import (
	"testing"
)

func TestClean(t *testing.T) {
	for _, tc := range []struct {
		inp string
		exp string
	}{
		{"a/b", "a/b"},
		{"vendor/golang_org/x/net/http/httpguts", "golang_org/x/net/http/httpguts"},
		{"github.com/FiloSottile/BERserk/_vendor/github.com/cloudflare/cfssl/cli", "github.com/cloudflare/cfssl/cli"},
		{"github.com/coreos/etcd/Godeps/_workspace/src/google.golang.org/grpc", "google.golang.org/grpc"},
	} {
		if c := clean(tc.inp); c != tc.exp {
			t.Error("got: ", c, "expected: ", tc.exp)
		}
	}
}
