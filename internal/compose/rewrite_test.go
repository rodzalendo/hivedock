package compose

import (
	"errors"
	"testing"
)

func TestSetImageTagGolden(t *testing.T) {
	cases := []struct {
		name    string
		service string
		newTag  string
		in      string
		out     string // expected exact bytes
	}{
		{
			name:    "plain with surrounding comments preserved",
			service: "web",
			newTag:  "v1.11.0",
			in:      "# my stack\nservices:\n  web:\n    image: traefik/whoami:v1.10.0 # pinned\n    ports:\n      - \"80:80\"\n",
			out:     "# my stack\nservices:\n  web:\n    image: traefik/whoami:v1.11.0 # pinned\n    ports:\n      - \"80:80\"\n",
		},
		{
			name:    "double-quoted value keeps quotes",
			service: "web",
			newTag:  "1.27.2",
			in:      "services:\n  web:\n    image: \"nginx:1.27.0\"\n",
			out:     "services:\n  web:\n    image: \"nginx:1.27.2\"\n",
		},
		{
			name:    "single-quoted value keeps quotes",
			service: "cache",
			newTag:  "7.4.1",
			in:      "services:\n  cache:\n    image: 'redis:7.4.0'\n",
			out:     "services:\n  cache:\n    image: 'redis:7.4.1'\n",
		},
		{
			name:    "untagged image gains a tag",
			service: "cache",
			newTag:  "7.4",
			in:      "services:\n  cache:\n    image: redis\n",
			out:     "services:\n  cache:\n    image: redis:7.4\n",
		},
		{
			name:    "registry with port",
			service: "app",
			newTag:  "1.1.0",
			in:      "services:\n  app:\n    image: localhost:5000/app:1.0.0\n",
			out:     "services:\n  app:\n    image: localhost:5000/app:1.1.0\n",
		},
		{
			name:    "only the target service changes; anchors untouched",
			service: "web",
			newTag:  "1.28.0",
			in:      "x-common: &common\n  restart: unless-stopped\nservices:\n  web:\n    <<: *common\n    image: nginx:1.27.0\n  db:\n    <<: *common\n    image: postgres:16.4\n",
			out:     "x-common: &common\n  restart: unless-stopped\nservices:\n  web:\n    <<: *common\n    image: nginx:1.28.0\n  db:\n    <<: *common\n    image: postgres:16.4\n",
		},
		{
			name:    "CRLF line endings preserved",
			service: "web",
			newTag:  "1.28.0",
			in:      "services:\r\n  web:\r\n    image: nginx:1.27.0\r\n",
			out:     "services:\r\n  web:\r\n    image: nginx:1.28.0\r\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SetImageTag([]byte(tc.in), tc.service, tc.newTag)
			if err != nil {
				t.Fatalf("SetImageTag: %v", err)
			}
			if string(got) != tc.out {
				t.Errorf("mismatch:\n--- got ---\n%q\n--- want ---\n%q", got, tc.out)
			}
		})
	}
}

func TestSetImageTagSurfacesEnvManaged(t *testing.T) {
	in := "services:\n  media:\n    image: jellyfin/jellyfin:${TAG}\n"
	got, err := SetImageTag([]byte(in), "media", "10.9.0")
	if !errors.Is(err, ErrEnvManaged) {
		t.Fatalf("err = %v, want ErrEnvManaged", err)
	}
	if got != nil {
		t.Errorf("env-managed image must not be rewritten; got %q", got)
	}
}

func TestSetImageTagRejectsDigestPinned(t *testing.T) {
	in := "services:\n  web:\n    image: nginx@sha256:abcdef\n"
	if _, err := SetImageTag([]byte(in), "web", "1.27.2"); !errors.Is(err, ErrDigestPinned) {
		t.Fatalf("err = %v, want ErrDigestPinned", err)
	}
}

func TestSetImageTagNotFound(t *testing.T) {
	in := "services:\n  web:\n    image: nginx:1.27.0\n"
	if _, err := SetImageTag([]byte(in), "ghost", "1.28.0"); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("err = %v, want ErrServiceNotFound", err)
	}
}

func TestSetImageTagNoChange(t *testing.T) {
	in := "services:\n  web:\n    image: nginx:1.27.0\n"
	got, err := SetImageTag([]byte(in), "web", "1.27.0")
	if err != nil {
		t.Fatalf("SetImageTag: %v", err)
	}
	if string(got) != in {
		t.Errorf("no-op rewrite changed content:\n%q", got)
	}
}
