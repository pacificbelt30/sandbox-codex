package cxd

import "testing"

func TestCaseProxyCannotReuseDifferentModelPolicy(t *testing.T) {
	for _, tc := range []struct {
		args, models []string
		want         bool
	}{
		{[]string{"proxy", "serve"}, nil, true},
		{[]string{"proxy", "serve", "--allow-model", "one"}, []string{"one"}, true},
		{[]string{"proxy", "serve", "--allow-model=one"}, []string{"one"}, true},
		{[]string{"proxy", "serve", "--allow-model", "one"}, nil, false},
		{[]string{"proxy", "serve"}, []string{"one"}, false},
		{[]string{"proxy", "serve", "--allow-model", "one"}, []string{"two"}, false},
		{[]string{"proxy", "serve", "--allow-model"}, nil, false},
	} {
		if got := sameModelPolicy(tc.args, tc.models); got != tc.want {
			t.Errorf("%v / %v: %v", tc.args, tc.models, got)
		}
	}
}
