package endpoint

import "testing"

func TestNormalizeOperationKind(t *testing.T) {
	tests := []struct {
		input OperationKind
		want  OperationKind
	}{
		{input: "", want: OperationKindUnspecified},
		{input: " read ", want: OperationKindRead},
		{input: "WRITE", want: OperationKindWrite},
	}

	for _, tt := range tests {
		if got := NormalizeOperationKind(tt.input); got != tt.want {
			t.Fatalf("NormalizeOperationKind(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeOperationKindRejectsUnknownKind(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("NormalizeOperationKind did not panic")
		}
	}()

	NormalizeOperationKind("query")
}

func TestRequiredOperationKindRejectsUnspecifiedKind(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("RequiredOperationKind did not panic")
		}
	}()

	RequiredOperationKind(OperationKindUnspecified)
}

func TestOperationKindIsExplicitAndDoesNotInherit(t *testing.T) {
	read := DefineEndpoint(EndpointSpec{
		Method:  POST,
		Path:    "/tasks/lookup",
		Handler: noopTranscriptionHandler,
		Operation: OperationSpec{
			Kind: OperationKindRead,
		},
	})
	if got := read.OperationKind(); got != OperationKindRead {
		t.Fatalf("operation kind = %q, want %q", got, OperationKindRead)
	}

	resolved := OperationSpec{}.WithDefaults(OperationSpec{Kind: OperationKindWrite})
	if resolved.Kind != OperationKindUnspecified {
		t.Fatalf("inherited operation kind = %q, want unspecified", resolved.Kind)
	}

	group := EndpointGroup{
		Operation: OperationSpec{Kind: OperationKindWrite},
		Endpoints: []Endpoint{
			read,
			DefineEndpoint(EndpointSpec{
				Method:  POST,
				Path:    "/tasks/unspecified",
				Handler: noopTranscriptionHandler,
			}),
		},
	}
	endpoints := group.ResolvedEndpoints()
	if got := endpoints[0].OperationKind(); got != OperationKindRead {
		t.Fatalf("explicit group endpoint kind = %q, want %q", got, OperationKindRead)
	}
	if got := endpoints[1].OperationKind(); got != OperationKindUnspecified {
		t.Fatalf("group-inherited operation kind = %q, want unspecified", got)
	}
}
