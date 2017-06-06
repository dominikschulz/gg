package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh/terminal"

	"github.com/alexflint/go-arg"
	"github.com/fatih/color"
)

var (
	colMatch = color.New(color.FgRed, color.Bold).SprintfFunc()
	colRepl  = color.New(color.FgGreen, color.Bold).SprintfFunc()
)

type fileMatch struct {
	fn  string
	buf *bytes.Buffer
}

func main() {
	// only emit color codes when stdout is a terminal
	if !terminal.IsTerminal(int(os.Stdout.Fd())) {
		color.NoColor = true
	}

	g, err := newGoGrep()
	if err != nil {
		fmt.Println(color.RedString("Error: %s\n", err))
		os.Exit(1)
	}

	if err := g.walk(""); err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}

type gg struct {
	Pattern     string         `arg:"positional"`
	Basedir     string         `arg:"positional"`
	Workers     int            `arg:"--workers"`
	Replacement string         `arg:"--replace"`
	Excludes    []string       `arg:"--exclude"`
	Overwrite   bool           `arg:"--eat-my-data"`
	NoGit       bool           `arg:"--who-needs-backups"`
	PostProc    string         `arg:"--post-proc"`
	regexp      *regexp.Regexp `arg:"-"`
	git         *git           `arg:"-"`
}

func newGoGrep() (*gg, error) {
	g := &gg{
		Workers: runtime.NumCPU() * 2,
		Basedir: ".",
		git:     &git{},
	}
	if err := arg.Parse(g); err != nil {
		return nil, err
	}
	if g.Pattern == "" {
		return nil, fmt.Errorf("Pattern must not be empty")
	}
	re, err := regexp.Compile(g.Pattern)
	if err != nil {
		return nil, err
	}
	g.regexp = re
	if !strings.HasSuffix(g.Basedir, "/") {
		g.Basedir += "/"
	}
	return g, nil
}

func (g *gg) walk(dir string) error {
	if dir == "" {
		dir = g.Basedir
	}
	// file chan
	fc := make(chan string, 1000)
	// done chan
	dc := make(chan struct{}, g.Workers+1)
	// match chan
	mc := make(chan fileMatch, 1000)
	// git chan
	gc := make(chan string, 1000)

	// start desired number of workers
	for i := 0; i < g.Workers; i++ {
		go g.worker(fc, mc, gc, dc)
	}
	// start a printer, that synchronizes the per-file output
	go g.printer(mc, dc)
	// start a commiter, that commits dirty git repo
	go g.commiter(gc, dc)

	// walk the tree
	if err := filepath.Walk(dir, g.walkerFunc(dir, fc)); err != nil {
		return err
	}
	// closing the fc will signal the workers to quit once done
	close(fc)

	// wait for the workers to stop
	for i := 0; i < g.Workers; i++ {
		<-dc
	}
	// closing the mc will signal the printer to quit once done
	close(mc)
	// wait for the printer to stop
	<-dc
	// closing gc will signal the commiter to quit once done
	close(gc)
	// wait for the commiter to stop
	<-dc

	return nil
}

func (g *gg) printer(mc chan fileMatch, dc chan struct{}) {
	for m := range mc {
		fmt.Println(color.MagentaString(strings.TrimPrefix(m.fn, g.Basedir)))
		fmt.Println(m.buf.String())
		fmt.Println("")
	}
	dc <- struct{}{}
}

func tempFileName(fn string) string {
	return filepath.Dir(fn) + "/." + filepath.Base(fn) + ".gg"
}

func fileMode(fn string) os.FileMode {
	if fi, err := os.Stat(fn); err == nil {
		return fi.Mode()
	}
	return 0755
}

func (g *gg) worker(fc chan string, mc chan fileMatch, gc chan string, dc chan struct{}) {
	for fn := range fc {
		if g.Pattern == "" {
			continue
		}
		// scan and match file, possibly replace
		fh, err := os.Open(fn)
		if err != nil {
			fmt.Println(color.RedString("Failed to open file %s: %s", fn, err))
			continue
		}
		fm := fileMatch{
			fn:  fn,
			buf: &bytes.Buffer{},
		}
		var ln uint64
		s := bufio.NewScanner(fh)
		for s.Scan() {
			ln++
			line := s.Text()
			if g.regexp.MatchString(line) {
				g.printMatch(fm, ln, line)
			}
		}

		fh.Close()
		if fm.buf.Len() > 0 {
			if g.Overwrite {
				if err := g.rewriteFile(fn, gc); err != nil {
					fmt.Println(color.RedString("Failed to rewrite file %s: %s", fn, err))
				}
			}
			mc <- fm
		}
	}
	dc <- struct{}{}
}

func (g *gg) rewriteFile(fn string, gc chan string) error {
	if !g.Overwrite {
		return nil
	}
	if !isGitRepo(fn) && !g.NoGit {
		return fmt.Errorf("Matching file is not in a Git repo. Not touching (use --who-needs-backups to force)")
	}
	if !g.git.isClean(fn) && !g.NoGit {
		return fmt.Errorf("Matching file is in dirty Git repo. Not touching (use --who-needs-backups to force)")
	}
	fh, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer fh.Close()

	tfn := tempFileName(fn)
	tfh, err := os.OpenFile(tfn, os.O_RDWR|os.O_CREATE, fileMode(fn))
	if err != nil {
		return err
	}

	s := bufio.NewScanner(fh)
	for s.Scan() {
		line := s.Text()
		if g.regexp.MatchString(line) {
			line = g.replaceMatch(line)
		}
		fmt.Fprint(tfh, line)
		fmt.Fprint(tfh, "\n")
	}
	tfh.Close()

	if g.PostProc != "" {
		if err := g.runPostProc(tfn); err != nil {
			fmt.Println(color.YellowString("PostProc %s failed on %s: %s"), g.PostProc, fn, err)
		}
	}

	if err := os.Remove(fn); err != nil {
		return err
	}
	if err := os.Rename(tfn, fn); err != nil {
		return err
	}
	if err := g.git.add(fn); err != nil {
		return err
	}
	// mark git repo as dirty
	gc <- fn
	return nil
}

func (g *gg) runPostProc(fn string) error {
	if g.PostProc == "" {
		return nil
	}
	p := strings.Split(g.PostProc, " ")
	p = append(p, fn)
	cmd := exec.Command(p[0], p[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println(color.RedString(string(out)))
		return err
	}
	return nil
}

func (g *gg) replaceMatch(line string) string {
	return g.regexp.ReplaceAllString(line, g.Replacement)
}

func (g *gg) printMatch(fm fileMatch, ln uint64, line string) {
	match := g.regexp.FindString(line)
	coloredLine := g.regexp.ReplaceAllString(line, colMatch(match))
	fmt.Fprint(fm.buf, color.GreenString(strconv.FormatUint(ln, 10))+": ")
	if g.Replacement == "" {
		fmt.Fprint(fm.buf, coloredLine+"\n")
		return
	}
	fmt.Fprint(fm.buf, "\n")
	fmt.Fprint(fm.buf, colMatch("-")+coloredLine+"\n")
	replacedLine := g.regexp.ReplaceAllString(line, colRepl(g.Replacement))
	fmt.Fprint(fm.buf, colRepl("+")+replacedLine+"\n")
	return
}

func (g *gg) walkerFunc(dir string, fc chan string) func(string, os.FileInfo, error) error {
	return func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") && path != dir {
			return filepath.SkipDir
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}
		if path == dir {
			return nil
		}
		for _, e := range g.Excludes {
			if m, _ := filepath.Match(e, path); m {
				return nil
			}
			if strings.Contains(path, e) {
				return nil
			}
		}
		fc <- path
		return nil
	}
}

func (g *gg) commiter(gc chan string, dc chan struct{}) {
	repos := make(map[string]struct{}, 10)
	for fn := range gc {
		repos[findGitRepo(fn)] = struct{}{}
	}
	for repo := range repos {
		fmt.Println(color.MagentaString("%s", repo))
		if err := g.git.commit(repo, fmt.Sprintf("gg - Replaced '%s' with '%s'", g.Pattern, g.Replacement)); err != nil {
			fmt.Println(color.RedString("Failed to commit change to repo %s: %s", repo, err))
		}
	}
	dc <- struct{}{}
}
