package main

import (
	"fmt"
	"os"
	"runtime/debug"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return runMenu()
	}

	switch args[0] {
	case "create":
		name, err := nameArg(args, "create")
		if err != nil {
			return err
		}
		return cmdCreate(name)
	case "list":
		return cmdList()
	case "current":
		return cmdCurrent()
	case "delete":
		name, err := nameArg(args, "delete")
		if err != nil {
			return err
		}
		return cmdDelete(name)
	case "reauth":
		name, err := nameArg(args, "reauth")
		if err != nil {
			return err
		}
		return cmdReauth(name)
	case "help", "-h", "--help":
		printHelp()
		return nil
	case "version", "-v", "--version":
		fmt.Println("claude-accounts", versionString())
		return nil
	default:
		if err := validateProfileName(args[0]); err != nil {
			return err
		}
		return cmdSwitch(args[0])
	}
}

func nameArg(args []string, cmd string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("usage: claude-accounts %s <name>", cmd)
	}
	if err := validateProfileName(args[1]); err != nil {
		return "", err
	}
	return args[1], nil
}

func printHelp() {
	fmt.Println(`claude-accounts — switch between Claude Code logins

Usage:
  claude-accounts                  open interactive menu
  claude-accounts <name>           switch to profile <name>
  claude-accounts create <name>    create a new profile by logging in
  claude-accounts list             list profiles
  claude-accounts current          print active profile
  claude-accounts delete <name>    delete a profile
  claude-accounts reauth <name>    re-run login for an existing profile
  claude-accounts version          print version`)
}

func versionString() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}
