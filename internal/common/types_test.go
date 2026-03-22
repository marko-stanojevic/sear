package common

import (
	"reflect"
	"testing"
)

func TestFlattenPlaybook(t *testing.T) {
	pb := &Playbook{
		Name: "test",
		Jobs: []Job{
			{
				Name: "job1",
				Steps: []Step{
					{Name: "step1"},
					{Name: "step2"},
				},
			},
			{
				Name: "job2",
				Steps: []Step{
					{Name: "step3"},
				},
			},
		},
	}
	flat := FlattenPlaybook(pb)
	if len(flat) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(flat))
	}
	if flat[0].JobName != "job1" || flat[0].Step.Name != "step1" || flat[0].GlobalIndex != 0 {
		t.Errorf("unexpected first step: %+v", flat[0])
	}
	if flat[2].JobName != "job2" || flat[2].Step.Name != "step3" || flat[2].GlobalIndex != 2 {
		t.Errorf("unexpected last step: %+v", flat[2])
	}
}

func TestResolveSecrets(t *testing.T) {
	secrets := map[string]string{"FOO": "bar", "BAZ": "qux"}
	in := "token=${{ secrets.FOO }};x=${{ secrets.BAZ }};y=${{ secrets.UNKNOWN }}"
	want := "token=bar;x=qux;y="
	if got := ResolveSecrets(in, secrets); got != want {
		t.Errorf("ResolveSecrets() = %q; want %q", got, want)
	}
}

func TestResolveEnvSecrets(t *testing.T) {
	secrets := map[string]string{"FOO": "bar"}
	env := map[string]string{"A": "${{ secrets.FOO }}", "B": "plain"}
	want := map[string]string{"A": "bar", "B": "plain"}
	if got := ResolveEnvSecrets(env, secrets); !reflect.DeepEqual(got, want) {
		t.Errorf("ResolveEnvSecrets() = %+v; want %+v", got, want)
	}
}
