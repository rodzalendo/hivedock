package registry

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

type candidateCase struct {
	Name      string   `yaml:"name"`
	Current   string   `yaml:"current"`
	Available []string `yaml:"available"`
	Want      string   `yaml:"want"`
	Diff      string   `yaml:"diff"`
}

type candidateCorpus struct {
	Cases []candidateCase `yaml:"cases"`
}

// TestCandidateCorpus is the contract: every real-world case in
// testdata/candidates.yaml must pass. Add cases there, not new test funcs.
func TestCandidateCorpus(t *testing.T) {
	data, err := os.ReadFile("testdata/candidates.yaml")
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	var corpus candidateCorpus
	if err := yaml.Unmarshal(data, &corpus); err != nil {
		t.Fatalf("parse corpus: %v", err)
	}
	if len(corpus.Cases) < 30 {
		t.Fatalf("corpus has %d cases; the plan calls for 30+", len(corpus.Cases))
	}

	for _, tc := range corpus.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			got, diff, ok := Candidate(tc.Current, tc.Available)

			if tc.Want == "" {
				if ok {
					t.Fatalf("Candidate(%q, %v) = %q (%s), want no candidate",
						tc.Current, tc.Available, got, diff)
				}
				return
			}
			if !ok {
				t.Fatalf("Candidate(%q, %v) = none, want %q", tc.Current, tc.Available, tc.Want)
			}
			if got != tc.Want {
				t.Errorf("candidate = %q, want %q", got, tc.Want)
			}
			if string(diff) != tc.Diff {
				t.Errorf("diff = %q, want %q", diff, tc.Diff)
			}
		})
	}
}
