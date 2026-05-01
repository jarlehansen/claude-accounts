package main

import (
	"fmt"
	"os"
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
		if len(args) < 2 {
			return fmt.Errorf("usage: claude-accounts create <name>")
		}
		if err := validateProfileName(args[1]); err != nil {
			return err
		}
		return cmdCreate(args[1])
	case "list":
		return cmdList()
	case "current":
		return cmdCurrent()
	case "delete":
		if len(args) < 2 {
			return fmt.Errorf("usage: claude-accounts delete <name>")
		}
		if err := validateProfileName(args[1]); err != nil {
			return err
		}
		return cmdDelete(args[1])
	case "reauth":
		if len(args) < 2 {
			return fmt.Errorf("usage: claude-accounts reauth <name>")
		}
		if err := validateProfileName(args[1]); err != nil {
			return err
		}
		return cmdReauth(args[1])
	case "help", "-h", "--help":
		printHelp()
		return nil
	default:
		if err := validateProfileName(args[0]); err != nil {
			return err
		}
		return cmdSwitch(args[0])
	}
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
  claude-accounts reauth <name>    re-run login for an existing profile`)
}
