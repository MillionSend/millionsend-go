package millionsend

import (
	"context"
	"net/http"
	"net/url"
)

// CreateTopicRequest is the payload for Topics.Create. DefaultSubscription is
// "opt_in" or "opt_out".
type CreateTopicRequest struct {
	Name                string `json:"name"`
	Description         string `json:"description,omitempty"`
	DefaultSubscription string `json:"default_subscription"`
}

// Topic is returned by Create, Get and List.
type Topic struct {
	Id                  string `json:"id"`
	Name                string `json:"name"`
	Description         string `json:"description,omitempty"`
	DefaultSubscription string `json:"default_subscription"`
	CreatedAt           string `json:"created_at"`
}

// TopicId is returned by Create.
type TopicId struct {
	Id string `json:"id"`
}

// RemoveTopicResponse is returned by Topics.Remove.
type RemoveTopicResponse struct {
	Id      string `json:"id"`
	Object  string `json:"object"`
	Deleted bool   `json:"deleted"`
}

// TopicListResponse is the bare { data } the API returns for topics; they are
// unpaginated, so there is no object/has_more envelope.
type TopicListResponse struct {
	Data []Topic `json:"data"`
}

// TopicsService covers the /topics CRUD.
type TopicsService struct{ client *Client }

// Create makes a new subscription topic.
func (s *TopicsService) Create(params *CreateTopicRequest) (*TopicId, error) {
	return doJSON[TopicId](s.client, context.Background(), requestParams{
		method: http.MethodPost, path: "/topics", body: params,
	})
}

// Get fetches a topic by id.
func (s *TopicsService) Get(id string) (*Topic, error) {
	return doJSON[Topic](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: "/topics/" + url.PathEscape(id),
	})
}

// List returns all topics (unpaginated).
func (s *TopicsService) List() (*TopicListResponse, error) {
	return doJSON[TopicListResponse](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: "/topics",
	})
}

// Remove deletes a topic by id.
func (s *TopicsService) Remove(id string) (*RemoveTopicResponse, error) {
	return doJSON[RemoveTopicResponse](s.client, context.Background(), requestParams{
		method: http.MethodDelete, path: "/topics/" + url.PathEscape(id),
	})
}
