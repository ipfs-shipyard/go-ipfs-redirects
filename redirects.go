// Package redirects provides Netlify style _redirects file format parsing.
package redirects

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/ucarion/urlpath"
)

// A Rule represents a single redirection or rewrite rule.
type Rule struct {
	// From is the path which is matched to perform the rule.
	From string

	// To is the destination which may be relative, or absolute
	// in order to proxy the request to another URL.
	To string

	// Status is one of the following:
	//
	// - 3xx a redirect
	// - 200 a rewrite
	// - defaults to 301 redirect
	//
	// When proxying this field is ignored.
	//
	Status int
}

// IsRewrite returns true if the rule represents a rewrite (status 200).
func (r *Rule) IsRewrite() bool {
	return r.Status == 200
}

// IsProxy returns true if it's a proxy rule (aka contains a hostname).
func (r *Rule) IsProxy() bool {
	u, err := url.Parse(r.To)
	if err != nil {
		return false
	}

	return u.Host != ""
}

// MatchAndExpandPlaceholders expands placeholders in `r.To` and returns true if the provided path matches.
// Otherwise it returns false.
func (r *Rule) MatchAndExpandPlaceholders(urlPath string) bool {
	// get rule.From, trim trailing slash, ...
	fromPath := urlpath.New(strings.TrimSuffix(r.From, "/"))
	match, ok := fromPath.Match(urlPath)

	if !ok {
		return false
	}

	// We have a match!  Perform substitution and return the updated rule
	toPath := r.To
	toPath = replacePlaceholders(toPath, match)
	toPath = replaceSplat(toPath, match)

	r.To = toPath

	return true
}

func replacePlaceholders(to string, match urlpath.Match) string {
	if len(match.Params) > 0 {
		for key, value := range match.Params {
			to = strings.ReplaceAll(to, ":"+key, value)
		}
	}

	return to
}

func replaceSplat(to string, match urlpath.Match) string {
	return strings.ReplaceAll(to, ":splat", match.Trailing)
}

// Must parse utility.
func Must(v []Rule, err error) []Rule {
	if err != nil {
		panic(err)
	}

	return v
}

// Parse the given reader.
func Parse(r io.Reader) (rules []Rule, err error) {
	s := bufio.NewScanner(r)

	for s.Scan() {
		line := strings.TrimSpace(s.Text())

		// empty
		if line == "" {
			continue
		}

		// comment
		if strings.HasPrefix(line, "#") {
			continue
		}

		// fields
		fields := strings.Fields(line)

		// missing dst
		if len(fields) <= 1 {
			return nil, fmt.Errorf("missing destination path: %q", line)
		}

		if len(fields) > 3 {
			return nil, fmt.Errorf("must match format `from to [status][!]`")
		}

		// src and dst
		rule := Rule{
			From:   fields[0],
			To:     fields[1],
			Status: 301,
		}

		// status
		if len(fields) > 2 {
			code, err := parseStatus(fields[2])
			if err != nil {
				return nil, errors.Wrapf(err, "parsing status %q", fields[2])
			}

			rule.Status = code
		}

		rules = append(rules, rule)
	}

	err = s.Err()
	return
}

// ParseString parses the given string.
func ParseString(s string) ([]Rule, error) {
	return Parse(strings.NewReader(s))
}

// parseStatus returns the status code.
func parseStatus(s string) (code int, err error) {
	if strings.HasSuffix(s, "!") {
		// See https://docs.netlify.com/routing/redirects/rewrites-proxies/#shadowing
		return -1, fmt.Errorf("forced redirects (or \"shadowing\") are not supported by IPFS gateways")
	}

	code, err = strconv.Atoi(s)
	return code, err
}
