package stacks

import (
	"testing"

	"github.com/rogalinski/hivedock/internal/docker"
)

func findStack(stacks []Stack, name string) (Stack, bool) {
	for _, s := range stacks {
		if s.Name == name {
			return s, true
		}
	}
	return Stack{}, false
}

func TestMergeClassifiesManagedExternalStandalone(t *testing.T) {
	scanned := []ScannedStack{
		{
			Name: "whoami", Dir: "/opt/stacks/whoami", ComposeFile: "/opt/stacks/whoami/compose.yaml",
			Project:  "whoami",
			Services: map[string]ScannedSvc{"whoami": {Image: "traefik/whoami:v1.10.1"}},
		},
		{
			Name: "redis-app", Dir: "/opt/stacks/redis-app", Project: "redis-app",
			Services: map[string]ScannedSvc{
				"app":   {Image: "traefik/whoami:v1.10.1"},
				"redis": {Image: "redis:7.2-alpine"},
			},
		},
	}
	containers := []docker.Container{
		// managed whoami — running
		{ID: "aaaaaaaaaaaa0000", Name: "whoami-whoami-1", Image: "traefik/whoami:v1.10.1", State: "running", Project: "whoami", Service: "whoami"},
		// managed redis-app — app running, redis stopped -> partial
		{ID: "bbbbbbbbbbbb0000", Name: "redis-app-app-1", Image: "traefik/whoami:v1.10.1", State: "running", Project: "redis-app", Service: "app"},
		{ID: "cccccccccccc0000", Name: "redis-app-redis-1", Image: "redis:7.2-alpine", State: "exited", Project: "redis-app", Service: "redis"},
		// external compose project, no file on disk
		{ID: "dddddddddddd0000", Name: "otherproj-svc-1", Image: "nginx:latest", State: "running", Project: "otherproj", Service: "svc"},
		// standalone plain `docker run` container
		{ID: "eeeeeeeeeeee0000", Name: "lonely", Image: "busybox:latest", State: "running"},
		// one-off `compose run` container -> must be ignored
		{ID: "ffffffffffff0000", Name: "whoami-whoami-run-1", Image: "traefik/whoami", State: "exited", Project: "whoami", Service: "whoami", Oneoff: true},
	}

	stacks := Merge(scanned, containers, true)

	whoami, ok := findStack(stacks, "whoami")
	if !ok || whoami.Origin != OriginManaged || whoami.Status != StatusRunning {
		t.Fatalf("whoami = %+v", whoami)
	}
	if len(whoami.Services) != 1 || whoami.Services[0].ContainerID != "aaaaaaaaaaaa" {
		t.Errorf("whoami services = %+v", whoami.Services)
	}

	redisApp, ok := findStack(stacks, "redis-app")
	if !ok || redisApp.Status != StatusPartial {
		t.Fatalf("redis-app status = %v, want partial (%+v)", redisApp.Status, redisApp)
	}

	ext, ok := findStack(stacks, "otherproj")
	if !ok || ext.Origin != OriginExternal {
		t.Fatalf("otherproj = %+v, want external", ext)
	}

	lonely, ok := findStack(stacks, "lonely")
	if !ok || lonely.Origin != OriginExternal {
		t.Fatalf("lonely = %+v, want external standalone", lonely)
	}
}

func TestMergeDaemonDownShowsManagedOnly(t *testing.T) {
	scanned := []ScannedStack{
		{Name: "whoami", Project: "whoami", Services: map[string]ScannedSvc{"whoami": {Image: "x"}}},
	}
	// daemonOK=false: managed stacks show with unknown status, no external.
	stacks := Merge(scanned, nil, false)
	if len(stacks) != 1 {
		t.Fatalf("expected only managed stack, got %d", len(stacks))
	}
	if stacks[0].Status != StatusUnknown {
		t.Errorf("status = %v, want unknown", stacks[0].Status)
	}
}

func TestMergeSurfacesRunningServiceNotInFile(t *testing.T) {
	// A container running under a managed project but with a service not in the
	// compose file must still appear (drift; the UI never lies).
	scanned := []ScannedStack{
		{Name: "app", Project: "app", Services: map[string]ScannedSvc{"web": {Image: "nginx"}}},
	}
	containers := []docker.Container{
		{ID: "111111111111", Name: "app-web-1", Image: "nginx", State: "running", Project: "app", Service: "web"},
		{ID: "222222222222", Name: "app-ghost-1", Image: "redis", State: "running", Project: "app", Service: "ghost"},
	}
	stacks := Merge(scanned, containers, true)
	app, _ := findStack(stacks, "app")
	if len(app.Services) != 2 {
		t.Fatalf("expected 2 services (web + surfaced ghost), got %d: %+v", len(app.Services), app.Services)
	}
}
