package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

func main() {
	// make sure we're in some repository
	repo, err := getRepo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "are you sure you're in a repository? %s\n", err.Error())
		os.Exit(1)
	}

	// get the repo base branch reference
	_, err = getBaseBranch(repo)
	if err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		os.Exit(1)
	}

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

func getBaseBranch(repo *git.Repository) (*plumbing.Reference, error) {
	VALID_BASE_BRANCH_NAMES := []string{"refs/heads/main", "refs/heads/master"}

	for _, branchName := range VALID_BASE_BRANCH_NAMES {
		ref, err := repo.Reference(plumbing.ReferenceName(branchName), true)
		if err != nil {
			continue
		} else {
			return ref, nil
		}
	}

	return nil, errors.New("unable to find base branch among \"main\" or \"master\"")
}
