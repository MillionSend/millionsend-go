package millionsend

import (
	"context"
	"net/http"
	"net/url"
)

// CreateAudienceRequest is the payload for Audiences.Create.
type CreateAudienceRequest struct {
	Name string `json:"name"`
}

// Audience is returned by Create and Get.
type Audience struct {
	Object    string `json:"object"`
	Id        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at,omitempty"`
}

// AudienceListItem is a row from Audiences.List.
type AudienceListItem struct {
	Id        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// RemoveAudienceResponse is returned by Audiences.Remove.
type RemoveAudienceResponse struct {
	Object  string `json:"object"`
	Id      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

// AudiencesService covers the /audiences CRUD. Resend-compatible: a migrating
// app's audiences.* calls map straight over. (MillionSend's dynamic-filter
// segments are a separate, richer resource — see Client.Segments.)
type AudiencesService struct{ client *Client }

// Create makes a new audience.
func (s *AudiencesService) Create(params *CreateAudienceRequest) (*Audience, error) {
	return doJSON[Audience](s.client, context.Background(), requestParams{
		method: http.MethodPost, path: "/audiences", body: params,
	})
}

// Get fetches an audience by id.
func (s *AudiencesService) Get(id string) (*Audience, error) {
	return doJSON[Audience](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: "/audiences/" + url.PathEscape(id),
	})
}

// List returns audiences, paginated. Pass nil for defaults.
func (s *AudiencesService) List(opts *ListOptions) (*ListResponse[AudienceListItem], error) {
	return doJSON[ListResponse[AudienceListItem]](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: "/audiences", query: opts.values(),
	})
}

// Remove deletes an audience by id.
func (s *AudiencesService) Remove(id string) (*RemoveAudienceResponse, error) {
	return doJSON[RemoveAudienceResponse](s.client, context.Background(), requestParams{
		method: http.MethodDelete, path: "/audiences/" + url.PathEscape(id),
	})
}
