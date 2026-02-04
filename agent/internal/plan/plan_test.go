package plan

import "testing"

func TestValidatePlan(t *testing.T) {
	validYAML := []byte(`version: "v1"
health:
  type: probe.http
  url: http://localhost:8080/healthz
  expectStatus: 200
  timeoutSec: 30
  intervalSec: 2
steps:
  - id: copy-config
    type: file.copy
    timeoutSec: 30
    onFail: rollback
    params:
      src: files/config.yaml
      dest: /etc/app/config.yaml
  - id: health
    type: probe.http
    onFail: abort
    params:
      url: http://localhost:8080/healthz
`)

	invalidTypeYAML := []byte(`version: "v1"
steps:
  - id: bad-step
    type: file.delete
    onFail: abort
    params:
      path: /tmp/file
`)

	dedupYAML := []byte(`version: "v1"
steps:
  - id: dup
    type: file.copy
    onFail: abort
    params:
      src: files/a
      dest: /tmp/a
  - id: dup
    type: probe.http
    onFail: abort
    params:
      url: http://localhost:8080/healthz
`)

	missingParamsYAML := []byte(`version: "v1"
steps:
  - id: missing-dest
    type: file.copy
    onFail: abort
    params:
      src: files/a
`)

	invalidHealthYAML := []byte(`version: "v1"
health:
  type: probe.tcp
  url: http://localhost:8080/healthz
steps:
  - id: copy-config
    type: file.copy
    onFail: abort
    params:
      src: files/a
      dest: /tmp/a
`)

	cases := []struct {
		name    string
		yaml    []byte
		wantErr bool
	}{
		{name: "valid", yaml: validYAML, wantErr: false},
		{name: "invalid type", yaml: invalidTypeYAML, wantErr: true},
		{name: "duplicate id", yaml: dedupYAML, wantErr: true},
		{name: "missing params", yaml: missingParamsYAML, wantErr: true},
		{name: "invalid health", yaml: invalidHealthYAML, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Parse(tc.yaml)
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			err = Validate(*p)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
