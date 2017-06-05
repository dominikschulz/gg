package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/fatih/color"
)

func main() {
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
	regexp      *regexp.Regexp `arg:"-"`
}

func newGoGrep() (*gg, error) {
	g := &gg{
		Workers: 4,
		Basedir: ".",
	}
	if err := arg.Parse(g); err != nil {
		return nil, err
	}
	re, err := regexp.Compile(g.Pattern)
	if err != nil {
		return nil, err
	}
	g.regexp = re
	return g, nil
}

func (g *gg) walk(dir string) error {
	if dir == "" {
		dir = g.Basedir
	}
	fc := make(chan string, 1000)
	dc := make(chan struct{}, g.Workers)

	for i := 0; i < g.Workers; i++ {
		go g.worker(fc, dc)
	}

	if err := filepath.Walk(dir, g.walkerFunc(dir, fc)); err != nil {
		return err
	}
	close(fc)

	for i := 0; i < g.Workers; i++ {
		<-dc
	}

	return nil
}

func (g *gg) worker(fc chan string, dc chan struct{}) {
	for fn := range fc {
		fmt.Println("Match:", fn)
		// TODO scan and match file, possible replace
	}
	dc <- struct{}{}
}

func (g *gg) walkerFunc(dir string, fc chan string) func(string, os.FileInfo, error) error {
	return func(path string, info os.FileInfo, err error) error {
		println(path)
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
		fc <- path
		return nil
	}
}
