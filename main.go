package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

var version = "dev"

const DefaultOutputFileName = "dewdrops_context.md"

type mapFlagValue struct {
	enabled bool
	filter  string
}

func (f *mapFlagValue) String() string   { return f.filter }
func (f *mapFlagValue) IsBoolFlag() bool { return true }
func (f *mapFlagValue) Set(s string) error {
	f.enabled = true
	if s == "true" {
		f.filter = ""
	} else {
		f.filter = s
	}
	return nil
}

func main() {
	var mapVal mapFlagValue
	flag.Var(&mapVal, "map", "Output structural map (use --map=go,py to filter by extension, --map=any for all text files)")
	fromFlag := flag.String("from", "", "Comma-separated list of file/dir paths to include")
	sinceFlag := flag.String("since", "", "Git ref to diff against HEAD (branch, tag, hash, HEAD~N)")
	outputFlag := flag.String("o", "", "Output file path (default: dewdrops_context.md)")
	versionFlag := flag.Bool("version", false, "Print version and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: dewdrops [options] <repo-path>

Options:
  --map[=exts]       Output structural map (tree + signatures + token estimates).
                     Optionally filter by extensions (comma-separated, e.g.
                     --map=go,py). Default: supported languages only.
                     Use --map=any for all text files.
  --from <paths>     Only include specified files/dirs (comma-separated, relative
                     to repo root). Example: --from src/main.go,internal/auth/
  --since <ref>      Diff-aware output: map + diff + content for files changed
                     between <ref> and HEAD. Accepts branch names, tags, commit
                     hashes, or relative refs like HEAD~3.
                     Cannot be combined with --map or --from.
  -o <path>          Output file path (default: dewdrops_context.md)
  --version          Print version and exit
  -h, --help         Show this help message

Examples:
  dewdrops .                                        # Full repo dump
  dewdrops --map .                                  # Structural overview only
  dewdrops --from internal/auth/ .                  # Dump specific directory
  dewdrops --map --from internal/auth/,cmd/ .       # Map of specific subtree
  dewdrops --since main .                           # Review changes vs main
  dewdrops --since HEAD~3 -o review.md .            # Last 3 commits, custom path
`)
	}
	flag.Parse()

	if *versionFlag {
		fmt.Printf("dewdrops %s\n", version)
		os.Exit(0)
	}

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Error: Missing required argument <repo-path>")
		flag.Usage()
		os.Exit(1)
	}

	rootDir := flag.Arg(0)
	opts := RunOptions{
		MapMode:    mapVal.enabled,
		MapFilter:  mapVal.filter,
		OutputFile: DefaultOutputFileName,
	}
	if *outputFlag != "" {
		opts.OutputFile = *outputFlag
	}
	if *sinceFlag != "" {
		opts.SinceRef = *sinceFlag
		if *outputFlag == "" {
			opts.OutputFile = sinceOutputFileName(*sinceFlag)
		}
	}
	if *fromFlag != "" {
		opts.FromPaths = strings.Split(*fromFlag, ",")
	}

	if err := Run(rootDir, opts); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
}
