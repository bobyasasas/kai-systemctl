package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/bobyasasas/kai-systemctl/internal/systemd"
)

type interactiveShell struct {
	manager *systemd.Manager
	reader  *bufio.Reader
}

func runInteractive(manager *systemd.Manager) error {
	shell := &interactiveShell{
		manager: manager,
		reader:  bufio.NewReader(os.Stdin),
	}
	return shell.loop()
}

func (s *interactiveShell) loop() error {
	fmt.Println("Kai Systemctl interactive CLI")
	fmt.Println("Only kai-created units in /etc/systemd/system are managed.")

	for {
		fmt.Println()
		fmt.Println("1) List units")
		fmt.Println("2) Create unit")
		fmt.Println("3) Show unit")
		fmt.Println("4) Edit unit")
		fmt.Println("5) Rename unit")
		fmt.Println("6) Delete unit")
		fmt.Println("7) Systemctl action")
		fmt.Println("8) Open web UI")
		fmt.Println("9) Help")
		fmt.Println("0) Exit")

		choice, err := s.ask("Select")
		if err != nil {
			return err
		}

		switch strings.TrimSpace(choice) {
		case "1", "list", "l":
			s.report(s.list())
		case "2", "create", "new", "n":
			s.report(s.create())
		case "3", "show", "s":
			s.report(s.show())
		case "4", "edit", "e":
			s.report(s.edit())
		case "5", "rename", "mv", "r":
			s.report(s.rename())
		case "6", "delete", "remove", "rm", "d":
			s.report(s.delete())
		case "7", "action", "systemctl", "a":
			s.report(s.action())
		case "8", "web", "host", "w":
			return s.openWeb()
		case "9", "help", "h", "?":
			fmt.Print(usageText())
		case "0", "exit", "quit", "q":
			return nil
		default:
			fmt.Println("Unknown option.")
		}
	}
}

func (s *interactiveShell) list() error {
	return runList(s.manager)
}

func (s *interactiveShell) create() error {
	name, err := s.askRequired("Name")
	if err != nil {
		return err
	}
	mode, err := s.askDefault("Create mode: 1) guided 2) paste unit content", "1")
	if err != nil {
		return err
	}

	var content string
	if mode == "2" {
		fmt.Println("Paste full unit content. Finish with a single line containing only END.")
		content, err = s.readUntilEnd()
		if err != nil {
			return err
		}
	} else {
		desc, err := s.ask("Description")
		if err != nil {
			return err
		}
		execStart, err := s.askRequired("ExecStart")
		if err != nil {
			return err
		}
		workdir, err := s.ask("WorkingDirectory")
		if err != nil {
			return err
		}
		user, err := s.ask("User")
		if err != nil {
			return err
		}
		content = systemd.RenderService(systemd.ServiceTemplate{
			Description: desc,
			ExecStart:   execStart,
			WorkingDir:  workdir,
			User:        user,
		})
	}

	unit, err := s.manager.Create(name, content)
	if err != nil {
		return err
	}
	fmt.Println("Created:", unit.Name)
	return nil
}

func (s *interactiveShell) show() error {
	name, err := s.selectUnit("Select unit")
	if err != nil {
		return err
	}
	content, err := s.manager.Read(name)
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Print(content)
	return nil
}

func (s *interactiveShell) edit() error {
	name, err := s.selectUnit("Select unit")
	if err != nil {
		return err
	}
	fmt.Println("Paste replacement unit content. Finish with a single line containing only END.")
	content, err := s.readUntilEnd()
	if err != nil {
		return err
	}
	unit, err := s.manager.Update(name, content)
	if err != nil {
		return err
	}
	fmt.Println("Updated:", unit.Name)
	return nil
}

func (s *interactiveShell) rename() error {
	oldName, err := s.selectUnit("Select unit")
	if err != nil {
		return err
	}
	newName, err := s.askRequired("New name")
	if err != nil {
		return err
	}
	unit, err := s.manager.Rename(oldName, newName)
	if err != nil {
		return err
	}
	fmt.Println("Renamed to:", unit.Name)
	return nil
}

func (s *interactiveShell) delete() error {
	name, err := s.selectUnit("Select unit")
	if err != nil {
		return err
	}
	ok, err := s.confirm("Delete " + name + "? This will disable --now before removing the unit")
	if err != nil {
		return err
	}
	if !ok {
		fmt.Println("Canceled.")
		return nil
	}
	if err := s.manager.Delete(name); err != nil {
		return err
	}
	fmt.Println("Deleted:", name)
	return nil
}

func (s *interactiveShell) action() error {
	name, err := s.selectUnit("Select unit")
	if err != nil {
		return err
	}
	action, err := s.askRequired("Action (start/stop/restart/enable/disable/status)")
	if err != nil {
		return err
	}
	action = strings.TrimSpace(action)
	switch action {
	case "stop", "restart", "disable":
		ok, err := s.confirm("Run systemctl " + action + " on " + name)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("Canceled.")
			return nil
		}
	}
	if err := s.manager.Systemctl(action, name); err != nil {
		return err
	}
	fmt.Println("Done:", action, name)
	return nil
}

func (s *interactiveShell) openWeb() error {
	host, err := s.askDefault("Host", "127.0.0.1")
	if err != nil {
		return err
	}
	port, err := s.askDefault("Port", "8080")
	if err != nil {
		return err
	}
	fmt.Println("Starting web UI. Press Ctrl+C to stop.")
	return runWeb(s.manager, []string{host, "-port", port})
}

func (s *interactiveShell) selectUnit(label string) (string, error) {
	units, err := s.manager.List()
	if err != nil {
		return "", err
	}
	if len(units) == 0 {
		return "", fmt.Errorf("no kai-managed units found")
	}

	fmt.Println(label + ":")
	for i, unit := range units {
		desc := unit.Description
		if desc != "" {
			desc = " - " + desc
		}
		fmt.Printf("%d) %s [%s/%s]%s\n", i+1, unit.Name, unit.LoadState, unit.ActiveState, desc)
	}

	for {
		answer, err := s.ask("Number")
		if err != nil {
			return "", err
		}
		n, err := strconv.Atoi(answer)
		if err != nil || n < 1 || n > len(units) {
			fmt.Printf("Enter a number from 1 to %d.\n", len(units))
			continue
		}
		return units[n-1].Name, nil
	}
}

func (s *interactiveShell) ask(label string) (string, error) {
	fmt.Print(label + ": ")
	text, err := s.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

func (s *interactiveShell) askRequired(label string) (string, error) {
	for {
		text, err := s.ask(label)
		if err != nil {
			return "", err
		}
		if text != "" {
			return text, nil
		}
		fmt.Println(label + " is required.")
	}
}

func (s *interactiveShell) askDefault(label, def string) (string, error) {
	text, err := s.ask(label + " [" + def + "]")
	if err != nil {
		return "", err
	}
	if text == "" {
		return def, nil
	}
	return text, nil
}

func (s *interactiveShell) confirm(label string) (bool, error) {
	answer, err := s.ask(label + " [y/N]")
	if err != nil {
		return false, err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}

func (s *interactiveShell) readUntilEnd() (string, error) {
	var b strings.Builder
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(line) == "END" {
			break
		}
		b.WriteString(line)
	}
	return b.String(), nil
}

func (s *interactiveShell) report(err error) {
	if err != nil {
		fmt.Println("Error:", err)
	}
}
