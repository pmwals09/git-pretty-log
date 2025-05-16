package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/fatih/color"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/maniartech/gotime"
)

func main() {
	// make sure we're in some repository
	repo, err := getRepo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "are you sure you're in a repository? %s\n", err.Error())
		os.Exit(1)
	}

	args, err := parseArgs(repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing args: %s\n", err.Error())
		os.Exit(1)
	}
	fmt.Printf("%+v\n", args)

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

	table := getTableWriter()

	// start walking back 30 commits
	log, err := repo.Log(&git.LogOptions{})
	for i := 0; i < args.numberCommits; i++ {
		commit, err := log.Next()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				fmt.Fprintf(os.Stderr, "error walking log: %s", err.Error())
				os.Exit(1)
			} else {
				break
			}
		}

		// if commit contains master, produce a diff
		if contains, err := commitContainsCommit(commit, args.baseCommit); err != nil {
			fmt.Fprintf(os.Stderr, "error getting ancestry: %s\n", err.Error())
			os.Exit(1)
		} else if contains {
			printCommitWithDiff(commit, args.baseCommit, &table, refHashToName)
		} else {
			printCommit(commit, &table, refHashToName)
		}
	}

	table.Render()
}

func getRepo() (*git.Repository, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return git.PlainOpen(wd)
}

type Args struct {
	baseName      string
	numberCommits int
}

func (a Args) Parse(repo *git.Repository) (*ParsedArgs, error) {
	pa := ParsedArgs{numberCommits: a.numberCommits}

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
}

func parseArgs(repo *git.Repository) (*ParsedArgs, error) {
	args := Args{}

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
	if longNumberCommits != 0 {
		args.numberCommits = longNumberCommits
	}

	return args.Parse(repo)
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

func commitContainsCommit(descendant, ancestor *object.Commit) (bool, error) {
	mb, err := descendant.MergeBase(ancestor)
	if err != nil {
		return false, err
	}
	if len(mb) == 0 {
		return false, nil
	}

	contains := slices.ContainsFunc(mb, func(m *object.Commit) bool {
		return m.Hash == ancestor.Hash
	})
	return contains, nil
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

func printCommitWithDiff(commit, ancestor *object.Commit, tw *table.Writer, refHashToName map[string][]string) {
	hash := prettyHash(commit)
	relTime := prettyRelativeTime(commit)
	author := prettyAuthor(commit)
	diff := prettyDiff(commit, ancestor)
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
	var refName string
	if refNames, ok := refHashToName[commit.Hash.String()]; ok {
		formattedRefNames := make([]string, 0, len(refNames))
		for _, rn := range refNames {
			formattedRefNames = append(formattedRefNames, color.RedString("(%s)", rn))
		}
		refName = strings.Join(formattedRefNames, "")
		return fmt.Sprintf("%s %s", refName, strings.TrimSpace(commit.Message))
	} else {
		return strings.TrimSpace(commit.Message)
	}
}

type DiffItem struct {
	value int
	token rune
	color *color.Color
}

func prettyDiff(commit, ancestor *object.Commit) string {
	commitTree, _ := commit.Tree()
	ancestorTree, _ := ancestor.Tree()
	changes, _ := object.DiffTreeWithOptions(context.Background(), ancestorTree, commitTree, &object.DiffTreeOptions{DetectRenames: true})
	p, _ := changes.Patch()
	stats := p.Stats()
	var totalFiles int
	var totalAdded int
	var totalDeleted int
	for _, s := range stats {
		totalFiles++
		totalAdded += s.Addition
		totalDeleted += s.Deletion
	}
	items := []DiffItem{
		{value: totalFiles, token: '~', color: color.New(color.FgYellow)},
		{value: totalAdded, token: '+', color: color.New(color.FgGreen)},
		{value: totalDeleted, token: '-', color: color.New(color.FgRed)},
	}
	selectedItems := make([]string, 0, 3)
	for _, item := range items {
		if item.value > 0 {
			formattedItem := item.color.Sprintf("%d(%c)", item.value, item.token)
			selectedItems = append(selectedItems, formattedItem)
		}
	}
	return strings.Join(selectedItems, ",")
}
