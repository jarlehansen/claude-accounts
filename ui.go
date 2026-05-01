package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// safeDisplay strips C0/DEL control characters so values read from disk
// (e.g. the email field in claude.json) can't inject ANSI sequences into
// the TUI rendering.
func safeDisplay(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

var (
	menuTheme  = buildTheme()
	menuKeyMap = buildKeyMap()
)

func buildTheme() *huh.Theme {
	t := huh.ThemeCatppuccin()
	// Bolder selection indicator and bold-weight selected text.
	t.Focused.SelectSelector = t.Focused.SelectSelector.SetString(" ▶  ")
	t.Focused.SelectedOption = t.Focused.SelectedOption.Bold(true)
	// Breathing room above and below the logo and the description.
	t.Focused.Title = t.Focused.Title.MarginTop(1).MarginBottom(1)
	t.Focused.Description = t.Focused.Description.MarginBottom(1)
	return t
}

func buildKeyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	km.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("ctrl+c/esc", "quit"),
	)
	return km
}

// runField wraps a single field in a form so the shared theme, keymap
// (including esc-to-quit), and alt-screen rendering are applied consistently
// everywhere. Alt-screen means each form starts on a clean buffer instead of
// stacking on top of the previous form's leftover output.
func runField(f huh.Field) error {
	return huh.NewForm(huh.NewGroup(f)).
		WithTheme(menuTheme).
		WithKeyMap(menuKeyMap).
		WithProgramOptions(tea.WithAltScreen()).
		Run()
}

const logo = `
   ┌───┐    ██████╗██╗      █████╗ ██╗   ██╗██████╗ ███████╗
   │o o│   ██╔════╝██║     ██╔══██╗██║   ██║██╔══██╗██╔════╝
 ──┤ - ├── ██║     ██║     ███████║██║   ██║██║  ██║█████╗
   └─┬─┘   ██║     ██║     ██╔══██║██║   ██║██║  ██║██╔══╝
   ┌─┴─┐   ╚██████╗███████╗██║  ██║╚██████╔╝██████╔╝███████╗
   └───┘    ╚═════╝╚══════╝╚═╝  ╚═╝ ╚═════╝ ╚═════╝ ╚══════╝
                              accounts`

func runMenu() error {
	store, err := NewStore()
	if err != nil {
		return err
	}
	_ = store.ReconcileCurrent()

	for {
		current, _ := store.Current()
		desc := "switch between Claude Code logins  ·  esc to quit"
		if current != "" {
			active := current
			if email := safeDisplay(store.Email(current)); email != "" {
				active = current + " (" + email + ")"
			}
			desc = "active: " + active + "  ·  esc to quit"
		}

		var choice string
		err = runField(
			huh.NewSelect[string]().
				Title(logo).
				Description(desc).
				Options(
					huh.NewOption("switch profile", "list"),
					huh.NewOption("create profile", "create"),
					huh.NewOption("reauthenticate profile", "reauth"),
					huh.NewOption("delete profile", "delete"),
					huh.NewOption("quit", "quit"),
				).
				Value(&choice),
		)
		if err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return nil
			}
			return err
		}

		switch choice {
		case "create":
			name, err := promptName("profile name", "e.g. work")
			if err != nil {
				if errors.Is(err, huh.ErrUserAborted) {
					continue
				}
				return err
			}
			return cmdCreate(name)
		case "list":
			switched, err := runListMenu()
			if err != nil {
				return err
			}
			if switched {
				return nil
			}
		case "reauth":
			name, _, err := pickProfile(profilePickerOpts{title: "reauthenticate profile"})
			if err != nil {
				return err
			}
			if name == "" {
				continue
			}
			return cmdReauth(name)
		case "delete":
			if err := runDeleteMenu(); err != nil {
				return err
			}
		case "quit":
			return nil
		}
	}
}

func runListMenu() (bool, error) {
	choice, current, err := pickProfile(profilePickerOpts{title: "switch profile"})
	if err != nil || choice == "" {
		return false, err
	}
	if choice == current {
		fmt.Printf("%s is already the active profile\n", choice)
		return false, nil
	}
	if err := cmdSwitch(choice); err != nil {
		return false, err
	}
	return true, nil
}

type profilePickerOpts struct {
	title        string
	description  string // defaults to "esc to go back"
	activeSuffix string // shown after active profile's label; defaults to "(active)"
	emptyMessage string // printed when there are no profiles; has a default
}

// pickProfile shows a picker of existing profiles and returns the chosen
// name and the currently active profile. Returns ("", current, nil) if the
// user backs out (esc / "← back") or there are no profiles.
func pickProfile(opts profilePickerOpts) (choice, current string, err error) {
	if opts.description == "" {
		opts.description = "esc to go back"
	}
	if opts.activeSuffix == "" {
		opts.activeSuffix = "(active)"
	}
	if opts.emptyMessage == "" {
		opts.emptyMessage = "no profiles yet — run `claude-accounts create <name>` to get started"
	}

	store, err := NewStore()
	if err != nil {
		return "", "", err
	}
	names, err := store.List()
	if err != nil {
		return "", "", err
	}
	current, _ = store.Current()

	if len(names) == 0 {
		fmt.Println(opts.emptyMessage)
		return "", current, nil
	}

	options := make([]huh.Option[string], 0, len(names)+1)
	for _, n := range names {
		label := n
		if email := safeDisplay(store.Email(n)); email != "" {
			label += " — " + email
		}
		if n == current {
			label += "  " + opts.activeSuffix
		}
		options = append(options, huh.NewOption(label, n))
	}
	options = append(options, huh.NewOption("← back", "__back"))

	err = runField(
		huh.NewSelect[string]().
			Title(opts.title).
			Description(opts.description).
			Options(options...).
			Value(&choice),
	)
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", current, nil
		}
		return "", current, err
	}
	if choice == "__back" {
		return "", current, nil
	}
	return choice, current, nil
}

func promptName(title, placeholder string) (string, error) {
	var name string
	err := runField(
		huh.NewInput().
			Title(title).
			Placeholder(placeholder).
			Validate(validateProfileName).
			Value(&name),
	)
	return name, err
}

func confirmOverwrite(name string) (bool, error) {
	var ok bool
	err := runField(
		huh.NewConfirm().
			Title(fmt.Sprintf("profile %q already exists — overwrite?", name)).
			Affirmative("yes, overwrite").
			Negative("no, cancel").
			Value(&ok),
	)
	if errors.Is(err, huh.ErrUserAborted) {
		return false, nil
	}
	return ok, err
}

func runDeleteMenu() error {
	choice, current, err := pickProfile(profilePickerOpts{
		title:        "delete profile",
		activeSuffix: "(active — cannot delete)",
		emptyMessage: "no profiles to delete",
	})
	if err != nil || choice == "" {
		return err
	}
	if choice == current {
		fmt.Printf("cannot delete %q: it is the active profile\n", choice)
		return nil
	}

	var confirmed bool
	err = runField(
		huh.NewConfirm().
			Title(fmt.Sprintf("delete profile %q?", choice)).
			Affirmative("yes, delete").
			Negative("no, cancel").
			Value(&confirmed),
	)
	if err != nil || !confirmed {
		return err
	}
	return cmdDelete(choice)
}
