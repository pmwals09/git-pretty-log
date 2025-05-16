package main

import (
	"fmt"
	"os"

	"github.com/go-git/go-git/v5"
)

func main() {
	// make sure we're in some repository
	_, err := getRepo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "are you sure you're in a repository? %s\n", err.Error())
		os.Exit(1)
	}

	// get the repo base branch reference
	// start walking back 30 commits
	// if commit contains master, produce a diff
	// else print everything else
}

func getRepo() (*git.Repository, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return git.PlainOpen(wd)
}
