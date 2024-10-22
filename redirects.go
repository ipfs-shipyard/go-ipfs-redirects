// Package redirects provides Netlify style _redirects file format parsing.
package redirects

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/ucarion/urlpath"
)

// 64 KiB
const MaxFileSizeInBytes = 65536

// A Rule represents a single redirection or rewrite rule.
type Rule struct {
	// From is the path which is matched to perform the rule.
	From string

	// FromQuery is the set of required query parameters which
	// must be present to perform the rule.
	// A string without a preceding colon requires that query parameter is this exact value.
	// A string with a preceding colon will match any value, and provide it as a placeholder.
	FromQuery map[string]string

	// To is the destination which may be relative, or absolute
	// in order to proxy the request to another URL.
	To string

	// Status is one of the following:
	//
	// - 3xx a redirect
	// - 200 a rewrite
	// - defaults to 301 redirect
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
func (r *Rule) MatchAndExpandPlaceholders(urlPath string, urlParams url.Values) bool {
	// get rule.From, trim trailing slash, ...
	fromPath := urlpath.New(strings.TrimSuffix(r.From, "/"))
	match, ok := fromPath.Match(urlPath)
	if !ok {
		return false
	}

	placeholders := match.Params
	placeholders["splat"] = match.Trailing
	if !matchParams(r.FromQuery, urlParams, placeholders) {
		return false
	}

	// We have a match! Perform substitution and return the updated rule
	toPath := r.To
	toPath = replacePlaceholders(toPath, placeholders)

	// There's a placeholder unsupplied somewhere
	if strings.Contains(toPath, ":") {
		return false
	}

	r.To = toPath

	return true
}

func replacePlaceholders(to string, placeholders map[string]string) string {
	if len(placeholders) == 0 {
		return to
	}

	for key, value := range placeholders {
		to = strings.ReplaceAll(to, ":"+key, value)
	}

	return to
}

func replaceSplat(to string, splat string) string {
	return strings.ReplaceAll(to, ":splat", splat)
}

func matchParams(fromQuery map[string]string, urlParams url.Values, placeholders map[string]string) bool {
	for neededK, neededV := range fromQuery {
		haveVs, ok := urlParams[neededK]
		if !ok {
			return false
		}

		if isPlaceholder(neededV) {
			if _, ok := placeholders[neededV[1:]]; !ok {
				placeholders[neededV[1:]] = haveVs[0]
			}
			continue
		}

		if !contains(haveVs, neededV) {
			return false
		}
	}

	return true
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
	limiter := &io.LimitedReader{R: r, N: MaxFileSizeInBytes + 1}
	s := bufio.NewScanner(limiter)
	for s.Scan() {
		// detect when we've read one byte beyond MaxFileSizeInBytes
		// and return user-friendly error
		if limiter.N <= 0 {
			return nil, fmt.Errorf("redirects file size cannot exceed %d bytes", MaxFileSizeInBytes)
		}

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
			return nil, fmt.Errorf("missing 'to' path")
		}

		// implicit status
		rule := Rule{Status: 301}

		// from (must parse as an absolute path)
		from, err := parseFrom(fields[0])
		if err != nil {
			return nil, fmt.Errorf("parsing 'from': %w", err)
		}
		rule.From = from

		hasStatus := isLikelyStatusCode(fields[len(fields)-1])
		toIndex := len(fields) - 1
		if hasStatus {
			toIndex = len(fields) - 2
		}

		// to (must parse as an absolute path or an URL)
		to, err := parseTo(fields[toIndex])
		if err != nil {
			return nil, fmt.Errorf("parsing 'to': %w", err)
		}
		rule.To = to

		// status
		if hasStatus {
			code, err := parseStatus(fields[len(fields)-1])
			if err != nil {
				return nil, fmt.Errorf("parsing status %q: %w", fields[2], err)
			}

			rule.Status = code
		}

		// from query
		if toIndex > 1 {
			rule.FromQuery = make(map[string]string)

			for i := 1; i < toIndex; i++ {
				key, value, err := parseFromQuery(fields[i])
				if err != nil {
					return nil, errors.Wrapf(err, "parsing 'fromQuery'")
				}
				rule.FromQuery[key] = value
			}
		}

		rules = append(rules, rule)
	}

	err = s.Err()
	if err != nil {
		return nil, err
	}
	return rules, nil
}

// ParseString parses the given string.
func ParseString(s string) ([]Rule, error) {
	return Parse(strings.NewReader(s))
}

func parseFrom(s string) (string, error) {
	// enforce a single splat
	fromSplats := strings.Count(s, "*")
	if fromSplats > 0 {
		if !strings.HasSuffix(s, "*") {
			return "", fmt.Errorf("path must end with asterisk")
		}
		if fromSplats > 1 {
			return "", fmt.Errorf("path can have at most one asterisk")
		}
	}

	// confirm value is within URL path spec
	_, err := url.Parse(s)
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(s, "/") {
		return "", fmt.Errorf("path must begin with '/'")
	}
	return s, nil
}

func parseFromQuery(s string) (string, string, error) {
	params, err := url.ParseQuery(s)
	if err != nil {
		return "", "", err
	}
	if len(params) != 1 {
		return "", "", fmt.Errorf("separate different fromQuery arguments with a space")
	}

	var key string
	var val []string
	// We know there's only 1, but we don't know the key to access it
	for k, v := range params {
		key = k
		val = v
	}

	if url.QueryEscape(key) != key {
		return "", "", fmt.Errorf("fromQuery key must be URL encoded")
	}

	if len(val) > 1 {
		return "", "", fmt.Errorf("separate different fromQuery arguments with a space")
	}

	ignorePlaceholders := val[0]
	if isPlaceholder(val[0]) {
		ignorePlaceholders = ignorePlaceholders[1:]
	}

	if url.QueryEscape(ignorePlaceholders) != ignorePlaceholders {
		return "", "", fmt.Errorf("fromQuery val must be URL encoded")
	}
	return key, val[0], nil
}

func isPlaceholder(s string) bool {
	return strings.HasPrefix(s, ":")
}

func parseTo(s string) (string, error) {
	// confirm value is within URL path spec
	u, err := url.Parse(s)
	if err != nil {
		return "", err
	}

	// if the value is  a patch attached to full URL, only allow safelisted schemes
	if !strings.HasPrefix(s, "/") {
		if u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "ipfs" && u.Scheme != "ipns" {
			return "", fmt.Errorf("invalid URL scheme")
		}
	}

	return s, nil
}

var likeStatusCode = regexp.MustCompile(`^\d{1,3}!?$`)

// isLikelyStatusCode returns true if the given string is likely to be a status code.
func isLikelyStatusCode(s string) bool {
	return likeStatusCode.MatchString(s)
}

// parseStatus returns the status code.
func parseStatus(s string) (code int, err error) {
	if strings.HasSuffix(s, "!") {
		// See https://docs.netlify.com/routing/redirects/rewrites-proxies/#shadowing
		return 0, fmt.Errorf("forced redirects (or \"shadowing\") are not supported")
	}

	code, err = strconv.Atoi(s)
	if err != nil {
		return 0, err
	}

	if !isValidStatusCode(code) {
		return 0, fmt.Errorf("status code %d is not supported", code)
	}

	return code, nil
}

func isValidStatusCode(status int) bool {
	switch status {
	case 200, 301, 302, 303, 307, 308, 404, 410, 451:
		return true
	}
	return false
}

func contains(arr []string, s string) bool {
	for _, a := range arr {
		if a == s {
			return true
		}
	}
	return false
}
