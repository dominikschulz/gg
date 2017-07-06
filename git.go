package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fatih/color"
)

type git struct {
	sync.Mutex
}

func (g *git) isClean(file string) bool {
	g.Lock()
	defer g.Unlock()

	if !isGitRepo(file) {
		return false
	}
	gr := findGitRepo(file)

	out := &bytes.Buffer{}
	args := []string{"status", "--porcelain"}
	args = append(args, file)
	cmd := exec.Command("git", args...)
	cmd.Dir = gr
	cmd.Stdout = out

	if err := cmd.Run(); err != nil {
		fmt.Println(color.RedString("failed to check git status: %v", err))
		return false
	}

	return out.Len() == 0
}

// gitAdd adds the listed files to the git index
func (g *git) add(file string) error {
	g.Lock()
	defer g.Unlock()

	if !isGitRepo(file) {
		return fmt.Errorf("not a git repo")
	}
	file, err := filepath.Abs(file)
	if err != nil {
		return err
	}
	gr := findGitRepo(file)
	file = strings.TrimPrefix(file, gr+"/")

	args := []string{"add", "--all"}
	args = append(args, file)
	cmd := exec.Command("git", args...)
	cmd.Dir = gr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add files to git: %v", err)
	}

	return nil
}

// gitCommit creates a new git commit with the given commit message
func (g *git) commit(repo, msg string) error {
	g.Lock()
	defer g.Unlock()

	if !isGitRepo(repo) {
		return fmt.Errorf("not a git repo")
	}

	cmd := exec.Command("git", "commit", "-m", msg)
	cmd.Dir = repo
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to commit files to git: %v", err)
	}

	return nil
}

func findGitRepo(fn string) string {
	fn, err := filepath.Abs(fn)
	if err != nil {
		panic(err)
	}
	var cnt uint8
	for {
		cnt++
		if cnt > 10 {
			return ""
		}
		if fn == "" || fn == "/" {
			return ""
		}
		gfn := filepath.Join(fn, ".git")
		_, err := os.Stat(gfn)
		if err == nil {
			return fn
		}
		fn = filepath.Dir(fn)
	}
	return ""
}

func isGitRepo(fn string) bool {
	fn, err := filepath.Abs(fn)
	if err != nil {
		panic(err)
	}
	var cnt uint8
	for {
		cnt++
		if cnt > 10 {
			return false
		}
		fn = strings.TrimSpace(fn)
		if fn == "" || fn == "/" {
			return false
		}
		gfn := filepath.Join(fn, ".git")
		_, err := os.Stat(gfn)
		if err == nil {
			return true
		}
		fn = filepath.Dir(fn)
	}
	return false
}
