package watchdogs

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/haya14busa/watchdogs/diff"
)

type Watchdogs struct {
	p     Parser
	c     CommentService
	strip int
}

// CheckResult represents a checked result of static analysis tools.
// :h error-file-format
type CheckResult struct {
	Path    string // file path
	Lnum    int    // line number
	Col     int    // column number (1 <tab> == 1 character column)
	Vcol    int    // virtual column number (1 <tab> == 8 screen columns)
	Message string // error message
}

type Parser interface {
	Parse(r io.Reader) ([]*CheckResult, error)
}

type Comment struct {
	Body     string
	Path     string
	Lnum     int
	LnumDiff int
}

type CommentService interface {
	Post(*Comment) error
	Diff() ([]byte, error)
}

func (w *Watchdogs) Run(r io.Reader) error {
	results, err := w.p.Parse(r)
	if err != nil {
		return fmt.Errorf("parse error: %v", err)
	}

	d, err := w.c.Diff()
	if err != nil {
		return fmt.Errorf("fail to get diff: %v", err)
	}

	filediffs, err := diff.ParseMultiFile(bytes.NewReader(d))
	if err != nil {
		return fmt.Errorf("fail to parse diff: %v", err)
	}
	addedlines := AddedLines(filediffs, w.strip)

	for _, result := range results {
		addedline := addedlines.Get(result.Path, result.Lnum)
		if addedline != nil {
			comment := &Comment{
				Body:     result.Message, // TODO: format message
				Path:     addedline.Path,
				Lnum:     addedline.Lnum,
				LnumDiff: addedline.LnumDiff,
			}
			if err := w.c.Post(comment); err != nil {
				return err
			}
		}
	}

	return nil
}

// AddedLine represents added line in diff.
type AddedLine struct {
	Path     string // path to new file
	Lnum     int    // the line number in the new file
	LnumDiff int    // the line number of the diff (Same as Lnumdiff of diff.Line)
	Content  string // line content
}

// PosToAddedLine is a hash table of path to line number to AddedLine.
type PosToAddedLine map[string]map[int]*AddedLine

func (p PosToAddedLine) Get(path string, lnum int) *AddedLine {
	ltodiff, ok := p[path]
	if !ok {
		return nil
	}
	diffline, ok := ltodiff[lnum]
	if !ok {
		return nil
	}
	return diffline
}

// AddedLines traverse []*diff.FileDiff and returns PosToAddedLine.
func AddedLines(filediffs []*diff.FileDiff, strip int) PosToAddedLine {
	r := make(PosToAddedLine)
	for _, filediff := range filediffs {
		path := filediff.PathNew
		ltodiff := make(map[int]*AddedLine)
		ps := strings.Split(filepath.ToSlash(filediff.PathNew), "/")

		if len(ps) > strip {
			path = filepath.Join(ps[strip:]...)
		}

		for _, hunk := range filediff.Hunks {
			for _, line := range hunk.Lines {
				if line.Type == diff.LineAdded {
					ltodiff[line.LnumNew] = &AddedLine{
						Path:     path,
						Lnum:     line.LnumNew,
						LnumDiff: line.LnumDiff,
						Content:  line.Content,
					}
				}
			}
		}
		r[path] = ltodiff
	}
	return r
}