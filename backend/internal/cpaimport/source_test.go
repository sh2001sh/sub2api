package cpaimport

import "testing"

func TestBuildCloneURL_UsesTokenAndUsername(t *testing.T) {
	cfg := Config{
		GitURL:   "https://example.com/org/repo.git",
		GitUser:  "tester",
		GitToken: "secret-token",
	}

	cloneURL, err := buildCloneURL(cfg)
	if err != nil {
		t.Fatalf("buildCloneURL returned error: %v", err)
	}
	if cloneURL != "https://tester:secret-token@example.com/org/repo.git" {
		t.Fatalf("unexpected clone url: %q", cloneURL)
	}
}

func TestBuildCloneURL_DefaultsUsernameToGit(t *testing.T) {
	cfg := Config{
		GitURL:   "https://example.com/org/repo.git",
		GitToken: "secret-token",
	}

	cloneURL, err := buildCloneURL(cfg)
	if err != nil {
		t.Fatalf("buildCloneURL returned error: %v", err)
	}
	if cloneURL != "https://git:secret-token@example.com/org/repo.git" {
		t.Fatalf("unexpected clone url: %q", cloneURL)
	}
}

func TestBuildCloneURL_RejectsNonHTTPTokenAuth(t *testing.T) {
	cfg := Config{
		GitURL:   "ssh://git@example.com/org/repo.git",
		GitToken: "secret-token",
	}

	if _, err := buildCloneURL(cfg); err == nil {
		t.Fatalf("expected non-http token auth to be rejected")
	}
}
