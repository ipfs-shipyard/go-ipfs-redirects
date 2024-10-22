package redirects

import (
	"bufio"
	"bytes"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuleIsProxy(t *testing.T) {
	t.Run("without host", func(t *testing.T) {
		r := Rule{
			From: "/blog",
			To:   "/blog/engineering",
		}

		require.False(t, r.IsProxy())
	})

	t.Run("with host", func(t *testing.T) {
		r := Rule{
			From: "/blog",
			To:   "https://site.example.com",
		}

		require.True(t, r.IsProxy())
	})
}

func TestRuleIsRewrite(t *testing.T) {
	t.Run("with 3xx", func(t *testing.T) {
		r := Rule{
			From:   "/blog",
			To:     "/blog/engineering",
			Status: 302,
		}

		require.False(t, r.IsRewrite())
	})

	t.Run("with 200", func(t *testing.T) {
		r := Rule{
			From:   "/blog",
			To:     "/blog/engineering",
			Status: 200,
		}

		require.True(t, r.IsRewrite())
	})

	t.Run("with 0", func(t *testing.T) {
		r := Rule{
			From: "/blog",
			To:   "/blog/engineering",
		}

		require.False(t, r.IsRewrite())
	})
}

func TestParse(t *testing.T) {
	t.Run("with illegal force", func(t *testing.T) {
		_, err := Parse(strings.NewReader(`
		/home / 301!
		`))

		require.Error(t, err)
		require.ErrorContains(t, err, "forced redirects")
	})

	t.Run("with illegal code", func(t *testing.T) {
		_, err := Parse(strings.NewReader(`
		/home / 42
		`))

		require.Error(t, err)
		require.ErrorContains(t, err, "status code 42 is not supported")
	})

	t.Run("with too large file", func(t *testing.T) {
		// create a file larger than 64 KiB, using valid rules so the only possible error is the size
		line := "/from /to 301"
		bytesPerLine := len(line)
		totalBytes := 0

		var b bytes.Buffer
		for totalBytes <= MaxFileSizeInBytes {
			totalBytes += bytesPerLine
			b.WriteString(line + "\n")
		}
		text := b.String()

		_, err := ParseString(text)

		require.Error(t, err)
		require.ErrorContains(t, err, "redirects file size cannot exceed")
	})

	t.Run("with fromQuery arguments", func(t *testing.T) {
		rules, err := ParseString(`
		/fixed type=type /type.html
		/dynamic type=:type /type-:type.html
		/empty type= /empty-type.html
		/any type=:ignore /any-type.html
		/multi a=a b=:b c= d /multi-:b.html
		/fixed200 type=type /type.html 200
		/dynamic200 type=:type /type-:type.html 200
		/empty200 type= /empty-type.html 200
		/any200 type=:ignore /any-type.html 200
		/multi200 a=a b=:b c= d /multi-:b.html 200
		`)

		assert.NoError(t, err)
		assert.Len(t, rules, 10)
		assert.Equal(t, "type", rules[0].FromQuery["type"])
		assert.Equal(t, ":type", rules[1].FromQuery["type"])
		assert.Equal(t, "", rules[2].FromQuery["type"])
		assert.Equal(t, ":ignore", rules[3].FromQuery["type"])
	})
}

func FuzzParse(f *testing.F) {
	testcases := []string{"/a /b 999\n",
		"/redirect-one /one.html\n/301-redirect-one /one.html 301\n/302-redirect-two /two.html 302\n/200-index /index.html 200\n/posts/:year/:month/:day/:title /articles/:year/:month/:day/:title 301\n/splat/* /redirected-splat/:splat 301\n/not-found/* /404.html 404\n/* /index.html 200\n",
		"/a /b 301!\n",
		"/ą /ę 301\n",
		"/%C4%85 /ę 301\n",
		"#/a \n\n/b",
		"/a200 /b200 200\n/a301 /b301 301\n/a302 /b302 302\n/a303 /b303 303\n/a307 /b307 307\n/a308 /b308 308\n/a404 /b404 404\n/a410 /b410 410\n/a451 /b451 451\n",
		"hello\n", "/redirect-one /one.html\r\n/200-index /index.html 200\r\n", "a b 2\nc   d 42", "/a/*/b blah", "/from https://example.com 200\n/a/:blah/yeah /b/:blah/yeah",
		"/fixed-val val=val /to\n", "/dynamic-val val=:val /to/:val\n", "/empty-val val= /to\n", "/any-val val /to\n",
		"/fixed-val val=val /to 200\n/dynamic-val val=:val /to/:val 301\n/empty-val val= /to 404\n/any-val val /to 302\n",
		"/multi-query val1=val1 val2=:val2 val3= val4 /to/:val2\n/multi-query2 val1=val1 val2=:val2 val3= val4 /to/:val2 302\n",
		"/bad-syntax1 val=a&val=b /to\n", "/bad-syntax2 val=a&val2=b /to 302\n", "/a ^&notparams /b\n", "/bad-status type=:type /to 3oo\n", "/bad-chars :type=whatever /to\n", "/bad-chars type=what:ever /to\n",
	}
	for _, tc := range testcases {
		f.Add([]byte(tc))
	}
	f.Fuzz(func(t *testing.T, orig []byte) {
		rules, err := Parse(bytes.NewReader(orig))
		if err != nil {
			if rules != nil {
				t.Errorf("should not return rules on error")
			}
			t.Skip()
		}

		for _, r := range rules {
			if !isValidStatusCode(r.Status) {
				t.Errorf("should error for invalid status code.  orig=%q", orig)
			}

			if !strings.HasPrefix(r.From, "/") {
				t.Errorf("should error for 'from' path not starting with '/'.  orig=%q", orig)
			}
			_, err := url.Parse(r.From)
			if err != nil {
				t.Errorf("should error for 'from' path not parsing as relative URL. from=%q, orig=%q", r.From, orig)
			}

			fromSplats := strings.Count(r.From, "*")
			if fromSplats > 0 {
				if fromSplats > 1 {
					t.Errorf("more than one asterisk in 'from' should error.  orig=%q", orig)
				}
				if !strings.HasSuffix(r.From, "*") {
					t.Errorf("asterisk in 'from' not at end should error.  orig=%q", orig)
				}
			}

			// if does not start with / we assume it is a valid url
			to, err := url.Parse(r.To)
			if err != nil {
				t.Errorf("should error for 'to' path not parsing as a path or URL. to=%q, orig=%q", to, orig)
			}
			if !strings.HasPrefix(r.To, "/") {
				if to.Scheme != "http" && to.Scheme != "https" && to.Scheme != "ipfs" && to.Scheme != "ipns" {
					t.Errorf("should error for 'to' URL with scheme other than safelisted ones: url=%q, scheme=%q, orig=%q", to, to.Scheme, orig)
				}
			}

			for key, val := range r.FromQuery {
				if url.QueryEscape(key) != key {
					t.Errorf("should error for 'fromQuery' keys being unacceptable URL characters.  orig=%q", orig)
				}

				// Colons should only be present in values right at the start (they're invalid characters otherwise).
				if len(val) > 0 && val[0] == ':' {
					val = val[1:]
				}

				if url.QueryEscape(val) != val {
					t.Errorf("should error for 'fromQuery' values containing unacceptable URL characters.  orig=%q", orig)
				}
			}
		}

		s := bufio.NewScanner(bytes.NewReader(orig))

		for s.Scan() {
			line := strings.TrimSpace(s.Text())
			fields := strings.Fields(line)

			// Skip comments so we don't have to special case
			if strings.HasPrefix(line, "#") {
				continue
			}

			if len(fields) < 2 && line != "" {
				t.Errorf("should error with less than 2 fields.  orig=%q", orig)
				continue
			}

			if len(fields) > 0 && !strings.HasPrefix(fields[0], "/") {
				t.Errorf("should error for from path not starting with '/'.  orig=%q", orig)
				continue
			}

			if len(fields) > 0 && strings.Contains(fields[0], "*") && !strings.HasSuffix(fields[0], "*") {
				t.Errorf("asterisk in from not at end should error.  orig=%q", orig)
				continue
			}

			if len(fields) > 2 && strings.HasSuffix(fields[2], "!") {
				t.Errorf("should error for forced redirects.  orig=%q, err=%v", orig, err)
				continue
			}

		}
	})
}

func TestMatchAndExpandPlaceholders(t *testing.T) {
	testcases := []struct {
		name       string
		rule       *Rule
		inPath     string
		inParams   string
		success    bool
		expectedTo string
	}{
		{
			name: "No expansion",
			rule: &Rule{
				From: "/from",
				To:   "/to",
			},
			inPath:     "/from",
			inParams:   "",
			success:    true,
			expectedTo: "/to",
		},
		{
			name: "No expansion, but trailing slash",
			rule: &Rule{
				From: "/from/",
				To:   "/to",
			},
			inPath:     "/from",
			inParams:   "",
			success:    true,
			expectedTo: "/to",
		},
		{
			name: "Splat matching",
			rule: &Rule{
				From: "/*",
				To:   "/to",
			},
			inPath:     "/from",
			inParams:   "",
			success:    true,
			expectedTo: "/to",
		},
		{
			name: "Splat substitution",
			rule: &Rule{
				From: "/*",
				To:   "/other/:splat",
			},
			inPath:     "/from",
			inParams:   "",
			success:    true,
			expectedTo: "/other/from",
		},
		{
			name: "Named substitution",
			rule: &Rule{
				From: "/:thing",
				To:   "/:thing.html",
			},
			inPath:     "/from",
			inParams:   "",
			success:    true,
			expectedTo: "/from.html",
		},
		{
			name: "Missing placeholder",
			rule: &Rule{
				From: "/:this",
				To:   "/:that.html",
			},
			inPath:   "/from",
			inParams: "",
			success:  false,
		},
		{
			name: "Static query parameter, match",
			rule: &Rule{
				From: "/from",
				FromQuery: map[string]string{
					"a": "b",
				},
				To: "/to",
			},
			inPath:     "/from",
			inParams:   "a=b",
			success:    true,
			expectedTo: "/to",
		},
		{
			name: "Static query parameter, muli-match first",
			rule: &Rule{
				From: "/from",
				FromQuery: map[string]string{
					"a": "b",
				},
				To: "/to",
			},
			inPath:     "/from",
			inParams:   "a=b&a=c",
			success:    true,
			expectedTo: "/to",
		},
		{
			name: "Static query parameter, muli-match second",
			rule: &Rule{
				From: "/from",
				FromQuery: map[string]string{
					"a": "b",
				},
				To: "/to",
			},
			inPath:     "/from",
			inParams:   "a=c&a=b",
			success:    true,
			expectedTo: "/to",
		},
		{
			name: "Static query parameter, no match",
			rule: &Rule{
				From: "/from",
				FromQuery: map[string]string{
					"a": "b",
				},
				To: "/to",
			},
			inPath:   "/from",
			inParams: "",
			success:  false,
		},
		{
			name: "Dynamic query parameter, match",
			rule: &Rule{
				From: "/from",
				FromQuery: map[string]string{
					"a": ":a",
				},
				To: "/to/:a.html",
			},
			inPath:     "/from",
			inParams:   "a=b",
			success:    true,
			expectedTo: "/to/b.html",
		},
		{
			name: "Dynamic query parameter, multi-match",
			rule: &Rule{
				From: "/from",
				FromQuery: map[string]string{
					"a": ":a",
				},
				To: "/:a.html",
			},
			inPath:     "/from",
			inParams:   "a=b&a=c",
			success:    true,
			expectedTo: "/b.html",
		},
		{
			name: "Dynamic query parameter, no match",
			rule: &Rule{
				From: "/from",
				FromQuery: map[string]string{
					"a": "b",
				},
				To: "/to",
			},
			inPath:   "/from",
			inParams: "",
			success:  false,
		},
		{
			name: "Repeated placeholder in path",
			rule: &Rule{
				From: "/:from/:from",
				To:   "/:from.html",
			},
			inPath:     "/a/b",
			inParams:   "",
			success:    true,
			expectedTo: "/b.html",
		},
		{
			name: "Repeated placeholder in params",
			rule: &Rule{
				From: "/from",
				FromQuery: map[string]string{
					"q": ":val",
					"r": ":val",
				},
				To: "/:val.html",
			},
			inPath:     "/from",
			inParams:   "q=qq&r=rr",
			success:    true,
			expectedTo: "/qq.html",
		},
		{
			name: "Repeated placeholder in path then params",
			rule: &Rule{
				From: "/:val",
				FromQuery: map[string]string{
					"q": ":val",
				},
				To: "/:val.html",
			},
			inPath:     "/path",
			inParams:   "q=query",
			success:    true,
			expectedTo: "/path.html",
		},
		{
			name: "Repeated placeholder splat",
			rule: &Rule{
				From: "/*",
				FromQuery: map[string]string{
					"q": ":splat",
				},
				To: "/:splat.html",
			},
			inPath:     "/path",
			inParams:   "q=query",
			success:    true,
			expectedTo: "/path.html",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			params, err := url.ParseQuery(tc.inParams)
			if err != nil {
				t.Errorf("Invalid inParams given (%s): %v", tc.inParams, err)
			}

			ok := tc.rule.MatchAndExpandPlaceholders(tc.inPath, params)
			assert.Equal(t, tc.success, ok, "Expected success to be %v, but was %v", tc.success, ok)

			if tc.success {
				assert.Equal(t, tc.expectedTo, tc.rule.To, "Expected the To property to be changed to %q, but was %q", tc.expectedTo, tc.rule.To)
			}
		})
	}
}
