package systemd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeName(t *testing.T) {
	tests := map[string]string{
		"demo":             "kai-demo.service",
		"kai-demo":         "kai-demo.service",
		"kai-demo.service": "kai-demo.service",
	}
	for in, want := range tests {
		got, err := normalizeName(in)
		if err != nil {
			t.Fatalf("normalizeName(%q): %v", in, err)
		}
		if got != want {
			t.Fatalf("normalizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeNameRejectsTraversal(t *testing.T) {
	for _, name := range []string{"../x", "x/y", "..x"} {
		if _, err := normalizeName(name); err == nil {
			t.Fatalf("normalizeName(%q) succeeded", name)
		}
	}
}

func TestEnsureManagedContent(t *testing.T) {
	content := "[Unit]\nDescription=test\n\n[Service]\nExecStart=/bin/true\n"
	got := ensureManagedContent(content)
	if !isManagedContent(got) {
		t.Fatalf("expected managed content marker")
	}
}

func TestManagerCreateRefusesExisting(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	path := filepath.Join(dir, "kai-demo.service")
	if err := os.WriteFile(path, []byte(RenderService(ServiceTemplate{ExecStart: "/bin/true"})), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create("demo", RenderService(ServiceTemplate{ExecStart: "/bin/true"})); err == nil {
		t.Fatal("expected duplicate create to fail")
	}
}
