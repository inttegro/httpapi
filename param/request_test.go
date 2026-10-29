package param

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	callerpkg "github.com/inttegro/httpapi/caller"
)

var (
	publicCaller    = callerpkg.Define("public-api")
	workerCaller    = callerpkg.Define("worker")
	dashboardCaller = callerpkg.Define("dashboard")
	adminCaller     = callerpkg.Define("admin")
)

type lineItemParams struct {
	Name string `json:"name"`
}

type customerDataParams struct {
	Name  string
	Email string
	Phone string
}

type createdFromParams struct {
	Source string
}

type orderParams struct {
	LineItems    []lineItemParams
	CustomerID   string
	CustomerData *customerDataParams
	CreatedFrom  *createdFromParams
	Note         any
}

type requiredOptionalParams struct {
	Name     string
	Quantity int
	HasQty   bool
}

type parsedCustomerID string

type parserDefaultsParams struct {
	ID          string
	Tags        []string
	ExpiresAt   time.Time
	SubmittedAt *time.Time
}

type itemObjectParams struct {
	ID   string
	Name string
}

type itemArrayParams struct {
	Items []itemObjectParams
}

type enumParams struct {
	Status string
}

type discriminatedParams struct {
	Item discriminatedItemParams
}

type discriminatedItemParams struct {
	Type         string
	Name         string
	Amount       int
	InternalNote string
}

func TestRequestParseRequiredAndOptionalParameters(t *testing.T) {
	request := JSON[requiredOptionalParams]().
		Param(Required("name", String()).MinSize(2).MaxSize(20)).
		Param(Optional("quantity", Int()).MinSize(1).MaxSize(10)).
		Parse(func(values Values) (requiredOptionalParams, error) {
			quantity, hasQty := Get[int](values, "quantity")
			return requiredOptionalParams{
				Name:     Must[string](values, "name"),
				Quantity: quantity,
				HasQty:   hasQty,
			}, nil
		})

	got, err := request.Parse(`{"name":"Ada"}`)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	if got.Name != "Ada" || got.HasQty {
		t.Fatalf("parsed params = %#v", got)
	}

	got, err = request.Parse(`{"name":"Ada","quantity":3}`)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	if got.Quantity != 3 || !got.HasQty {
		t.Fatalf("parsed params = %#v", got)
	}

	_, err = request.Parse(`{"quantity":3}`)
	if err == nil {
		t.Fatal("Parse error = nil")
	}
	if err.Code != CodeMissing || err.Param != "name" {
		t.Fatalf("error = %#v", err)
	}
}

func TestRequestParseRejectsBlankRequiredString(t *testing.T) {
	request := JSON[string]().
		Param(Required("name", String())).
		Parse(func(values Values) (string, error) {
			return Must[string](values, "name"), nil
		})

	_, err := request.Parse(`{"name":" \t "}`)
	if err == nil {
		t.Fatal("Parse error = nil")
	}
	if err.Code != CodeMissing || err.Param != "name" {
		t.Fatalf("error = %#v", err)
	}
}

func TestRequestParseStringEnum(t *testing.T) {
	request := JSON[enumParams]().
		Param(Required("status", Enum("draft", "active"))).
		Parse(func(values Values) (enumParams, error) {
			return enumParams{Status: Must[string](values, "status")}, nil
		})

	got, err := request.Parse(`{"status":"active"}`)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	if got.Status != "active" {
		t.Fatalf("status = %q, want active", got.Status)
	}

	_, err = request.Parse(`{"status":"archived"}`)
	if err == nil {
		t.Fatal("Parse error = nil")
	}
	if err.Code != CodeValueNotAllowed || err.Param != "status" {
		t.Fatalf("error = %#v", err)
	}
	if !strings.Contains(err.Message, "`draft` or `active`") ||
		!strings.Contains(err.Message, `"archived"`) {
		t.Fatalf("error message = %q", err.Message)
	}

	spec := request.Describe()
	status := findParameterSpec(t, spec.Body, "status")
	if !reflect.DeepEqual(status.Shape.Enum, []string{"draft", "active"}) {
		t.Fatalf("status enum = %#v", status.Shape.Enum)
	}
}

func TestRequestParseTrimmedStringEnum(t *testing.T) {
	request := JSON[enumParams]().
		Param(Required("status", TrimmedEnum("draft", "active"))).
		Parse(func(values Values) (enumParams, error) {
			return enumParams{Status: Must[string](values, "status")}, nil
		})

	got, err := request.Parse(`{"status":" active "}`)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	if got.Status != "active" {
		t.Fatalf("status = %q, want active", got.Status)
	}

	_, err = request.Parse(`{"status":" archived "}`)
	if err == nil {
		t.Fatal("Parse error = nil")
	}
	if err.Code != CodeValueNotAllowed || err.Param != "status" {
		t.Fatalf("error = %#v", err)
	}

	spec := request.Describe()
	status := findParameterSpec(t, spec.Body, "status")
	if !reflect.DeepEqual(status.Shape.Enum, []string{"draft", "active"}) {
		t.Fatalf("status enum = %#v", status.Shape.Enum)
	}
}

func TestParameterParserCanChangeShapeType(t *testing.T) {
	request := JSON[parsedCustomerID]().
		Param(Required("customer_id", String()).
			Parse(func(raw string) (parsedCustomerID, error) {
				return parsedCustomerID(strings.TrimSpace(raw)), nil
			})).
		Parse(func(values Values) (parsedCustomerID, error) {
			return Must[parsedCustomerID](values, "customer_id"), nil
		})

	got, err := request.Parse(`{"customer_id":" cus_123 "}`)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	if got != "cus_123" {
		t.Fatalf("customer ID = %q", got)
	}
}

func TestRequestParseDiscriminatedObject(t *testing.T) {
	request := discriminatedRequestParser()

	got, err := request.Parse(`{"item":{"type":"product","name":"Book"}}`)
	if err != nil {
		t.Fatalf("Parse product error = %v", err)
	}
	if got.Item.Type != "product" || got.Item.Name != "Book" {
		t.Fatalf("product item = %#v", got.Item)
	}

	got, err = request.Parse(`{"item":{"type":"fee","amount":120}}`)
	if err != nil {
		t.Fatalf("Parse fee error = %v", err)
	}
	if got.Item.Type != "fee" || got.Item.Amount != 120 {
		t.Fatalf("fee item = %#v", got.Item)
	}
}

func TestRequestParseDiscriminatedObjectRejectsStructuralFailures(t *testing.T) {
	request := discriminatedRequestParser()

	tests := []struct {
		name      string
		body      string
		wantCode  ErrorCode
		wantParam string
	}{
		{
			name:      "missing discriminator",
			body:      `{"item":{"name":"Book"}}`,
			wantCode:  CodeMissing,
			wantParam: "item.type",
		},
		{
			name:      "wrong discriminator type",
			body:      `{"item":{"type":12,"name":"Book"}}`,
			wantCode:  CodeTypeMismatch,
			wantParam: "item.type",
		},
		{
			name:      "unknown discriminator",
			body:      `{"item":{"type":"discount","amount":10}}`,
			wantCode:  CodeValueNotAllowed,
			wantParam: "item.type",
		},
		{
			name:      "unknown branch parameter",
			body:      `{"item":{"type":"product","name":"Book","amount":10}}`,
			wantCode:  CodeUnexpected,
			wantParam: "item.amount",
		},
		{
			name:      "branch required parameter",
			body:      `{"item":{"type":"fee"}}`,
			wantCode:  CodeMissing,
			wantParam: "item.amount",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := request.Parse(tc.body)
			if err == nil {
				t.Fatal("Parse error = nil")
			}
			if err.Code != tc.wantCode || err.Param != tc.wantParam {
				t.Fatalf("error = %#v, want code=%q param=%q", err, tc.wantCode, tc.wantParam)
			}
		})
	}
}

func TestRequestParseDiscriminatedObjectPreservesBranchAvailability(t *testing.T) {
	request := discriminatedRequestParser()

	_, err := request.Parse(
		`{"item":{"type":"product","name":"Book","internal_note":"staff"}}`,
		WithCaller(publicCaller),
	)
	if err == nil {
		t.Fatal("Parse error = nil")
	}
	if err.Code != CodeUnexpected || err.Param != "item.internal_note" {
		t.Fatalf("public error = %#v", err)
	}

	got, err := request.Parse(
		`{"item":{"type":"product","name":"Book","internal_note":"staff"}}`,
		WithCaller(workerCaller),
	)
	if err != nil {
		t.Fatalf("worker parse error = %v", err)
	}
	if got.Item.InternalNote != "staff" {
		t.Fatalf("internal note = %q", got.Item.InternalNote)
	}
}

func TestRequestDescribeDiscriminatedObject(t *testing.T) {
	spec := discriminatedRequestParser().Describe()
	item := findParameterSpec(t, spec.Body, "item")
	if item.Shape.Discriminator == nil {
		t.Fatalf("item discriminator missing: %#v", item.Shape)
	}
	if item.Shape.Discriminator.Parameter != "type" {
		t.Fatalf("discriminator parameter = %q", item.Shape.Discriminator.Parameter)
	}
	if len(item.Shape.Discriminator.Variants) != 2 {
		t.Fatalf("variants = %#v", item.Shape.Discriminator.Variants)
	}
	if item.Shape.Discriminator.Variants[0].Value != "product" ||
		item.Shape.Discriminator.Variants[1].Value != "fee" {
		t.Fatalf("variant values = %#v", item.Shape.Discriminator.Variants)
	}

	productShape := item.Shape.Discriminator.Variants[0].Shape
	name := findParameterSpec(t, productShape, "name")
	if name.Shape.Type != TypeString {
		t.Fatalf("product name shape = %#v", name.Shape)
	}
}

func TestRequestParseRootDiscriminatedObject(t *testing.T) {
	request := JSONShape(
		DiscriminatedObject[discriminatedItemParams]("type").
			Variant("product",
				Object[discriminatedItemParams]().
					Param(Required("name", String())).
					Parse(parseDiscriminatedProduct),
			).
			Variant("fee",
				Object[discriminatedItemParams]().
					Param(Required("amount", Int())).
					Parse(parseDiscriminatedFee),
			),
	)

	got, err := request.Parse(`{"type":"product","name":"Book"}`)
	if err != nil {
		t.Fatalf("Parse product error = %v", err)
	}
	if got.Type != "product" || got.Name != "Book" {
		t.Fatalf("product = %#v", got)
	}

	_, err = request.Parse(`{"type":"product","name":"Book","amount":120}`)
	if err == nil {
		t.Fatal("Parse unexpected field error = nil")
	}
	if err.Code != CodeUnexpected || err.Param != "amount" {
		t.Fatalf("error = %#v, want unexpected amount", err)
	}

	spec := request.Describe()
	if spec.Body.Discriminator == nil {
		t.Fatalf("root discriminator missing: %#v", spec.Body)
	}
	if spec.Body.Discriminator.Parameter != "type" {
		t.Fatalf("discriminator parameter = %q", spec.Body.Discriminator.Parameter)
	}
}

func TestDefaultParsersCleanCommonHTTPInput(t *testing.T) {
	request := JSON[parserDefaultsParams]().
		Param(Required("id", String()).Parse(NonEmptyTrimmedString)).
		Param(Optional("tags", Array[string]()).Parse(TrimmedStringList)).
		Param(Optional("expires_at", String()).Parse(OptionalRFC3339Timestamp)).
		Param(Optional("submitted_at", String()).Parse(OptionalRFC3339TimestampPointer)).
		Parse(func(values Values) (parserDefaultsParams, error) {
			tags, _ := Get[[]string](values, "tags")
			expiresAt, _ := Get[time.Time](values, "expires_at")
			submittedAt, _ := Get[*time.Time](values, "submitted_at")
			return parserDefaultsParams{
				ID:          Must[string](values, "id"),
				Tags:        tags,
				ExpiresAt:   expiresAt,
				SubmittedAt: submittedAt,
			}, nil
		})

	got, err := request.Parse(`{
		"id": " file_123 ",
		"tags": [" image ", " ", "receipt"],
		"expires_at": " 2026-06-21T12:34:56Z ",
		"submitted_at": " 2026-06-22T12:34:56Z "
	}`)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	if got.ID != "file_123" {
		t.Fatalf("ID = %q", got.ID)
	}
	if !reflect.DeepEqual(got.Tags, []string{"image", "receipt"}) {
		t.Fatalf("tags = %#v", got.Tags)
	}
	wantExpiresAt, parseErr := time.Parse(time.RFC3339, "2026-06-21T12:34:56Z")
	if parseErr != nil {
		t.Fatalf("parse expected timestamp: %v", parseErr)
	}
	if !got.ExpiresAt.Equal(wantExpiresAt) {
		t.Fatalf("ExpiresAt = %v", got.ExpiresAt)
	}
	wantSubmittedAt, parseErr := time.Parse(time.RFC3339, "2026-06-22T12:34:56Z")
	if parseErr != nil {
		t.Fatalf("parse expected submitted timestamp: %v", parseErr)
	}
	if got.SubmittedAt == nil || !got.SubmittedAt.Equal(wantSubmittedAt) {
		t.Fatalf("SubmittedAt = %v", got.SubmittedAt)
	}

	got, err = request.Parse(`{"id":"file_123","submitted_at":" "}`)
	if err != nil {
		t.Fatalf("Parse blank optional pointer error = %v", err)
	}
	if got.SubmittedAt != nil {
		t.Fatalf("SubmittedAt = %v, want nil", got.SubmittedAt)
	}

	_, err = request.Parse(`{"id":"   "}`)
	if err == nil {
		t.Fatal("Parse error = nil")
	}
	if err.Code != CodeMissing || err.Param != "id" || err.Message != "`id` is required" {
		t.Fatalf("error = %#v", err)
	}

	_, err = request.Parse(`{"id":"file_123","expires_at":"tomorrow"}`)
	if err == nil {
		t.Fatal("Parse error = nil")
	}
	if err.Code != CodeInvalid || err.Param != "expires_at" ||
		err.Message != "`expires_at` must be an RFC3339 timestamp" {
		t.Fatalf("error = %#v", err)
	}
}

func TestRequestParseReturnsDomainValue(t *testing.T) {
	request := orderRequestParser()

	got, err := request.Parse(strings.NewReader(`{
		"line_items": [{"name": "Book"}],
		"customer_id": "cus_123"
	}`))
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}

	if len(got.LineItems) != 1 || got.LineItems[0].Name != "Book" {
		t.Fatalf("unexpected line items: %#v", got.LineItems)
	}
	if got.CustomerID != "cus_123" {
		t.Fatalf("CustomerID = %q", got.CustomerID)
	}
	if got.CustomerData != nil {
		t.Fatalf("CustomerData = %#v", got.CustomerData)
	}
}

func TestRequestParseNestedObjectShape(t *testing.T) {
	request := orderRequestParser()

	got, err := request.Parse([]byte(`{
		"line_items": [{"name": "Book"}],
		"customer_data": {
			"name": "Ada",
			"email_address": "ada@example.com"
		}
	}`))
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}

	if got.CustomerData == nil {
		t.Fatal("CustomerData = nil")
	}
	if got.CustomerData.Name != "Ada" || got.CustomerData.Email != "ada@example.com" {
		t.Fatalf("CustomerData = %#v", got.CustomerData)
	}
}

func TestRequestParseArrayOfObjectShape(t *testing.T) {
	request := JSON[itemArrayParams]().
		Param(Required("items", ArrayOf(
			Object[itemObjectParams]().
				Param(Required("id", String()).Parse(NonEmptyTrimmedString)).
				Param(Optional("name", String()).Parse(TrimmedString)).
				Parse(func(values Values) (itemObjectParams, error) {
					name, _ := Get[string](values, "name")
					return itemObjectParams{
						ID:   Must[string](values, "id"),
						Name: name,
					}, nil
				}),
		)).MinItems(1)).
		Parse(func(values Values) (itemArrayParams, error) {
			return itemArrayParams{Items: Must[[]itemObjectParams](values, "items")}, nil
		})

	got, err := request.Parse(`{"items":[{"id":" item_123 ","name":" First "}]}`)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].ID != "item_123" || got.Items[0].Name != "First" {
		t.Fatalf("items = %#v", got.Items)
	}

	_, err = request.Parse(`{"items":[{"name":"First"}]}`)
	if err == nil {
		t.Fatal("Parse error = nil")
	}
	if err.Code != CodeMissing || err.Param != "items[0].id" {
		t.Fatalf("error = %#v", err)
	}

	_, err = request.Parse(`{"items":[{"id":"item_123","extra":true}]}`)
	if err == nil {
		t.Fatal("Parse error = nil")
	}
	if err.Code != CodeUnexpected || err.Param != "items[0].extra" {
		t.Fatalf("error = %#v", err)
	}
}

func TestRequestParseOneOfGroup(t *testing.T) {
	request := orderRequestParser()

	_, err := request.Parse(`{
		"line_items": [{"name": "Book"}]
	}`)
	if err == nil {
		t.Fatal("Parse missing customer error = nil")
	}
	if err.Code != CodeRequiredChoice {
		t.Fatalf("missing customer error code = %q", err.Code)
	}
	if err.Param != "customer_id|customer_data" {
		t.Fatalf("missing customer error param = %q", err.Param)
	}

	_, err = request.Parse(`{
		"line_items": [{"name": "Book"}],
		"customer_id": "cus_123",
		"customer_data": {"name": "Ada", "email_address": "ada@example.com"}
	}`)
	if err == nil {
		t.Fatal("Parse error = nil")
	}
	if err.Code != CodeMutuallyExclusive {
		t.Fatalf("error code = %q", err.Code)
	}
	if err.Param != "customer_id|customer_data" {
		t.Fatalf("error param = %q", err.Param)
	}
}

func TestRequestParseRequiredMutuallyExclusiveGroup(t *testing.T) {
	request := JSON[requiredOptionalParams]().
		Param(MutuallyExclusive(
			Optional("name", String()),
			Optional("quantity", Int()),
		).Required()).
		Parse(func(values Values) (requiredOptionalParams, error) {
			quantity, hasQty := Get[int](values, "quantity")
			return requiredOptionalParams{
				Name:     Must[string](values, "name"),
				Quantity: quantity,
				HasQty:   hasQty,
			}, nil
		})

	if _, err := request.Parse(`{"name":"Ada"}`); err != nil {
		t.Fatalf("Parse name-only error = %v", err)
	}

	_, err := request.Parse(`{}`)
	if err == nil {
		t.Fatal("Parse missing choice error = nil")
	}
	if err.Code != CodeRequiredChoice || err.Param != "name|quantity" {
		t.Fatalf("missing choice error = %#v", err)
	}

	_, err = request.Parse(`{"name":"Ada","quantity":3}`)
	if err == nil {
		t.Fatal("Parse conflicting choice error = nil")
	}
	if err.Code != CodeMutuallyExclusive || err.Param != "name|quantity" {
		t.Fatalf("conflicting choice error = %#v", err)
	}
}

func TestRequestDescribeInlineParameterGroups(t *testing.T) {
	spec := orderRequestParser().Describe()

	customerRule := findRuleSpec(
		t,
		spec.Body,
		"customer_id",
		"customer_data",
	)
	if customerRule.MinPresent != 1 || customerRule.MaxPresent != 1 {
		t.Fatalf("customer rule = %#v, want exactly one", customerRule)
	}

	customerData := findParameterSpec(t, spec.Body, "customer_data")
	contactRule := findRuleSpec(
		t,
		customerData.Shape,
		"email_address",
		"phone_number",
	)
	if contactRule.MinPresent != 1 || contactRule.MaxPresent != 0 {
		t.Fatalf("contact rule = %#v, want at least one", contactRule)
	}
}

func TestRequestParseAtLeastOneInsideNestedObject(t *testing.T) {
	request := orderRequestParser()

	_, err := request.Parse(`{
		"line_items": [{"name": "Book"}],
		"customer_data": {"name": "Ada"}
	}`)
	if err == nil {
		t.Fatal("Parse error = nil")
	}
	if err.Code != CodeRequiredChoice {
		t.Fatalf("error code = %q", err.Code)
	}
	if err.Param != "customer_data.email_address|customer_data.phone_number" {
		t.Fatalf("error param = %q", err.Param)
	}
}

func TestRequestParseRestrictedParamLooksUnexpected(t *testing.T) {
	request := orderRequestParser()

	_, err := request.Parse(`{
		"line_items": [{"name": "Book"}],
		"customer_id": "cus_123",
		"created_from": {"source": "dashboard"}
	}`, WithCaller(publicCaller))
	if err == nil {
		t.Fatal("Parse error = nil")
	}
	if err.Code != CodeUnexpected {
		t.Fatalf("error code = %q", err.Code)
	}
	if err.Message != "`created_from` is an unexpected parameter" {
		t.Fatalf("error message = %q", err.Message)
	}
}

func TestRequestParseRestrictedParamAllowedForCaller(t *testing.T) {
	request := orderRequestParser()

	got, err := request.Parse(`{
		"line_items": [{"name": "Book"}],
		"customer_id": "cus_123",
		"created_from": {"source": "dashboard"}
	}`, WithCaller(workerCaller))
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	if got.CreatedFrom == nil || got.CreatedFrom.Source != "dashboard" {
		t.Fatalf("CreatedFrom = %#v", got.CreatedFrom)
	}
}

func TestRequestParseCanReadCallerFromSource(t *testing.T) {
	request := JSON[string]().
		Param(Required("id", String())).
		Param(Optional("internal_note", String()).AvailableTo(workerCaller)).
		Parse(func(values Values) (string, error) {
			note, _ := Get[string](values, "internal_note")
			return note, nil
		})

	got, err := request.Parse(
		`{"id":"rec_123","internal_note":"ready"}`,
		WithRequestCaller(testCallerSource{caller: workerCaller}),
	)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	if got != "ready" {
		t.Fatalf("internal note = %q, want ready", got)
	}
}

func TestRequestParseReadsRequestSource(t *testing.T) {
	ctx := context.WithValue(context.Background(), testContextKey{}, "request-context")
	source := &testRequestSource{
		body:   []byte(`{"id":"rec_123","internal_note":"ready"}`),
		caller: workerCaller,
		ctx:    ctx,
	}

	request := JSON[string]().
		Param(Required("id", String())).
		Param(Optional("internal_note", String()).AvailableTo(workerCaller)).
		Parse(func(values Values) (string, error) {
			if values.Context().Value(testContextKey{}) != "request-context" {
				t.Fatalf("parse context was not sourced from request")
			}
			gotSource, ok := RequestFromValues[*testRequestSource](values)
			if !ok || gotSource != source {
				t.Fatalf("request source = %#v, %t", gotSource, ok)
			}
			note, _ := Get[string](values, "internal_note")
			return note, nil
		})

	got, err := request.Parse(source)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	if got != "ready" {
		t.Fatalf("internal note = %q, want ready", got)
	}
}

func TestRequestParseNullPolicy(t *testing.T) {
	request := orderRequestParser()

	got, err := request.Parse(`{
		"line_items": [{"name": "Book"}],
		"customer_id": null,
		"customer_data": {
			"name": "Ada",
			"email_address": "ada@example.com"
		}
	}`)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	if got.CustomerID != "" || got.CustomerData == nil {
		t.Fatalf("default null-as-absent parse = %#v", got)
	}

	_, err = request.Parse(`{
		"line_items": null,
		"customer_id": "cus_123"
	}`)
	if err == nil {
		t.Fatal("Parse error = nil")
	}
	if err.Code != CodeNullRejected {
		t.Fatalf("error code = %q", err.Code)
	}

	got, err = request.Parse(`{
		"line_items": [{"name": "Book"}],
		"customer_id": "cus_123",
		"note": null
	}`)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	if got.Note != nil {
		t.Fatalf("Note = %#v", got.Note)
	}
}

func TestRequestHasNoParseJSONMethod(t *testing.T) {
	request := orderRequestParser()
	if _, ok := reflect.TypeOf(request).MethodByName("ParseJSON"); ok {
		t.Fatal("Request exposes ParseJSON")
	}
}

func TestRequestParsePreservesParamErrorFromParser(t *testing.T) {
	request := JSON[string]().
		Param(Required("name", String()).
			Parse(func(string) (string, error) {
				return "", Invalid("name", "bad name")
			})).
		Parse(func(values Values) (string, error) {
			return Must[string](values, "name"), nil
		})

	_, err := request.Parse(`{"name":"Ada"}`)
	if err == nil {
		t.Fatal("Parse error = nil")
	}
	if err.Code != CodeInvalid || err.Param != "name" {
		t.Fatalf("error = %#v", err)
	}
}

func TestRequestParseWrapsGenericParserError(t *testing.T) {
	sentinel := errors.New("boom")
	request := JSON[string]().
		Param(Required("name", String()).
			Parse(func(string) (string, error) {
				return "", sentinel
			})).
		Parse(func(values Values) (string, error) {
			return Must[string](values, "name"), nil
		})

	_, err := request.Parse(`{"name":"Ada"}`)
	if err == nil {
		t.Fatal("Parse error = nil")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped sentinel, got %v", err)
	}
	if err.Code != CodeParseFailed {
		t.Fatalf("error code = %q", err.Code)
	}
}

func TestRequestParseRejectsUnknownAndMalformedBodies(t *testing.T) {
	request := orderRequestParser()

	_, err := request.Parse(`{"line_items":[{"name":"Book"}],"customer_id":"cus_123","extra":true}`)
	if err == nil {
		t.Fatal("Parse error = nil")
	}
	if err.Code != CodeUnexpected || err.Param != "extra" {
		t.Fatalf("error = %#v", err)
	}

	_, err = request.Parse(io.NopCloser(strings.NewReader(`[]`)))
	if err == nil {
		t.Fatal("Parse error = nil")
	}
	if err.Code != CodeInvalidBody {
		t.Fatalf("error code = %q", err.Code)
	}
}

func TestRequestParseReportsTypeMismatchBeforeSize(t *testing.T) {
	request := JSON[string]().
		Param(Required("name", String()).MinSize(3)).
		Parse(func(values Values) (string, error) {
			return Must[string](values, "name"), nil
		})

	_, err := request.Parse(`{"name":[]}`)
	if err == nil {
		t.Fatal("Parse error = nil")
	}
	if err.Code != CodeTypeMismatch {
		t.Fatalf("error code = %q", err.Code)
	}
}

func TestParameterDefinitionPanicsForContradictoryBounds(t *testing.T) {
	tests := []struct {
		name string
		fn   func()
	}{
		{
			name: "size",
			fn: func() {
				Required("name", String()).MinSize(10).MaxSize(1)
			},
		},
		{
			name: "items",
			fn: func() {
				Required("items", Array[string]()).MaxItems(1).MinItems(2)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected panic")
				}
			}()
			tc.fn()
		})
	}
}

func discriminatedRequestParser() *Request[discriminatedParams] {
	return JSON[discriminatedParams]().
		Param(Required("item",
			DiscriminatedObject[discriminatedItemParams]("type").
				Variant("product",
					Object[discriminatedItemParams]().
						Param(Required("name", String())).
						Param(Optional("internal_note", String()).AvailableTo(workerCaller)).
						Parse(parseDiscriminatedProduct),
				).
				Variant("fee",
					Object[discriminatedItemParams]().
						Param(Required("amount", Int())).
						Parse(parseDiscriminatedFee),
				),
		)).
		Parse(func(values Values) (discriminatedParams, error) {
			return discriminatedParams{
				Item: Must[discriminatedItemParams](values, "item"),
			}, nil
		})
}

func parseDiscriminatedProduct(values Values) (discriminatedItemParams, error) {
	note, _ := Get[string](values, "internal_note")
	return discriminatedItemParams{
		Type:         "product",
		Name:         Must[string](values, "name"),
		InternalNote: note,
	}, nil
}

func parseDiscriminatedFee(values Values) (discriminatedItemParams, error) {
	return discriminatedItemParams{
		Type:   "fee",
		Amount: Must[int](values, "amount"),
	}, nil
}

func orderRequestParser() *Request[orderParams] {
	return JSON[orderParams]().
		Param(Required("line_items", Array[lineItemParams]()).
			Null(NullRejected).
			MinItems(1).
			Parse(parseLineItems)).
		Param(OneOf(
			Optional("customer_id", String()).
				Parse(parseCustomerID),
			Optional("customer_data",
				Object[customerDataParams]().
					Param(Required("name", String())).
					Param(AtLeastOneOf(
						Optional("email_address", String()),
						Optional("phone_number", String()),
					)).
					Parse(parseCustomerData),
			),
		)).
		Param(Optional("created_from",
			Object[createdFromParams]().
				Param(Optional("source", String())).
				Parse(parseCreatedFrom),
		).AvailableTo(workerCaller, dashboardCaller, adminCaller)).
		Param(Optional("note", Any()).
			Null(NullAccepted)).
		Parse(parseInitiateOrder)
}

func parseLineItems(items []lineItemParams) ([]lineItemParams, error) {
	return items, nil
}

func parseCustomerID(customerID string) (string, error) {
	return strings.TrimSpace(customerID), nil
}

func parseCustomerData(values Values) (customerDataParams, error) {
	name := Must[string](values, "name")
	email, _ := Get[string](values, "email_address")
	phone, _ := Get[string](values, "phone_number")
	return customerDataParams{Name: name, Email: email, Phone: phone}, nil
}

func parseCreatedFrom(values Values) (createdFromParams, error) {
	source, _ := Get[string](values, "source")
	return createdFromParams{Source: source}, nil
}

func parseInitiateOrder(values Values) (orderParams, error) {
	lineItems := Must[[]lineItemParams](values, "line_items")
	customerID, _ := Get[string](values, "customer_id")
	customerData, _ := Get[customerDataParams](values, "customer_data")
	createdFrom, _ := Get[createdFromParams](values, "created_from")
	note, _ := Get[any](values, "note")

	params := orderParams{
		LineItems:  lineItems,
		CustomerID: customerID,
		Note:       note,
	}
	if values.Present("customer_data") {
		params.CustomerData = &customerData
	}
	if values.Present("created_from") {
		params.CreatedFrom = &createdFrom
	}
	return params, nil
}

func findParameterSpec(t *testing.T, spec ShapeSpec, name string) ParameterSpec {
	t.Helper()

	for _, parameter := range spec.Parameters {
		if parameter.Name == name {
			return parameter
		}
	}

	t.Fatalf("parameter %q not found in %#v", name, spec.Parameters)
	return ParameterSpec{}
}

func findRuleSpec(t *testing.T, spec ShapeSpec, names ...string) RuleSpec {
	t.Helper()

	for _, rule := range spec.Rules {
		if reflect.DeepEqual(rule.Names, names) {
			return rule
		}
	}

	t.Fatalf("rule %v not found in %#v", names, spec.Rules)
	return RuleSpec{}
}

type testCallerSource struct {
	caller callerpkg.Caller
}

func (source testCallerSource) RequestCaller() callerpkg.Caller {
	return source.caller
}

type testContextKey struct{}

type testRequestSource struct {
	body   []byte
	caller callerpkg.Caller
	ctx    context.Context
}

func (source *testRequestSource) RequestCaller() callerpkg.Caller {
	if source == nil {
		return callerpkg.Caller{}
	}
	return source.caller
}

func (source *testRequestSource) Context() context.Context {
	if source == nil || source.ctx == nil {
		return context.Background()
	}
	return source.ctx
}

func (source *testRequestSource) RequestBody() []byte {
	if source == nil {
		return nil
	}
	return source.body
}
