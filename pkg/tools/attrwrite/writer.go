// Package attrwrite encapsulates the "convert Markdown to HTML for
// RICH_TEXT attribute values" decision for chip's write tools. Any tool
// that submits attribute values to Collibra (e.g. create_asset, edit_asset)
// instantiates a Writer per request and calls PrepareValue before sending
// each value.
//
// The Writer also caches per-attribute-type metadata for the lifetime of
// one tool call, so multiple writes to the same attribute type within a
// single request share one /attributeTypes/{id} fetch.
package attrwrite

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/markdown"
)

// Writer prepares attribute values for submission to Collibra. Scope a
// Writer to a single tool invocation: the cache is request-local and not
// safe for concurrent use.
type Writer struct {
	client *http.Client
	cache  map[string]*clients.PrepareCreateAttributeTypeFull
}

// New returns a Writer that uses client for /attributeTypes/{id} lookups.
// Pass the same client used for the tool's other Collibra calls.
func New(client *http.Client) *Writer {
	return &Writer{
		client: client,
		cache:  map[string]*clients.PrepareCreateAttributeTypeFull{},
	}
}

// PrepareValue returns the value that should be submitted to Collibra for
// an attribute write. If the attribute's stringType is RICH_TEXT, the
// input is converted from Markdown to HTML so it renders correctly in the
// Collibra UI. Other kinds pass through unchanged.
//
// converted reports whether MD→HTML conversion happened, so callers can
// surface this on their per-operation result.
//
// attributeTypeID and kind come from the asset type's scoped assignment.
// An empty attributeTypeID or a non-string kind short-circuits without
// any HTTP traffic.
//
// If the /attributeTypes/{id} fetch fails (transient HTTP error, missing
// permission, etc.), the original value is returned unchanged so the
// agent's data is never silently dropped. The downstream write may then
// fail with a Collibra error, which is the right place to surface the
// problem.
func (w *Writer) PrepareValue(ctx context.Context, attributeTypeID, kind, value string) (written string, converted bool) {
	if attributeTypeID == "" || !markdown.IsStringKind(kind) {
		return value, false
	}
	details, cached := w.cache[attributeTypeID]
	if !cached {
		fetched, err := clients.GetAttributeTypeFull(ctx, w.client, attributeTypeID)
		if err != nil {
			// Cache the failure as a nil entry so a sibling op on the
			// same type doesn't re-try the same failing fetch.
			w.cache[attributeTypeID] = nil
			return value, false
		}
		w.cache[attributeTypeID] = fetched
		details = fetched
	}
	if details == nil || !markdown.IsRichTextStringType(details.StringType) {
		return value, false
	}
	return markdown.ToHTML(value), true
}
