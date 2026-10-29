package seed

import (
	"errors"
	"reflect"
	"testing"

	"github.com/tweinmann/siding/internal/compose"
	"github.com/tweinmann/siding/internal/fail"
)

func project(services map[string]compose.Service) *compose.Project {
	return &compose.Project{Services: services, Volumes: map[string]compose.Resource{
		"data": {Name: "shop-main_data"}, "cache": {Name: "shop-main_cache"}, "seeded": {Name: "shop-main_seeded"},
		"shared": {Name: "shared", External: true},
	}}
}

func TestMake(t *testing.T) {
	t.Parallel()
	p := project(map[string]compose.Service{
		"db":    {Labels: map[string]string{Label: Script, LabelCommand: "psql -f /seed.sql"}, Volumes: []string{"seeded"}},
		"cache": {Labels: map[string]string{Label: None}, Volumes: []string{"cache"}},
		"web":   {Labels: map[string]string{}, Volumes: []string{"data", "shared"}},
	})
	plan, err := Make(p, true)
	if err != nil {
		t.Fatal(err)
	}
	want := Plan{Volumes: []string{"data"}, Scripts: []Run{{Service: "db", Command: "psql -f /seed.sql"}}}
	if !reflect.DeepEqual(plan, want) {
		t.Errorf("plan %+v, want %+v", plan, want)
	}
	// Without a snapshot only the scripts run.
	if plan, _ := Make(p, false); len(plan.Volumes) != 0 || len(plan.Scripts) != 1 {
		t.Errorf("without a snapshot: %+v", plan)
	}
}

func TestMakeInvalid(t *testing.T) {
	t.Parallel()
	for name, labels := range map[string]map[string]string{
		"script without command": {Label: Script},
		"unknown mode":           {Label: "always"},
	} {
		_, err := Make(project(map[string]compose.Service{"db": {Labels: labels}}), true)
		var e *fail.Error
		if !errors.As(err, &e) || e.Code != fail.InvalidArguments {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestFirstUp(t *testing.T) {
	t.Parallel()
	if !FirstUp(0, 0) || FirstUp(1, 0) || FirstUp(0, 1) {
		t.Error("FirstUp")
	}
}
