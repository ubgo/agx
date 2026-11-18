package apperr

import "regexp"

// Library prefixes in user-facing text. Go libraries prefix their errors
// with their package name ("agentio: login expired at …"), which says
// nothing to a person reading agx's output; v0.3 showed the finding
// alone. Agent names ("codex: autonomy accept-edits: …") are kept: they
// say which agent refused.
//
// libraryPrefix matches one such prefix at the start of the text, after
// a ": " separator or after "(", so an agx message wrapping a library
// error ("login carol: agentio: …") loses only the library's name.
var libraryPrefix = regexp.MustCompile(`(^|: |\()(?:agentio|textdata|jsonc|schema|value|dotenv|gopass|pick|builtin|frontmatter): `)

// UserText is s without library prefixes, for any message agx shows.
func UserText(s string) string {
	for {
		t := libraryPrefix.ReplaceAllString(s, "$1")
		if t == s {
			return s
		}
		s = t
	}
}

// Text is err's message without library prefixes; "" for nil.
func Text(err error) string {
	if err == nil {
		return ""
	}
	return UserText(err.Error())
}
