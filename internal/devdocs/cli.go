package devdocs

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// Version is set at release build time with -ldflags -X.
var Version = "dev"

const usageText = `Devdocs: Local-first CLI for private project context.

Usage:
  devdocs <command> [flags] [arguments]

Commands:
  config             Display or update global configuration
  init               Initialize a new devdocs project workspace
  list               List attachable project branches on remote
  attach             Attach an existing project branch to this repository
  status             Show offline workspace status, changes, and health
  sync               Snapshot changes, integrate remote, and push
  link               Create a relative symlink from code root to devdocs
  unlink             Remove a locally owned symlink and declaration
  git                Run native Git in this project's docs checkout

Flags:
  -h, --help         Show help for devdocs or a command
  -v, --version      Show devdocs version

Run 'devdocs <command> --help' for details on a specific command.
`

// Run executes the CLI with the provided arguments and IO streams, returning an exit code.
func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(out, usageText)
		return 0
	}

	first := args[0]
	switch first {
	case "-h", "--help", "help":
		fmt.Fprint(out, usageText)
		return 0
	case "-v", "--version", "version":
		fmt.Fprintf(out, "devdocs version %s\n", Version)
		return 0
	}

	subcommand := args[0]
	subArgs := args[1:]

	var err error
	switch subcommand {
	case "config":
		err = runConfig(ctx, subArgs, in, out, errOut)
	case "init":
		err = runInit(ctx, subArgs, out, errOut)
	case "list":
		err = runList(ctx, subArgs, out, errOut)
	case "attach":
		err = runAttach(ctx, subArgs, out, errOut)
	case "status":
		err = runStatus(ctx, subArgs, out, errOut)
	case "sync":
		err = runSync(ctx, subArgs, out, errOut)
	case "link":
		err = runLink(ctx, subArgs, out, errOut)
	case "unlink":
		err = runUnlink(ctx, subArgs, out, errOut)
	case "git":
		return runDocsGit(ctx, subArgs, in, out, errOut)
	default:
		fmt.Fprintf(errOut, "error: unknown command %q\nRun 'devdocs --help' for usage.\n", subcommand)
		return 1
	}

	if err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintf(errOut, "error: %v\n", err)
		return 1
	}

	return 0
}

func runConfig(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) error {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	fs.SetOutput(errOut)
	remoteFlag := fs.String("remote", "", "Set the default remote URL for future workspaces")
	editFlag := fs.Bool("edit", false, "Open configuration file in $VISUAL or $EDITOR")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	if *editFlag && *remoteFlag != "" {
		return errors.New("cannot specify both --remote and --edit")
	}

	if *editFlag {
		return EditConfigFile(in, out, errOut)
	}

	if *remoteFlag != "" {
		cfg, _, err := LoadConfig()
		if err != nil {
			return err
		}
		cfg.DefaultRemote = *remoteFlag
		cfgPath, err := SaveConfig(cfg)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Updated default remote in %s\nRemote: %s\n", cfgPath, RedactRemoteURL(*remoteFlag))
		return nil
	}

	// Show current configuration
	cfg, cfgPath, err := LoadConfig()
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Configuration file: %s\n", cfgPath)
	if cfg.DefaultRemote != "" {
		fmt.Fprintf(out, "Default remote:     %s\n", RedactRemoteURL(cfg.DefaultRemote))
	} else {
		fmt.Fprintf(out, "Default remote:     (not set; run 'devdocs config --remote <url>')\n")
	}

	return nil
}

func runInit(ctx context.Context, args []string, out, errOut io.Writer) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(errOut)
	nameFlag := fs.String("name", "", "Human-readable display name for the project")
	adoptFlag := fs.Bool("adopt", false, "Initialize existing unmanaged devdocs files in place without overwriting them")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	return Init(ctx, *nameFlag, *adoptFlag, out, errOut)
}

func runList(ctx context.Context, args []string, out, errOut io.Writer) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(errOut)

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	return List(ctx, out, errOut)
}

func runAttach(ctx context.Context, args []string, out, errOut io.Writer) error {
	fs := flag.NewFlagSet("attach", flag.ContinueOnError)
	fs.SetOutput(errOut)

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() != 1 {
		return errors.New("usage: devdocs attach <project-branch>")
	}

	return Attach(ctx, fs.Arg(0), out, errOut)
}

func runStatus(ctx context.Context, args []string, out, errOut io.Writer) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(errOut)

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	return Status(ctx, out, errOut)
}

func runSync(ctx context.Context, args []string, out, errOut io.Writer) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(errOut)
	msgFlag := fs.String("m", "", "Commit message for local changes")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	return Sync(ctx, *msgFlag, out, errOut)
}

func runLink(ctx context.Context, args []string, out, errOut io.Writer) error {
	fs := flag.NewFlagSet("link", flag.ContinueOnError)
	fs.SetOutput(errOut)
	applyFlag := fs.Bool("apply", false, "Apply all declared links in project metadata")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *applyFlag {
		if fs.NArg() > 0 {
			return errors.New("no arguments allowed with 'devdocs link --apply'")
		}
		return ApplyLinks(ctx, out, errOut)
	}

	if fs.NArg() != 2 {
		return errors.New("usage: devdocs link <source> <destination> or devdocs link --apply")
	}

	return Link(ctx, fs.Arg(0), fs.Arg(1), out, errOut)
}

func runUnlink(ctx context.Context, args []string, out, errOut io.Writer) error {
	fs := flag.NewFlagSet("unlink", flag.ContinueOnError)
	fs.SetOutput(errOut)

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() != 1 {
		return errors.New("usage: devdocs unlink <destination>")
	}

	return Unlink(ctx, fs.Arg(0), out, errOut)
}
