package caller

import (
	"errors"
	"testing"
)

func TestProjectionPolicyAllowsIdentityAndTransitiveDescent(t *testing.T) {
	admin := Define("admin")
	worker := Define("worker")
	publicAPI := Define("public-api")
	policy := DefineProjectionPolicy(
		Descends(admin, worker),
		Descends(worker, publicAPI),
	)

	tests := []struct {
		name      string
		granted   Caller
		requested Caller
		want      Caller
	}{
		{name: "identity", granted: worker, requested: worker, want: worker},
		{name: "unspecified retains granted", granted: worker, want: worker},
		{name: "direct descent", granted: worker, requested: publicAPI, want: publicAPI},
		{name: "transitive descent", granted: admin, requested: publicAPI, want: publicAPI},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := policy.Resolve(test.granted, test.requested)
			if err != nil {
				t.Fatalf("Resolve error = %v", err)
			}
			if got != test.want {
				t.Fatalf("Resolve = %q, want %q", got, test.want)
			}
		})
	}
}

func TestProjectionPolicyRejectsElevationAndLateralProjection(t *testing.T) {
	worker := Define("worker")
	dashboard := Define("dashboard")
	publicAPI := Define("public-api")
	policy := DefineProjectionPolicy(
		Descends(worker, publicAPI),
		Descends(dashboard, publicAPI),
	)

	for _, test := range []struct {
		name      string
		granted   Caller
		requested Caller
	}{
		{name: "elevation", granted: publicAPI, requested: worker},
		{name: "lateral", granted: worker, requested: dashboard},
	} {
		t.Run(test.name, func(t *testing.T) {
			if policy.Allows(test.granted, test.requested) {
				t.Fatal("Allows = true")
			}
			if _, err := policy.Resolve(test.granted, test.requested); !errors.Is(err, ErrProjectionElevation) {
				t.Fatalf("Resolve error = %v, want ErrProjectionElevation", err)
			}
		})
	}
}

func TestProjectionPolicyRejectsUndefinedGrantedCaller(t *testing.T) {
	policy := ProjectionPolicy{}
	if _, err := policy.Resolve(Caller{}, Define("public-api")); !errors.Is(err, ErrProjectionCallerUndefined) {
		t.Fatalf("Resolve error = %v, want ErrProjectionCallerUndefined", err)
	}
}

func TestProjectionPolicyRejectsInvalidRules(t *testing.T) {
	worker := Define("worker")
	publicAPI := Define("public-api")

	tests := []struct {
		name string
		fn   func()
	}{
		{
			name: "self edge",
			fn: func() {
				Descends(worker, worker)
			},
		},
		{
			name: "undefined caller",
			fn: func() {
				Descends(worker, Caller{})
			},
		},
		{
			name: "duplicate edge",
			fn: func() {
				rule := Descends(worker, publicAPI)
				DefineProjectionPolicy(rule, rule)
			},
		},
		{
			name: "cycle",
			fn: func() {
				DefineProjectionPolicy(
					Descends(worker, publicAPI),
					Descends(publicAPI, worker),
				)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("did not panic")
				}
			}()
			test.fn()
		})
	}
}
