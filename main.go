package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/fatih/color"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/maniartech/gotime"
)

func main() {
	// make sure we're in some repository
	args, err := parseArgs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing args: %s\n", err.Error())
		os.Exit(1)
	}
	repo := args.repo

	// Map local branch hashes to branch name
	refs, err := repo.References()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting repo references: %s\n", err.Error())
		os.Exit(1)
	}
	refHashToName := make(map[string][]string)
	refs.ForEach(func(r *plumbing.Reference) error {
		refHash := r.Hash().String()
		if _, ok := refHashToName[refHash]; ok {
			refHashToName[refHash] = append(refHashToName[refHash], r.Name().Short())
		} else {
			refHashToName[refHash] = []string{r.Name().Short()}
		}
		return nil
	})

	head, _ := repo.Head()
	headCommit, _ := repo.CommitObject(head.Hash())
	mbCommits, _ := headCommit.MergeBase(args.baseCommit)
	reachable := true
	if len(mbCommits) == 0 {
		reachable = false
	}

	table := getTableWriter()

	// start walking back 30 commits
	log, err := repo.Log(&git.LogOptions{})
	count := args.numberCommits

	log.ForEach(func(commit *object.Commit) error {
		if commit.Hash.String() == args.baseCommit.Hash.String() {
			reachable = false
		}
		if count == 0 {
			return storer.ErrStop
		}
		count--

		// if commit contains master, produce a diff
		if reachable {
			printCommitWithDiff(commit, args.baseCommit, &table, refHashToName, args)
		} else {
			printCommit(commit, &table, refHashToName)
		}
		return nil
	})

	table.Render()
}

type Args struct {
	baseName      string
	numberCommits int
	repoPath      string
}

func (a Args) Parse() (*ParsedArgs, error) {
	pa := ParsedArgs{numberCommits: a.numberCommits, repoPath: a.repoPath}

	repo, err := git.PlainOpen(a.repoPath)
	if err != nil {
		return nil, err
	}
	pa.repo = repo

	// check if the provided reference is valid
	var baseCommit *object.Commit
	if a.baseName == "" {
		r, err := getBaseBranch(repo)
		if err != nil {
			return nil, fmt.Errorf("error getting repo base branch: %w", err)
		}
		commit, err := repo.CommitObject(r.Hash())
		if err != nil {
			return nil, fmt.Errorf("error getting base branch commit: %w", err)
		}
		baseCommit = commit
	} else {
		hash, err := repo.ResolveRevision(plumbing.Revision(a.baseName))
		if err != nil {
			return nil, fmt.Errorf("the provided base %s is invalid: %w", a.baseName, err)
		}
		commit, err := repo.CommitObject(*hash)
		if err != nil {
			return nil, fmt.Errorf("error getting provided base %s commit: %w", a.baseName, err)
		}
		baseCommit = commit
	}

	pa.baseCommit = baseCommit
	return &pa, nil
}

type ParsedArgs struct {
	baseCommit    *object.Commit
	numberCommits int
	repo          *git.Repository
	repoPath      string
}

func parseArgs() (*ParsedArgs, error) {
	args := Args{}

	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	var longRepo string
	flag.StringVar(&longRepo, "repo-path", wd, "The path of the git repository")
	flag.StringVar(&args.repoPath, "r", wd, "The path of the git repository")
	var longBase string
	flag.StringVar(&longBase, "base", "", "The commit against which to compare")
	flag.StringVar(&args.baseName, "b", "", "The commit against which to compare")
	var longNumberCommits int
	flag.IntVar(&longNumberCommits, "num-commits", 30, "The number of commits to display. Note that a large number will degrade performance")
	flag.IntVar(&args.numberCommits, "n", 30, "The number of commits to display. Note that a large number will degrade performance")
	flag.Parse()

	// Prefer the long version if both are provided
	if longBase != "" {
		args.baseName = longBase
	}
	if longRepo != wd {
		args.repoPath = longRepo
	}
	if longNumberCommits != 0 {
		args.numberCommits = longNumberCommits
	}

	return args.Parse()
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

func getTableWriter() table.Writer {
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.Style().Options.DrawBorder = false
	t.Style().Options.SeparateColumns = false
	t.Style().Options.SeparateFooter = false
	t.Style().Options.SeparateHeader = false
	t.Style().Options.SeparateRows = false
	return t
}

func printCommit(commit *object.Commit, tw *table.Writer, refHashToName map[string][]string) {
	hash := prettyHash(commit)
	relTime := prettyRelativeTime(commit)
	author := prettyAuthor(commit)
	diff := ""
	message := prettyMessage(commit, refHashToName)
	(*tw).AppendRow(table.Row{hash, relTime, author, diff, message})
}

// func printCommitWithDiff(commit *object.Commit, ancestor *object.Tree, tw *table.Writer, refHashToName map[string][]string) {
func printCommitWithDiff(commit, ancestor *object.Commit, tw *table.Writer, refHashToName map[string][]string, pa *ParsedArgs) {
	hash := prettyHash(commit)
	relTime := prettyRelativeTime(commit)
	author := prettyAuthor(commit)
	diff := prettyDiff(commit, ancestor, pa)
	message := prettyMessage(commit, refHashToName)
	(*tw).AppendRow(table.Row{hash, relTime, author, diff, message})
}

func prettyHash(commit *object.Commit) string {
	return color.YellowString(commit.Hash.String()[:7])
}
func prettyRelativeTime(commit *object.Commit) string {
	return color.GreenString(gotime.TimeAgo(commit.Author.When))
}
func prettyAuthor(commit *object.Commit) string {
	return color.New(color.FgBlue).Add(color.Bold).Sprint(commit.Author.Name)
}
func prettyMessage(commit *object.Commit, refHashToName map[string][]string) string {
	messageLines := strings.SplitN(commit.Message, "\n", 2)
	message := strings.TrimSpace(messageLines[0])
	var refName string
	if refNames, ok := refHashToName[commit.Hash.String()]; ok {
		formattedRefNames := make([]string, 0, len(refNames))
		for _, rn := range refNames {
			formattedRefNames = append(formattedRefNames, color.RedString("(%s)", rn))
		}
		refName = strings.Join(formattedRefNames, "")
		return fmt.Sprintf("%s %s", refName, message)
	} else {
		return message
	}
}

var shortstatRE = regexp.MustCompile(`(?:(\d+)\s+files?\s+changed)?(?:,\s+(\d+)\s+insertions?\(\+\))?(?:,\s+(\d+)\s+deletions?\(-\))?`)

func prettyDiff(commit, ancestor *object.Commit, pa *ParsedArgs) string {
	cmd := exec.Command("git", "diff", "--shortstat", ancestor.Hash.String(), commit.Hash.String())
	cmd.Dir = pa.repoPath
	ba, _ := cmd.Output()
	matches := shortstatRE.FindStringSubmatch(strings.TrimSpace(string(ba)))
	if len(matches) == 0 {
		return ""
	}
	var totalFiles, totalAdded, totalDeleted string
	if matches[1] != "" {
		totalFiles = matches[1]
	}
	if matches[2] != "" {
		totalAdded = matches[2]
	}
	if matches[3] != "" {
		totalDeleted = matches[3]
	}

	parts := make([]string, 0, 3)
	if totalFiles != "" {
		parts = append(parts, color.YellowString("%s(~)", totalFiles))
	}
	if totalAdded != "" {
		parts = append(parts, color.GreenString("%s(+)", totalAdded))
	}
	if totalDeleted != "" {
		parts = append(parts, color.RedString("%s(-)", totalDeleted))
	}
	return strings.Join(parts, ",")
}
