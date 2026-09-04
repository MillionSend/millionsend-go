package millionsend

import (
	"context"
	"net/http"
	"net/url"
)

// SegmentCondition is one clause of a segment filter. Value is omitted for ops
// that do not need one (e.g. "is_set").
type SegmentCondition struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value string `json:"value,omitempty"`
}

// SegmentFilter is the rule set a segment evaluates. Match is "all" or "any".
type SegmentFilter struct {
	Match      string             `json:"match"`
	Conditions []SegmentCondition `json:"conditions"`
}

// CreateSegmentRequest is the payload for Segments.Create.
type CreateSegmentRequest struct {
	Name   string        `json:"name"`
	Filter SegmentFilter `json:"filter"`
}

// UpdateSegmentRequest is the payload for Segments.Update.
type UpdateSegmentRequest struct {
	Name   string         `json:"name,omitempty"`
	Filter *SegmentFilter `json:"filter,omitempty"`
}

// Segment is returned by Create, Get, List and Update. Get also populates
// ContactCount.
type Segment struct {
	Object       string        `json:"object"`
	Id           string        `json:"id"`
	Name         string        `json:"name"`
	Filter       SegmentFilter `json:"filter"`
	CreatedAt    string        `json:"created_at"`
	ContactCount int           `json:"contact_count,omitempty"`
}

// RemoveSegmentResponse is returned by Segments.Remove.
type RemoveSegmentResponse struct {
	Object  string `json:"object"`
	Id      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

// SegmentsService covers /segments — a segment is a saved filter over the
// team's contacts.
type SegmentsService struct{ client *Client }

// Create makes a new segment.
func (s *SegmentsService) Create(params *CreateSegmentRequest) (*Segment, error) {
	return doJSON[Segment](s.client, context.Background(), requestParams{
		method: http.MethodPost, path: "/segments", body: params,
	})
}

// Get fetches a segment by id, including a live contact_count.
func (s *SegmentsService) Get(id string) (*Segment, error) {
	return doJSON[Segment](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: "/segments/" + url.PathEscape(id),
	})
}

// List returns segments, paginated. Pass nil for defaults.
func (s *SegmentsService) List(opts *ListOptions) (*ListResponse[Segment], error) {
	return doJSON[ListResponse[Segment]](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: "/segments", query: opts.values(),
	})
}

// Update patches a segment's name and/or filter.
func (s *SegmentsService) Update(id string, params *UpdateSegmentRequest) (*Segment, error) {
	return doJSON[Segment](s.client, context.Background(), requestParams{
		method: http.MethodPatch, path: "/segments/" + url.PathEscape(id), body: params,
	})
}

// Remove deletes a segment by id.
func (s *SegmentsService) Remove(id string) (*RemoveSegmentResponse, error) {
	return doJSON[RemoveSegmentResponse](s.client, context.Background(), requestParams{
		method: http.MethodDelete, path: "/segments/" + url.PathEscape(id),
	})
}

// ListContacts returns the contacts matching a segment, paginated. Pass nil
// for defaults.
func (s *SegmentsService) ListContacts(id string, opts *ListOptions) (*ListResponse[ContactListItem], error) {
	return doJSON[ListResponse[ContactListItem]](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: "/segments/" + url.PathEscape(id) + "/contacts", query: opts.values(),
	})
}
