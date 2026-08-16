package millionsend

import (
	"context"
	"net/http"
	"net/url"
)

// ContactAddress identifies a contact by Id or Email (Email wins if both are
// set), optionally scoped to an audience. Leave AudienceId empty for the
// top-level /contacts endpoints.
type ContactAddress struct {
	AudienceId string
	Id         string
	Email      string
}

func (a ContactAddress) key() string {
	if a.Email != "" {
		return a.Email
	}
	return a.Id
}

// CreateContactRequest is the payload for Contacts.Create. AudienceId selects
// the endpoint and is not sent in the body; omit it to use POST /contacts
// (which the API always rejects with 422 "audience_id is required" —
// kept optional only to mirror resend-go's shape).
type CreateContactRequest struct {
	AudienceId   string         `json:"-"`
	Email        string         `json:"email"`
	FirstName    string         `json:"first_name,omitempty"`
	LastName     string         `json:"last_name,omitempty"`
	Unsubscribed bool           `json:"unsubscribed,omitempty"`
	Properties   map[string]any `json:"properties,omitempty"`
}

// UpdateContactRequest is the payload for Contacts.Update. The addressing
// fields (AudienceId/Id/Email) select the endpoint and are not sent in the
// body. The body fields are pointers so a nil leaves the value unchanged while
// an explicit value (including false or "") is sent.
type UpdateContactRequest struct {
	AudienceId   string         `json:"-"`
	Id           string         `json:"-"`
	Email        string         `json:"-"`
	FirstName    *string        `json:"first_name,omitempty"`
	LastName     *string        `json:"last_name,omitempty"`
	Unsubscribed *bool          `json:"unsubscribed,omitempty"`
	Properties   map[string]any `json:"properties,omitempty"`
}

// ContactId is returned by Create and Update.
type ContactId struct {
	Object string `json:"object"`
	Id     string `json:"id"`
}

// Contact is the full record returned by Contacts.Get.
type Contact struct {
	Object       string            `json:"object"`
	Id           string            `json:"id"`
	Email        string            `json:"email"`
	FirstName    string            `json:"first_name"`
	LastName     string            `json:"last_name"`
	CreatedAt    string            `json:"created_at"`
	Unsubscribed bool              `json:"unsubscribed"`
	Properties   map[string]string `json:"properties"`
}

// ContactListItem is a row from Contacts.List.
type ContactListItem struct {
	Id           string `json:"id"`
	Email        string `json:"email"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	CreatedAt    string `json:"created_at"`
	Unsubscribed bool   `json:"unsubscribed"`
}

// RemoveContactResponse is returned by Contacts.Remove.
type RemoveContactResponse struct {
	Object  string `json:"object"`
	Contact string `json:"contact"`
	Deleted bool   `json:"deleted"`
}

// ContactTopicUpdate opts a contact in or out of a topic. Subscription is
// "opt_in" or "opt_out".
type ContactTopicUpdate struct {
	Id           string `json:"id"`
	Subscription string `json:"subscription"`
}

// UpdateContactTopicsResponse is returned by Contacts.UpdateTopics.
type UpdateContactTopicsResponse struct {
	Id string `json:"id"`
}

func contactsBase(audienceId string) string {
	if audienceId != "" {
		return "/audiences/" + url.PathEscape(audienceId) + "/contacts"
	}
	return "/contacts"
}

func contactPath(audienceId, idOrEmail string) string {
	return contactsBase(audienceId) + "/" + url.PathEscape(idOrEmail)
}

// ContactsService covers audience-scoped and top-level contact endpoints.
type ContactsService struct{ client *Client }

// Create adds a contact to an audience (or the top-level list when AudienceId is empty).
func (s *ContactsService) Create(params *CreateContactRequest) (*ContactId, error) {
	return doJSON[ContactId](s.client, context.Background(), requestParams{
		method: http.MethodPost, path: contactsBase(params.AudienceId), body: params,
	})
}

// Get fetches a contact by id or email.
func (s *ContactsService) Get(addr ContactAddress) (*Contact, error) {
	return doJSON[Contact](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: contactPath(addr.AudienceId, addr.key()),
	})
}

// Update patches a contact. Nil body fields are left unchanged.
func (s *ContactsService) Update(params *UpdateContactRequest) (*ContactId, error) {
	addr := ContactAddress{AudienceId: params.AudienceId, Id: params.Id, Email: params.Email}
	return doJSON[ContactId](s.client, context.Background(), requestParams{
		method: http.MethodPatch, path: contactPath(addr.AudienceId, addr.key()), body: params,
	})
}

// Remove deletes a contact by id or email.
func (s *ContactsService) Remove(addr ContactAddress) (*RemoveContactResponse, error) {
	return doJSON[RemoveContactResponse](s.client, context.Background(), requestParams{
		method: http.MethodDelete, path: contactPath(addr.AudienceId, addr.key()),
	})
}

// List returns contacts in an audience (or the top-level list when audienceId is empty).
func (s *ContactsService) List(audienceId string, opts *ListOptions) (*ListResponse[ContactListItem], error) {
	return doJSON[ListResponse[ContactListItem]](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: contactsBase(audienceId), query: opts.values(),
	})
}

// UpdateTopics patches a contact's topic subscriptions with the bare array the
// API expects, at PATCH /contacts/{idOrEmail}/topics.
func (s *ContactsService) UpdateTopics(addr ContactAddress, topics []ContactTopicUpdate) (*UpdateContactTopicsResponse, error) {
	return doJSON[UpdateContactTopicsResponse](s.client, context.Background(), requestParams{
		method: http.MethodPatch, path: "/contacts/" + url.PathEscape(addr.key()) + "/topics", body: topics,
	})
}
