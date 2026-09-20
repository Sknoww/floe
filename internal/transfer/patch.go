package transfer

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Sknoww/floe/internal/git"
)

// filePatch is one file's section of a patch.
type filePatch struct {
	Path   string
	Binary bool
	// Added holds the added lines, without the leading "+", numbered as they
	// stand in the file's new version.
	Added []git.Line
}

// splitPatch reads a patch from git.Repo.Patch into its file sections. Only
// added lines are kept: they are what the content guard scans.
func splitPatch(patch []byte) ([]filePatch, error) {
	var files []filePatch
	inHunk := false
	next := 0 // the new version's number for the hunk's next line
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
			n, ok := git.HunkStart(line)
			if !ok {
				return nil, fmt.Errorf("patch: malformed hunk header %q", line)
			}
			inHunk, next = true, n
		case inHunk && strings.HasPrefix(line, "+"):
			f.Added = append(f.Added, git.Line{No: next, Text: line[1:]})
			next++
		case inHunk && (line == "" || line[0] == ' '):
			// A context line. diff.suppressBlankEmpty writes an empty one as
			// nothing at all.
			next++
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
	// LineNo numbers the line that matched: in the file's new version for a
	// line of the patch, in the merged result for a line the merge brought in
	// (in diff3 style, for a conflict), or in the commit message.
	LineNo int
	Line   string // the line that matched, without the patch's leading "+"
	// Spans are the byte ranges of Line the pattern matched, zero-width matches
	// left out. A match in the message that runs past its line stops there.
	Spans [][2]int
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
			line := message[start : start+end]
			matches = append(matches, GuardMatch{
				Pattern: re.String(),
				LineNo:  strings.Count(message[:start], "\n") + 1,
				Line:    line,
				Spans:   spans([][]int{{loc[0] - start, min(loc[1]-start, len(line))}}),
			})
		}
	}
	return matches
}

// landedMatches matches the guard against the lines a preview's result brings
// into each file (git.ApplyResult.Landed), in path order. The patch's added
// lines are among them and have already passed, so every match is a line the
// merge brought in.
func landedMatches(guard []*regexp.Regexp, landed map[string][]git.Line) []GuardMatch {
	paths := slices.Sorted(maps.Keys(landed))
	var matches []GuardMatch
	for _, re := range guard {
		for _, path := range paths {
			matches = append(matches, lineMatches(re, path, landed[path], true)...)
		}
	}
	return matches
}

func lineMatches(re *regexp.Regexp, path string, lines []git.Line, merged bool) []GuardMatch {
	var matches []GuardMatch
	for _, line := range lines {
		if locs := re.FindAllStringIndex(line.Text, -1); locs != nil {
			matches = append(matches, GuardMatch{
				Pattern: re.String(),
				Path:    path,
				LineNo:  line.No,
				Line:    line.Text,
				Spans:   spans(locs),
				Merged:  merged,
			})
		}
	}
	return matches
}

// spans keeps the matched ranges that cover at least one byte.
func spans(locs [][]int) [][2]int {
	var out [][2]int
	for _, loc := range locs {
		if loc[1] > loc[0] {
			out = append(out, [2]int{loc[0], loc[1]})
		}
	}
	return out
}
