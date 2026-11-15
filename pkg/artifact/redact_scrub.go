package artifact

import (
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	// The hex base is the base of the digits of a percent escape.
	hexBase = 16
	// The byte bits are the size of the byte a percent escape encodes.
	byteBits = 8
	// The escape length is the length of "%XX".
	escapeLength = 3
)

// wordPattern matches a white-space delimited word of free text.
var wordPattern = regexp.MustCompile(`\S+`)

// accountPattern matches a plain account name, the only user name that may stand alone in text.
var accountPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{0,64}$`)

// credentialSet is everything one address can leak into free text.
type credentialSet struct {
	// cuts are the userinfos, each with its trailing "@", that are removed from text.
	cuts []string
	// passwords are the passwords, in every form an error may quote them, and the fragments a
	// delimiter inside a password cuts loose. They are replaced by [RedactedValue].
	passwords []string
}

// empty reports whether the address carried no credential.
func (c credentialSet) empty() bool {
	return len(c.cuts) == 0
}

// credential collects the credential of the address. Every "@" that may end a userinfo is
// considered, so a password holding "@", "/", "?", "#" or "\" is found whole.
func (a *address) credential() credentialSet {
	var set credentialSet
	for _, boundary := range a.boundaries() {
		userinfo := a.rest[:boundary]
		for _, form := range textForms(userinfo) {
			set.cuts = append(set.cuts, form+"@")
		}
		// The userinfo is split at its literal colon first, while it is still encoded, and only the
		// password is decoded afterwards: an escaped colon belongs to the user name.
		user, password, found := strings.Cut(userinfo, ":")
		if !found || password == "" {
			// A plain account name such as "git" is not a secret when it stands alone. Anything else
			// in the user position is a token, and a delimiter inside it may cut it into fragments.
			if !accountPattern.MatchString(user) {
				set.passwords = append(set.passwords, fragmentsOf(user)...)
			}
			continue
		}
		// A parser may quote the user name followed by the start of the password, with the "@" cut
		// off by a delimiter inside the password, so "user:" goes with its password.
		for _, form := range textForms(user) {
			set.cuts = append(set.cuts, form+":")
		}
		// Once a password is known the user name is withheld too, wherever it is quoted: it names
		// the account the password opens.
		set.passwords = append(set.passwords, fragmentsOf(user)...)
		set.passwords = append(set.passwords, fragmentsOf(password)...)
	}
	set.cuts = unique(set.cuts)
	set.passwords = unique(set.passwords)
	return set
}

// fragmentsOf returns s in every form an error may quote it, and the fragments of each form that a
// delimiter inside s cuts loose.
func fragmentsOf(s string) []string {
	var out []string
	for _, form := range textForms(s) {
		out = append(out, form)
		for _, fragment := range strings.FieldsFunc(form, isDelimiter) {
			if len(fragment) >= minFragmentLength {
				out = append(out, fragment)
			}
		}
	}
	return out
}

// isDelimiter reports whether r separates the fragments of a password.
func isDelimiter(r rune) bool {
	return unicode.IsSpace(r) || strings.ContainsRune(`:@/?#\[]"'`, r)
}

// textForms returns s as an error message may quote it: as written, decoded, quoted, and escaped.
func textForms(s string) []string {
	bases := []string{s, lenientUnescape(s)}
	if decoded, err := url.PathUnescape(s); err == nil {
		bases = append(bases, decoded)
	}
	if decoded, err := url.QueryUnescape(s); err == nil {
		bases = append(bases, decoded)
	}
	forms := make([]string, 0, len(bases)*4)
	for _, base := range bases {
		quoted := strconv.Quote(base)
		forms = append(forms, base, quoted[1:len(quoted)-1], url.PathEscape(base), url.QueryEscape(base))
	}
	return unique(forms)
}

// lenientUnescape decodes every valid "%XX" escape of s and leaves anything else as it is, so a
// stray "%" never keeps a decoded form from being tried. A parser may decode what a strict decoder
// rejects, and an error message quotes whatever the parser saw.
func lenientUnescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+escapeLength-1 < len(s) && isHexDigit(s[i+1]) && isHexDigit(s[i+2]) {
			high, _ := strconv.ParseUint(s[i+1:i+escapeLength], hexBase, byteBits)
			b.WriteByte(byte(high))
			i += escapeLength - 1
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// unique drops empty and repeated strings, longest first, so that a longer form is always tried
// before a shorter form it contains.
func unique(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

// scrub removes the credential from text in a single pass, longest forms first.
func (c credentialSet) scrub(text string) string {
	type pair struct{ old, replacement string }
	pairs := make([]pair, 0, len(c.cuts)+len(c.passwords))
	for _, cut := range c.cuts {
		pairs = append(pairs, pair{cut, ""})
	}
	for _, password := range c.passwords {
		pairs = append(pairs, pair{password, RedactedValue})
	}
	sort.SliceStable(pairs, func(i, j int) bool { return len(pairs[i].old) > len(pairs[j].old) })
	args := make([]string, 0, len(pairs)*2)
	for _, p := range pairs {
		args = append(args, p.old, p.replacement)
	}
	return strings.NewReplacer(args...).Replace(text)
}

// survives reports whether any part of the credential is still in text, which means it was not
// removed and the whole text must be withheld. The marker that replaced a credential is ignored.
func (c credentialSet) survives(text string) bool {
	if c.empty() {
		return false
	}
	text = strings.ReplaceAll(text, RedactedValue, "")
	for _, password := range c.passwords {
		if strings.Contains(text, password) {
			return true
		}
	}
	for _, cut := range c.cuts {
		if strings.Contains(text, cut) {
			return true
		}
	}
	return false
}

// redactWords replaces every URL-shaped word of text that holds an "@" which cannot be proven to
// introduce a digest. When bearing is true, because an address that carried a credential was
// scrubbed from the text, any word with such an "@" is replaced, URL-shaped or not.
func redactWords(text string, bearing bool) string {
	if !strings.Contains(text, "@") {
		return text
	}
	return wordPattern.ReplaceAllStringFunc(text, func(word string) string {
		if !strings.Contains(word, "@") {
			return word
		}
		body := word
		if i := strings.Index(word, schemeSeparator); i >= 0 {
			for i > 0 && isSchemeByte(word[i-1]) {
				i--
			}
			body = word[i:]
		} else if !bearing {
			return word
		}
		if a := parseAddress(body); len(a.boundaries()) > 0 {
			return RedactedValue
		}
		return word
	})
}

// isSchemeByte reports whether c may appear in a URL scheme.
func isSchemeByte(c byte) bool {
	return isAlphanumeric(c) || c == '+' || c == '.' || c == '-'
}
