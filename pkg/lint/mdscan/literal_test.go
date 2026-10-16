package mdscan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchAutolinkLiteral(t *testing.T) {
	tests := []struct {
		name string
		text string
		pos  int
		prev byte
		want string
		kind LiteralKind
	}{
		{"trailing period", "https://example.com/path.", 0, ' ', "https://example.com/path", LiteralURL},
		{"balanced parentheses", "https://en.wikipedia.org/wiki/Foo_(bar)", 0, ' ', "https://en.wikipedia.org/wiki/Foo_(bar)", LiteralURL},
		{"unbalanced closing parenthesis", "https://example.com/a)", 0, '(', "https://example.com/a", LiteralURL},
		{"parentheses inside", "https://www.google.com/search?q=(business))+ok", 0, ' ',
			"https://www.google.com/search?q=(business))+ok", LiteralURL},
		{"query with ampersand", "https://google.com/search?q=commonmark&hl=en", 0, ' ',
			"https://google.com/search?q=commonmark&hl=en", LiteralURL},
		{"entity at end", "https://google.com/search?q=commonmark&hl;", 0, ' ', "https://google.com/search?q=commonmark", LiteralURL},
		{"trailing underscore", "https://example.com/a_b_", 0, ' ', "https://example.com/a_b", LiteralURL},
		{"less than ends the url", "https://example.com/a<b", 0, ' ', "https://example.com/a", LiteralURL},
		{"closing bracket before space", "https://example.com/a] rest", 0, '[', "https://example.com/a", LiteralURL},
		{"port and query", "http://localhost:8080/x?y=1", 0, ' ', "http://localhost:8080/x?y=1", LiteralURL},
		{"uppercase protocol", "HTTPS://EXAMPLE.COM", 0, ' ', "HTTPS://EXAMPLE.COM", LiteralURL},
		{"underscore in earlier segment", "https://a_b.example.com", 0, ' ', "https://a_b.example.com", LiteralURL},
		{"email", "foo@bar.baa.com", 0, ' ', "foo@bar.baa.com", LiteralEmail},
		{"email with plus", "hello+xyz@mail.example", 0, ' ', "hello+xyz@mail.example", LiteralEmail},
		{"email trailing dot", "a.b-c_d@a.b.", 0, ' ', "a.b-c_d@a.b", LiteralEmail},
		{"email after open parenthesis", "x@y.zz)", 0, '(', "x@y.zz", LiteralEmail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			end, kind := MatchAutolinkLiteral([]byte(tt.text), tt.pos, tt.prev)
			assert.Equal(t, tt.kind, kind)
			assert.Equal(t, tt.want, tt.text[tt.pos:end])
		})
	}
}

func TestMatchAutolinkLiteralRejects(t *testing.T) {
	tests := []struct {
		name string
		text string
		prev byte
	}{
		{"letter before protocol", "https://example.com", 'x'},
		{"protocol only", "https://", ' '},
		{"punctuation after protocol", "https://.example.com", ' '},
		{"underscore in last segments", "https://example.a_b.com", ' '},
		{"other protocol", "ftp://example.com", ' '},
		{"www is not matched", "www.example.com", ' '},
		{"email without dot", "hello@mail+xyz.example", ' '},
		{"email ending in hyphen", "a.b-c_d@a.b-", ' '},
		{"email ending in underscore", "a.b-c_d@a.b_", ' '},
		{"email after slash", "a@b.cc", '/'},
		{"email inside atext", "a@b.cc", 'x'},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, kind := MatchAutolinkLiteral([]byte(tt.text), 0, tt.prev)
			assert.Equal(t, LiteralNone, kind)
		})
	}
}
