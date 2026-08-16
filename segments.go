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
	Name       string        `json:"name"`
	AudienceId string        `json:"audience_id"`
	Filter     SegmentFilter `json:"filter"`
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
	AudienceId   string        `json:"audience_id"`
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

// SegmentsService covers /segments2 — MillionSend's dynamic segments, a saved
// filter over an audience's contacts with no Resend equivalent.
type SegmentsService struct{ client *Client }

// Create makes a new segment.
func (s *SegmentsService) Create(params *CreateSegmentRequest) (*Segment, error) {
	return doJSON[Segment](s.client, context.Background(), requestParams{
		method: http.MethodPost, path: "/segments2", body: params,
	})
}

// Get fetches a segment by id, including a live contact_count.
func (s *SegmentsService) Get(id string) (*Segment, error) {
	return doJSON[Segment](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: "/segments2/" + url.PathEscape(id),
	})
}

// List returns segments, paginated. Pass nil for defaults.
func (s *SegmentsService) List(opts *ListOptions) (*ListResponse[Segment], error) {
	return doJSON[ListResponse[Segment]](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: "/segments2", query: opts.values(),
	})
}

// Update patches a segment's name and/or filter.
func (s *SegmentsService) Update(id string, params *UpdateSegmentRequest) (*Segment, error) {
	return doJSON[Segment](s.client, context.Background(), requestParams{
		method: http.MethodPatch, path: "/segments2/" + url.PathEscape(id), body: params,
	})
}

// Remove deletes a segment by id.
func (s *SegmentsService) Remove(id string) (*RemoveSegmentResponse, error) {
	return doJSON[RemoveSegmentResponse](s.client, context.Background(), requestParams{
		method: http.MethodDelete, path: "/segments2/" + url.PathEscape(id),
	})
}
