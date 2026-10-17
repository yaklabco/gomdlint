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
		{"www address", "www.example.com/path.", 0, ' ', "www.example.com/path", LiteralWWW},
		{"www after emphasis marker", "WWW.example.com*", 0, '*', "WWW.example.com", LiteralWWW},
		{"www email is an email", "www.foo@bar.com", 0, ' ', "www.foo@bar.com", LiteralEmail},
		// micromark tests whitespace with JavaScript's \s, which holds U+FEFF
		// but not U+0085, and after the protocol rejects only ASCII control
		// characters.
		{"next line in the path", "https://example.com/a\u0085b c", 0, ' ', "https://example.com/a\u0085b", LiteralURL},
		{"byte order mark ends the path", "https://example.com/a\ufeffb", 0, ' ', "https://example.com/a", LiteralURL},
		{"no-break space ends the path", "https://example.com/a\u00a0b", 0, ' ', "https://example.com/a", LiteralURL},
		{"next line in the domain", "https://exa\u0085mple.com c", 0, ' ', "https://exa\u0085mple.com", LiteralURL},
		{"next line after the protocol", "https://\u0085example.com c", 0, ' ', "https://\u0085example.com", LiteralURL},
		{"period before a byte order mark", "https://example.com/a.\ufeffb", 0, ' ', "https://example.com/a", LiteralURL},
		{"bracket before a byte order mark", "https://example.com/a]\ufeffb", 0, ' ', "https://example.com/a", LiteralURL},
		{"next line in a www domain", "www.exa\u0085mple.com c", 0, ' ', "www.exa\u0085mple.com", LiteralWWW},
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
		{"letter before www", "www.example.com", 'x'},
		{"www without a domain", "www.", ' '},
		{"www without a dot", "wwwx.example.com", ' '},
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
