package qiniu

import (
	"testing"

	"myblogx/conf"
)

func cfgFor(uri string) conf.QiNiu { return conf.QiNiu{Uri: uri} }

func TestObjectURLNormalizesToHTTPS(t *testing.T) {
	const key = "myblogx/images/20261030/a.jpg"
	cases := []struct {
		name string
		uri  string
		want string
	}{
		{"empty uri yields empty url", "", ""},
		{"bare domain gets https", "image.example.com", "https://image.example.com/" + key},
		{"trailing slash is trimmed", "image.example.com/", "https://image.example.com/" + key},
		{"http is upgraded to https", "http://image.example.com", "https://image.example.com/" + key},
		{"protocol-relative gets https", "//image.example.com", "https://image.example.com/" + key},
		{"https stays https", "https://image.example.com", "https://image.example.com/" + key},
		{"whitespace is trimmed", "  https://image.example.com  ", "https://image.example.com/" + key},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := New(cfgFor(tc.uri))
			if got := s.ObjectURL(key); got != tc.want {
				t.Fatalf("ObjectURL(%q) with uri %q = %q, want %q", key, tc.uri, got, tc.want)
			}
		})
	}
}
