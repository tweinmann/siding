package stack

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestSlug(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, in, want string
	}{
		{name: "plain", in: "main", want: "main"},
		{name: "slash and case", in: "feature/Login_Form", want: "feature-login-form"},
		{name: "runs of symbols", in: "a//b__c..d", want: "a-b-c-d"},
		{name: "leading and trailing", in: "-/x/-", want: "x"},
		{name: "non-ASCII", in: "fix/über", want: "fix-ber"},
		{name: "40 characters stay", in: strings.Repeat("a", 40), want: strings.Repeat("a", 40)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Slug(tt.in); got != tt.want {
				t.Errorf("Slug(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}

	hashed := []struct {
		name, in string
		shape    *regexp.Regexp
	}{
		{name: "41 characters are cut", in: strings.Repeat("a", 41), shape: regexp.MustCompile(`^a{33}-[0-9a-f]{6}$`)},
		{name: "60 characters are cut", in: strings.Repeat("abcdefghij", 6), shape: regexp.MustCompile(`^(abcdefghij){3}abc-[0-9a-f]{6}$`)},
		{name: "only symbols", in: "///", shape: regexp.MustCompile(`^x-[0-9a-f]{6}$`)},
	}
	for _, tt := range hashed {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Slug(tt.in); !tt.shape.MatchString(got) {
				t.Errorf("Slug(%q) = %q, want it to match %s", tt.in, got, tt.shape)
			}
		})
	}

	t.Run("long names with the same start differ", func(t *testing.T) {
		t.Parallel()
		long := strings.Repeat("abcdefghij", 6)
		if a, b := Slug(long), Slug(long[:59]+"z"); a == b {
			t.Errorf("both slugs are %q", a)
		}
	})
}

func TestIdentity(t *testing.T) {
	t.Parallel()
	id := Identity{Repo: "shop", Branch: "feature/login", Worktree: "/src/shop-login", Machine: "m1"}
	if got := id.Project(); got != "shop-feature-login" {
		t.Errorf("Project = %q", got)
	}
	if got := id.Host(); got != "shop.feature-login.localhost" {
		t.Errorf("Host = %q", got)
	}
	if got := id.ServiceHost("Mail_Hog"); got != "mail-hog.shop.feature-login.localhost" {
		t.Errorf("ServiceHost = %q", got)
	}
	if _, ok := id.ContainerLabels()[LabelAgent]; ok {
		t.Error("an agent label without an agent")
	}
	id.Agent = "claude"
	if got := id.ContainerLabels()[LabelAgent]; got != "claude" {
		t.Errorf("agent label = %q", got)
	}
	if _, ok := id.Labels()[LabelAgent]; ok {
		t.Error("networks and volumes must not carry the agent label")
	}
	id.GitDir = "/src/shop/.git"
	if got := id.ContainerLabels()[LabelGitDir]; got != "/src/shop/.git" {
		t.Errorf("gitdir label = %q", got)
	}
	if _, ok := id.Labels()[LabelGitDir]; ok {
		t.Error("networks and volumes must not carry the gitdir label")
	}
	if got := FromLabels(id.ContainerLabels()); got != id {
		t.Errorf("FromLabels = %+v, want %+v", got, id)
	}
	want := map[string]string{LabelRepo: "shop", LabelBranch: "feature/login", LabelWorktree: "/src/shop-login", LabelMachine: "m1"}
	for k, v := range want {
		if id.Labels()[k] != v {
			t.Errorf("label %s = %q, want %q", k, id.Labels()[k], v)
		}
	}
}

func TestMachineID(t *testing.T) {
	t.Parallel()

	t.Run("from the operating system", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		a, err := MachineID("4c4c4544-not-a-real-id\n", home)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := MachineID("4c4c4544-not-a-real-id", home)
		if a != b || len(a) != 16 || strings.Contains(a, "4c4c4544") {
			t.Errorf("ids %q and %q: want the same 16 hex digits, without the raw id", a, b)
		}
		if _, err := os.Stat(filepath.Join(home, ".siding")); err == nil {
			t.Error("wrote ~/.siding although the operating system has an id")
		}
	})

	t.Run("generated once", func(t *testing.T) {
		t.Parallel()
		home := t.TempDir()
		a, err := MachineID("", home)
		if err != nil {
			t.Fatal(err)
		}
		b, err := MachineID("", home)
		if err != nil || a != b {
			t.Fatalf("second call: %q, %v; want %q", b, err, a)
		}
		saved, _ := os.ReadFile(filepath.Join(home, ".siding", "machine-id"))
		if strings.TrimSpace(string(saved)) != a {
			t.Errorf("~/.siding/machine-id holds %q, want %q", saved, a)
		}
	})
}
