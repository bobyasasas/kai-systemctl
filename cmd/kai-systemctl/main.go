package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/bobyasasas/kai-systemctl/internal/systemd"
	"github.com/bobyasasas/kai-systemctl/internal/web"
)

const defaultDir = "/etc/systemd/system"

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	manager := systemd.NewManager(defaultDir)
	if len(args) == 0 {
		return runInteractive(manager)
	}

	switch args[0] {
	case "host":
		return runWeb(manager, args[1:])
	case "list":
		return runList(manager)
	case "show":
		return runShow(manager, args[1:])
	case "new", "create":
		return runCreate(manager, args[1:])
	case "delete", "remove", "rm":
		return runDelete(manager, args[1:])
	case "rename", "mv":
		return runRename(manager, args[1:])
	case "edit":
		return runEdit(manager, args[1:])
	case "enable", "disable", "start", "stop", "restart", "status":
		return runAction(manager, args[0], args[1:])
	case "version":
		fmt.Println(version)
		return nil
	case "help", "-h", "--help":
		return usage()
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usageText())
	}
}

func runWeb(manager *systemd.Manager, args []string) error {
	fs := flag.NewFlagSet("host", flag.ExitOnError)
	host := fs.String("host", "127.0.0.1", "listen host")
	port := fs.String("port", "8080", "listen port")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("usage: kai-systemctl host [host] [-port 8080]")
	}
	if fs.NArg() == 1 {
		*host = fs.Arg(0)
	}

	addr := *host + ":" + *port
	server := &http.Server{
		Addr:              addr,
		Handler:           web.NewServer(manager),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("kai-systemctl web listening on http://%s", addr)
		errCh <- server.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("received %s, shutting down", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func runList(manager *systemd.Manager) error {
	units, err := manager.List()
	if err != nil {
		return err
	}
	if len(units) == 0 {
		fmt.Println("no kai-managed units found")
		return nil
	}
	for _, unit := range units {
		fmt.Printf("%-36s %-10s %-10s %s\n", unit.Name, unit.LoadState, unit.ActiveState, unit.Description)
	}
	return nil
}

func runShow(manager *systemd.Manager, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: kai-systemctl show <name>")
	}
	content, err := manager.Read(args[0])
	if err != nil {
		return err
	}
	fmt.Print(content)
	return nil
}

func runCreate(manager *systemd.Manager, args []string) error {
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	desc := fs.String("description", "", "service description")
	execStart := fs.String("exec", "", "ExecStart command")
	workdir := fs.String("workdir", "", "WorkingDirectory")
	user := fs.String("user", "", "User")
	contentFile := fs.String("file", "", "read full unit content from file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: kai-systemctl new <name> [-description text] [-exec command] [-workdir dir] [-user user]\n       kai-systemctl new <name> -file unit.service")
	}

	var content string
	if *contentFile != "" {
		b, err := os.ReadFile(*contentFile)
		if err != nil {
			return err
		}
		content = string(b)
	} else {
		if *execStart == "" {
			return fmt.Errorf("-exec is required when -file is not provided")
		}
		content = systemd.RenderService(systemd.ServiceTemplate{
			Description: *desc,
			ExecStart:   *execStart,
			WorkingDir:  *workdir,
			User:        *user,
		})
	}

	unit, err := manager.Create(fs.Arg(0), content)
	if err != nil {
		return err
	}
	fmt.Println(unit.Name)
	return nil
}

func runDelete(manager *systemd.Manager, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: kai-systemctl delete <name>")
	}
	return manager.Delete(args[0])
}

func runRename(manager *systemd.Manager, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: kai-systemctl rename <old-name> <new-name>")
	}
	unit, err := manager.Rename(args[0], args[1])
	if err != nil {
		return err
	}
	fmt.Println(unit.Name)
	return nil
}

func runEdit(manager *systemd.Manager, args []string) error {
	fs := flag.NewFlagSet("edit", flag.ExitOnError)
	contentFile := fs.String("file", "", "read full unit content from file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: kai-systemctl edit <name> [-file unit.service]")
	}

	if *contentFile != "" {
		b, err := os.ReadFile(*contentFile)
		if err != nil {
			return err
		}
		_, err = manager.Update(fs.Arg(0), string(b))
		return err
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		return fmt.Errorf("EDITOR is not set; use kai-systemctl edit <name> -file unit.service")
	}
	tmp, err := os.CreateTemp("", "kai-systemctl-*.service")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	content, err := manager.Read(fs.Arg(0))
	if err != nil {
		return err
	}
	if _, err := tmp.WriteString(content); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	cmd := systemd.Command(editor, tmp.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	edited, err := os.ReadFile(tmp.Name())
	if err != nil {
		return err
	}
	_, err = manager.Update(fs.Arg(0), string(edited))
	return err
}

func runAction(manager *systemd.Manager, action string, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: kai-systemctl %s <name>", action)
	}
	return manager.Systemctl(action, args[0])
}

func usage() error {
	fmt.Print(usageText())
	return nil
}

func usageText() string {
	bin := filepath.Base(os.Args[0])
	lines := []string{
		"Kai Systemctl manages only kai-created systemd units in /etc/systemd/system.",
		"",
		"Usage:",
		"  " + bin + " list",
		"  " + bin + " new <name> -exec '/usr/bin/example' [-description text] [-workdir dir] [-user user]",
		"  " + bin + " new <name> -file ./unit.service",
		"  " + bin + " show <name>",
		"  " + bin + " edit <name> [-file ./unit.service]",
		"  " + bin + " rename <old-name> <new-name>",
		"  " + bin + " delete <name>",
		"  " + bin + " enable|disable|start|stop|restart|status <name>",
		"  " + bin + " host 0.0.0.0 -port 8080",
		"  " + bin + " version",
		"",
		"Run without arguments to open the interactive CLI.",
		"",
		"Unit names are normalized to kai-<name>.service.",
	}
	return strings.Join(lines, "\n") + "\n"
}
