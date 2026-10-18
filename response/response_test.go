package response

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRenderJSONSetsResponse(t *testing.T) {
	target := &testTarget{}

	RenderJSON(target, http.StatusCreated, map[string]string{"ok": "true"})

	if target.res == nil {
		t.Fatal("response was not set")
	}
	if target.res.Status != http.StatusCreated {
		t.Fatalf("status = %d, want %d", target.res.Status, http.StatusCreated)
	}
	if target.res.ContentType != ApplicationJson {
		t.Fatalf("content type = %q, want %q", target.res.ContentType, ApplicationJson)
	}
}

func TestConstructorsBuildCommonResponseTypes(t *testing.T) {
	sentAt := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	json := JSON(
		http.StatusCreated,
		map[string]string{"ok": "true"},
		WithHeader("X-Test", "yes"),
		WithSentAt(sentAt),
	)
	if json.Status != http.StatusCreated || json.ContentType != ApplicationJson {
		t.Fatalf("json response = %#v", json)
	}
	if json.SentAt != sentAt {
		t.Fatalf("sent_at = %v, want %v", json.SentAt, sentAt)
	}
	if json.Header.Get("X-Test") != "yes" {
		t.Fatalf("header = %#v", json.Header)
	}

	text := Text(http.StatusAccepted, "accepted")
	if text.ContentType != TextPlain || text.Body != "accepted" {
		t.Fatalf("text response = %#v", text)
	}

	html := HTML(http.StatusOK, "<p>ok</p>")
	if html.ContentType != TextHTML || html.Body != "<p>ok</p>" {
		t.Fatalf("html response = %#v", html)
	}
}

func TestBytesConstructorClonesBodyAndDefaultsContentType(t *testing.T) {
	src := []byte("hello")
	res := Bytes(http.StatusOK, "", src)
	src[0] = 'j'

	if res.ContentType != ApplicationOctetStream {
		t.Fatalf("content type = %q, want %q", res.ContentType, ApplicationOctetStream)
	}
	got, ok := res.Body.([]byte)
	if !ok {
		t.Fatalf("body = %T, want []byte", res.Body)
	}
	if string(got) != "hello" {
		t.Fatalf("body = %q, want hello", got)
	}
}

func TestRedirectResponseSetsLocationAndDefaultStatus(t *testing.T) {
	res := Redirect(0, "https://example.test/next")

	if res.Status != http.StatusFound {
		t.Fatalf("status = %d, want %d", res.Status, http.StatusFound)
	}
	if res.Header.Get("Location") != "https://example.test/next" {
		t.Fatalf("location = %q", res.Header.Get("Location"))
	}
	if res.Body != nil || res.ContentType != "" {
		t.Fatalf("redirect body/content type = %#v/%q", res.Body, res.ContentType)
	}
}

func TestHeaderOptionsCloneInputHeaders(t *testing.T) {
	headers := http.Header{"X-Test": {"yes"}}
	res := JSON(http.StatusOK, map[string]string{"ok": "true"}, WithHeaders(headers))
	headers.Set("X-Test", "changed")

	if !reflect.DeepEqual(res.Header.Values("X-Test"), []string{"yes"}) {
		t.Fatalf("headers = %#v", res.Header)
	}
}

func TestEncodeResponseBodyEncodesText(t *testing.T) {
	body, err := EncodeResponseBody(&Res{
		ContentType: TextPlain,
		Body:        "hello",
	}, Context{})

	if err != nil {
		t.Fatalf("EncodeResponseBody error = %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("body = %q, want hello", body)
	}
}

func TestEncodeResponseBodyEncodesJSON(t *testing.T) {
	body, err := EncodeResponseBody(&Res{
		ContentType: ApplicationJson,
		Body:        map[string]string{"ok": "true"},
	}, Context{})

	if err != nil {
		t.Fatalf("EncodeResponseBody error = %v", err)
	}
	if !strings.Contains(string(body), `"ok":"true"`) {
		t.Fatalf("body = %q, want json body", body)
	}
}

func TestEncodeResponseBodyReturnsRawBytes(t *testing.T) {
	body, err := EncodeResponseBody(Bytes(http.StatusOK, "image/png", []byte{1, 2, 3}), Context{})

	if err != nil {
		t.Fatalf("EncodeResponseBody error = %v", err)
	}
	if !reflect.DeepEqual(body, []byte{1, 2, 3}) {
		t.Fatalf("body = %#v, want raw bytes", body)
	}
}

func TestEncodeResponseBodyKeepsJSONSemanticsForByteSlices(t *testing.T) {
	body, err := EncodeResponseBody(JSON(http.StatusOK, []byte("hello")), Context{})

	if err != nil {
		t.Fatalf("EncodeResponseBody error = %v", err)
	}
	if !strings.Contains(string(body), `"aGVsbG8="`) {
		t.Fatalf("body = %q, want base64 JSON string", body)
	}
}

func TestEncodeResponseBodyWritesRawJSONMessage(t *testing.T) {
	body, err := EncodeResponseBody(JSON(http.StatusOK, json.RawMessage(`{"ok":true}`)), Context{})

	if err != nil {
		t.Fatalf("EncodeResponseBody error = %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("body = %q, want raw JSON", body)
	}
}

func TestRenderConvenienceHelpers(t *testing.T) {
	target := &testTarget{}

	RenderText(target, http.StatusAccepted, "accepted")
	if target.res.ContentType != TextPlain || target.res.Body != "accepted" {
		t.Fatalf("text response = %#v", target.res)
	}

	RenderNoContent(target)
	if target.res.Status != http.StatusNoContent || target.res.Body != nil {
		t.Fatalf("no-content response = %#v", target.res)
	}

	RenderRedirect(target, http.StatusSeeOther, "https://example.test/next")
	if target.res.Status != http.StatusSeeOther || target.res.Header.Get("Location") == "" {
		t.Fatalf("redirect response = %#v", target.res)
	}
}

type testTarget struct {
	res *Res
}

func (t *testTarget) SetResponse(res *Res) {
	t.res = res
}
