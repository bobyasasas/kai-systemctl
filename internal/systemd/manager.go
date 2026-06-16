package systemd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	ManagedPrefix = "kai-"
	ManagedMark   = "X-Kai-Systemctl=managed"
)

var validUnitPart = regexp.MustCompile(`^[A-Za-z0-9_.@-]+$`)

type Manager struct {
	dir string
}

type Unit struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Description string `json:"description"`
	LoadState   string `json:"loadState"`
	ActiveState string `json:"activeState"`
}

type ServiceTemplate struct {
	Description string
	ExecStart   string
	WorkingDir  string
	User        string
}

func NewManager(dir string) *Manager {
	return &Manager{dir: filepath.Clean(dir)}
}

func (m *Manager) List() ([]Unit, error) {
	matches, err := filepath.Glob(filepath.Join(m.dir, ManagedPrefix+"*.service"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)

	units := make([]Unit, 0, len(matches))
	for _, path := range matches {
		name := filepath.Base(path)
		if err := m.ensureManagedName(name); err != nil {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil || !isManagedContent(string(content)) {
			continue
		}
		unit := Unit{
			Name:        name,
			Path:        path,
			Description: parseDescription(string(content)),
		}
		unit.LoadState, unit.ActiveState = queryStates(name)
		units = append(units, unit)
	}
	return units, nil
}

func (m *Manager) Read(name string) (string, error) {
	path, unitName, err := m.resolveExisting(name)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !isManagedContent(string(content)) {
		return "", fmt.Errorf("%s is not managed by kai-systemctl", unitName)
	}
	return string(content), nil
}

func (m *Manager) Create(name, content string) (Unit, error) {
	unitName, err := normalizeName(name)
	if err != nil {
		return Unit{}, err
	}
	path, err := m.safePath(unitName)
	if err != nil {
		return Unit{}, err
	}
	if _, err := os.Stat(path); err == nil {
		return Unit{}, fmt.Errorf("%s already exists", unitName)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Unit{}, err
	}
	content = ensureManagedContent(content)
	if err := validateUnitContent(content); err != nil {
		return Unit{}, err
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return Unit{}, err
	}
	if err := daemonReload(); err != nil {
		return Unit{}, err
	}
	return Unit{Name: unitName, Path: path, Description: parseDescription(content)}, nil
}

func (m *Manager) Update(name, content string) (Unit, error) {
	path, unitName, err := m.resolveExisting(name)
	if err != nil {
		return Unit{}, err
	}
	oldContent, err := os.ReadFile(path)
	if err != nil {
		return Unit{}, err
	}
	if !isManagedContent(string(oldContent)) {
		return Unit{}, fmt.Errorf("%s is not managed by kai-systemctl", unitName)
	}
	content = ensureManagedContent(content)
	if err := validateUnitContent(content); err != nil {
		return Unit{}, err
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return Unit{}, err
	}
	if err := daemonReload(); err != nil {
		return Unit{}, err
	}
	return Unit{Name: unitName, Path: path, Description: parseDescription(content)}, nil
}

func (m *Manager) Delete(name string) error {
	path, unitName, err := m.resolveExisting(name)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !isManagedContent(string(content)) {
		return fmt.Errorf("%s is not managed by kai-systemctl", unitName)
	}
	_ = systemctl("disable", "--now", unitName)
	if err := os.Remove(path); err != nil {
		return err
	}
	return daemonReload()
}

func (m *Manager) Rename(oldName, newName string) (Unit, error) {
	oldPath, oldUnit, err := m.resolveExisting(oldName)
	if err != nil {
		return Unit{}, err
	}
	newUnit, err := normalizeName(newName)
	if err != nil {
		return Unit{}, err
	}
	newPath, err := m.safePath(newUnit)
	if err != nil {
		return Unit{}, err
	}
	if _, err := os.Stat(newPath); err == nil {
		return Unit{}, fmt.Errorf("%s already exists", newUnit)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Unit{}, err
	}
	content, err := os.ReadFile(oldPath)
	if err != nil {
		return Unit{}, err
	}
	if !isManagedContent(string(content)) {
		return Unit{}, fmt.Errorf("%s is not managed by kai-systemctl", oldUnit)
	}
	_ = systemctl("disable", "--now", oldUnit)
	if err := os.Rename(oldPath, newPath); err != nil {
		return Unit{}, err
	}
	if err := daemonReload(); err != nil {
		return Unit{}, err
	}
	return Unit{Name: newUnit, Path: newPath, Description: parseDescription(string(content))}, nil
}

func (m *Manager) Systemctl(action, name string) error {
	_, unitName, err := m.resolveExisting(name)
	if err != nil {
		return err
	}
	switch action {
	case "enable", "disable", "start", "stop", "restart", "status":
	default:
		return fmt.Errorf("unsupported action %q", action)
	}
	return systemctl(action, unitName)
}

func (m *Manager) resolveExisting(name string) (string, string, error) {
	unitName, err := normalizeName(name)
	if err != nil {
		return "", "", err
	}
	path, err := m.safePath(unitName)
	if err != nil {
		return "", "", err
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", fmt.Errorf("%s does not exist", unitName)
		}
		return "", "", err
	}
	return path, unitName, nil
}

func (m *Manager) safePath(unitName string) (string, error) {
	if err := m.ensureManagedName(unitName); err != nil {
		return "", err
	}
	path := filepath.Clean(filepath.Join(m.dir, unitName))
	if filepath.Dir(path) != m.dir {
		return "", fmt.Errorf("unit path escapes %s", m.dir)
	}
	return path, nil
}

func (m *Manager) ensureManagedName(unitName string) error {
	if !strings.HasPrefix(unitName, ManagedPrefix) || !strings.HasSuffix(unitName, ".service") {
		return fmt.Errorf("unit must match %s*.service", ManagedPrefix)
	}
	return nil
}

func normalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".service")
	name = strings.TrimPrefix(name, ManagedPrefix)
	if name == "" {
		return "", fmt.Errorf("unit name is required")
	}
	if strings.Contains(name, "/") || strings.Contains(name, `\`) || strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid unit name %q", name)
	}
	if !validUnitPart.MatchString(name) {
		return "", fmt.Errorf("invalid unit name %q", name)
	}
	return ManagedPrefix + name + ".service", nil
}

func ensureManagedContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimSpace(content) + "\n"
	if isManagedContent(content) {
		return content
	}
	if strings.Contains(content, "[Unit]\n") {
		return strings.Replace(content, "[Unit]\n", "[Unit]\n"+ManagedMark+"\n", 1)
	}
	return "[Unit]\n" + ManagedMark + "\n\n" + content
}

func isManagedContent(content string) bool {
	return strings.Contains(content, ManagedMark)
}

func validateUnitContent(content string) error {
	if !strings.Contains(content, "[Service]") {
		return fmt.Errorf("unit content must include [Service]")
	}
	if !strings.Contains(content, "ExecStart=") {
		return fmt.Errorf("unit content must include ExecStart=")
	}
	return nil
}

func parseDescription(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Description=") {
			return strings.TrimPrefix(line, "Description=")
		}
	}
	return ""
}

func RenderService(t ServiceTemplate) string {
	desc := strings.TrimSpace(t.Description)
	if desc == "" {
		desc = "Kai managed service"
	}
	var b strings.Builder
	b.WriteString("[Unit]\n")
	b.WriteString(ManagedMark + "\n")
	b.WriteString("Description=" + desc + "\n")
	b.WriteString("After=network.target\n\n")
	b.WriteString("[Service]\n")
	b.WriteString("Type=simple\n")
	if strings.TrimSpace(t.User) != "" {
		b.WriteString("User=" + strings.TrimSpace(t.User) + "\n")
	}
	if strings.TrimSpace(t.WorkingDir) != "" {
		b.WriteString("WorkingDirectory=" + strings.TrimSpace(t.WorkingDir) + "\n")
	}
	b.WriteString("ExecStart=" + strings.TrimSpace(t.ExecStart) + "\n")
	b.WriteString("Restart=on-failure\n")
	b.WriteString("RestartSec=3\n\n")
	b.WriteString("[Install]\n")
	b.WriteString("WantedBy=multi-user.target\n")
	return b.String()
}

func Command(name string, args ...string) *exec.Cmd {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return exec.Command(name, args...)
	}
	allArgs := append(parts[1:], args...)
	return exec.Command(parts[0], allArgs...)
}

func daemonReload() error {
	return systemctl("daemon-reload")
}

func systemctl(args ...string) error {
	cmd := exec.Command("systemctl", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("systemctl %s failed: %s", strings.Join(args, " "), msg)
	}
	return nil
}

func queryStates(unitName string) (string, string) {
	cmd := exec.Command("systemctl", "show", unitName, "--property=LoadState", "--property=ActiveState", "--value")
	out, err := cmd.Output()
	if err != nil {
		return "unknown", "unknown"
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) >= 2 {
		return strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1])
	}
	return "unknown", "unknown"
}
