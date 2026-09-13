package transfer

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// filePatch is one file's section of a patch.
type filePatch struct {
	Path   string
	Binary bool
	Added  []string // added lines, without the leading "+"
}

// splitPatch reads a patch from git.Repo.Patch into its file sections. Only
// added lines are kept: they are what the content guard scans.
func splitPatch(patch []byte) ([]filePatch, error) {
	var files []filePatch
	inHunk := false
	for _, line := range strings.Split(string(patch), "\n") {
		// A hunk line starts with ' ', '+', '-' or '\', so a line starting
		// "diff --git " is always a new file's header.
		if rest, ok := strings.CutPrefix(line, "diff --git "); ok {
			path, err := headerPath(rest)
			if err != nil {
				return nil, err
			}
			files = append(files, filePatch{Path: path})
			inHunk = false
			continue
		}
		if len(files) == 0 {
			if line == "" {
				continue
			}
			return nil, fmt.Errorf("patch: %q before the first file header", line)
		}
		f := &files[len(files)-1]
		switch {
		case f.Binary:
		case !inHunk && line == "GIT binary patch":
			f.Binary = true
		case strings.HasPrefix(line, "@@ "):
			inHunk = true
		case inHunk && strings.HasPrefix(line, "+"):
			f.Added = append(f.Added, line[1:])
		}
	}
	return files, nil
}

// headerPath reads the path out of a "diff --git " header. With renames off and
// the a/ b/ prefixes pinned, both halves name the same path: "a/P b/P", or
// `"a/P" "b/P"` when git quoted P for containing a quote, a backslash, a control
// character or a byte outside ASCII.
func headerPath(rest string) (string, error) {
	if strings.HasPrefix(rest, `"`) {
		end := closingQuote(rest)
		if end < 0 {
			return "", fmt.Errorf("patch: unterminated quoted path in %q", rest)
		}
		// Git's C-style quoting — \t, \", \\ and three-digit octal bytes — is
		// a subset of Go's.
		half, err := strconv.Unquote(rest[:end+1])
		if err != nil {
			return "", fmt.Errorf("patch: quoted path in %q: %w", rest, err)
		}
		path, ok := strings.CutPrefix(half, "a/")
		if !ok {
			return "", fmt.Errorf("patch: header %q lacks the a/ prefix", rest)
		}
		return path, nil
	}
	// "a/" + P + " b/" + P is 2|P| + 5 bytes, which fixes |P| even when P itself
	// contains " b/".
	if len(rest) < 7 || (len(rest)-5)%2 != 0 {
		return "", fmt.Errorf("patch: malformed header %q", rest)
	}
	path := rest[2 : 2+(len(rest)-5)/2]
	if rest != "a/"+path+" b/"+path {
		return "", fmt.Errorf("patch: malformed header %q", rest)
	}
	return path, nil
}

// closingQuote is the index of the quote that closes the quoted string s opens,
// or -1.
func closingQuote(s string) int {
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return i
		}
	}
	return -1
}

// GuardMatch is one place a content guard pattern matched.
type GuardMatch struct {
	Pattern string
	Path    string // the file whose line matched; "" for the commit message
	Line    string // the line that matched, without the patch's leading "+"
	// Merged marks a line the 3-way merge brought in beyond the patch's added
	// lines: a side of a conflict, or what a merge driver kept.
	Merged bool
}

// GuardError refuses a transfer whose patch, message or merge result matches
// the pair's content guard. Nothing visible has been written.
type GuardError struct {
	Matches []GuardMatch
}

func (e *GuardError) Error() string {
	m := e.Matches[0]
	where := "the commit message"
	if m.Path != "" {
		where = m.Path
	}
	if m.Merged {
		where += ", brought in by the 3-way merge"
	}
	return fmt.Sprintf("the content guard refused the transfer: %q matched in %s (%d match(es) in all)", m.Pattern, where, len(e.Matches))
}

// guardMatches matches the guard against every added line of the patch and
// against the commit's full message. A pattern is matched against the message
// whole, so one written to span lines can; the match is reported by the line it
// starts on.
func guardMatches(guard []*regexp.Regexp, files []filePatch, message string) []GuardMatch {
	var matches []GuardMatch
	for _, re := range guard {
		for _, f := range files {
			matches = append(matches, lineMatches(re, f.Path, f.Added, false)...)
		}
		for _, loc := range re.FindAllStringIndex(message, -1) {
			start := strings.LastIndex(message[:loc[0]], "\n") + 1
			end := strings.Index(message[start:], "\n")
			if end < 0 {
				end = len(message) - start
			}
			matches = append(matches, GuardMatch{Pattern: re.String(), Line: message[start : start+end]})
		}
	}
	return matches
}

// landedMatches matches the guard against the lines a preview's result brings
// into each file (git.ApplyResult.Landed), in path order. The patch's added
// lines are among them and have already passed, so every match is a line the
// merge brought in.
func landedMatches(guard []*regexp.Regexp, landed map[string][]string) []GuardMatch {
	paths := slices.Sorted(maps.Keys(landed))
	var matches []GuardMatch
	for _, re := range guard {
		for _, path := range paths {
			matches = append(matches, lineMatches(re, path, landed[path], true)...)
		}
	}
	return matches
}

func lineMatches(re *regexp.Regexp, path string, lines []string, merged bool) []GuardMatch {
	var matches []GuardMatch
	for _, line := range lines {
		if re.MatchString(line) {
			matches = append(matches, GuardMatch{Pattern: re.String(), Path: path, Line: line, Merged: merged})
		}
	}
	return matches
}
