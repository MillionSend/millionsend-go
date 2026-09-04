package millionsend

import (
	"context"
	"net/http"
	"net/url"
)

// ContactAddress identifies a contact by Id or Email (Email wins if both are
// set). Contacts are team-global.
type ContactAddress struct {
	Id    string
	Email string
}

func (a ContactAddress) key() string {
	if a.Email != "" {
		return a.Email
	}
	return a.Id
}

// ContactSegmentRef names a segment to add a contact to on creation.
type ContactSegmentRef struct {
	Id string `json:"id"`
}

// CreateContactRequest is the payload for Contacts.Create and each element of
// Contacts.Batch.Create. Contacts are unique per team by email
// (case-insensitive); a duplicate is a 409 validation_error.
type CreateContactRequest struct {
	Email        string               `json:"email"`
	FirstName    string               `json:"first_name,omitempty"`
	LastName     string               `json:"last_name,omitempty"`
	Unsubscribed bool                 `json:"unsubscribed,omitempty"`
	Properties   map[string]any       `json:"properties,omitempty"`
	Segments     []ContactSegmentRef  `json:"segments,omitempty"`
	Topics       []ContactTopicUpdate `json:"topics,omitempty"`
}

// UpdateContactRequest is the payload for Contacts.Update. The addressing
// fields (Id/Email) select the endpoint and are not sent in the body. The body
// fields are pointers so a nil leaves the value unchanged while an explicit
// value (including false or "") is sent; ClearFirstName/ClearLastName send an
// explicit null, erasing the stored value. A nil Properties value removes that
// key.
type UpdateContactRequest struct {
	Id           string         `json:"-"`
	Email        string         `json:"-"`
	FirstName    *string        `json:"first_name,omitempty"`
	LastName     *string        `json:"last_name,omitempty"`
	Unsubscribed *bool          `json:"unsubscribed,omitempty"`
	Properties   map[string]any `json:"properties,omitempty"`

	nulls []string
}

// ClearFirstName sends first_name as null, erasing it.
func (r *UpdateContactRequest) ClearFirstName() { r.nulls = append(r.nulls, "first_name") }

// ClearLastName sends last_name as null, erasing it.
func (r *UpdateContactRequest) ClearLastName() { r.nulls = append(r.nulls, "last_name") }

// MarshalJSON adds the cleared fields as explicit nulls.
func (r UpdateContactRequest) MarshalJSON() ([]byte, error) {
	type plain UpdateContactRequest
	return marshalWithNulls(plain(r), r.nulls)
}

// ContactId is returned by Create and Update.
type ContactId struct {
	Object string `json:"object"`
	Id     string `json:"id"`
}

// ContactPropertyValue is one custom property as returned by Contacts.Get:
// Value is a string or a float64, per Type ("string" | "number").
type ContactPropertyValue struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
}

// Contact is the full record returned by Contacts.Get.
type Contact struct {
	Object       string                          `json:"object"`
	Id           string                          `json:"id"`
	Email        string                          `json:"email"`
	FirstName    string                          `json:"first_name"`
	LastName     string                          `json:"last_name"`
	CreatedAt    string                          `json:"created_at"`
	Unsubscribed bool                            `json:"unsubscribed"`
	Properties   map[string]ContactPropertyValue `json:"properties"`
}

// ContactListItem is a row from Contacts.List and Segments.ListContacts.
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

// ContactTopic is one row of Contacts.Topics.List. Subscription ("opt_in" |
// "opt_out") is the contact's effective choice: their own when Explicit is
// true, otherwise the topic's default.
type ContactTopic struct {
	Id           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Subscription string `json:"subscription"`
	Explicit     bool   `json:"explicit"`
}

// ListContactTopicsResponse is returned by Contacts.Topics.List; a contact's
// topics are unpaginated, so HasMore is always false.
type ListContactTopicsResponse = ListResponse[ContactTopic]

// BatchContactResult is one successful item of Contacts.Batch.Create. Status
// is "created", "updated" (upsert) or "skipped"; Id is the existing contact's
// for the latter two.
type BatchContactResult struct {
	Object string `json:"object"`
	Index  int    `json:"index"`
	Id     string `json:"id"`
	Status string `json:"status"`
}

// BatchContactsCounts are the per-status totals of a batch; they sum to the
// request length.
type BatchContactsCounts struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

// BatchContactsResponse is returned by Contacts.Batch.Create. Errors is
// populated only in permissive mode.
type BatchContactsResponse struct {
	Data   []BatchContactResult `json:"data"`
	Counts BatchContactsCounts  `json:"counts"`
	Errors []BatchError         `json:"errors,omitempty"`
}

// AddContactSegmentRequest adds the contact (ContactId or Email; Email wins) to
// SegmentId.
type AddContactSegmentRequest struct {
	SegmentId string
	ContactId string
	Email     string
}

// AddContactSegmentResponse is returned by Contacts.Segments.Add.
type AddContactSegmentResponse struct {
	Id string `json:"id"`
}

// RemoveContactSegmentRequest removes the contact (ContactId or Email; Email
// wins) from SegmentId.
type RemoveContactSegmentRequest struct {
	SegmentId string
	ContactId string
	Email     string
}

// RemoveContactSegmentResponse is returned by Contacts.Segments.Remove. Id is
// the contact; AudienceId is the segment (Resend's field name on the wire).
type RemoveContactSegmentResponse struct {
	Id         string `json:"id"`
	AudienceId string `json:"audienceId"`
	Deleted    bool   `json:"deleted"`
}

func contactPath(idOrEmail string) string {
	return "/contacts/" + url.PathEscape(idOrEmail)
}

// ContactsService covers the team-global /contacts endpoints.
type ContactsService struct {
	client *Client

	// Batch covers POST /contacts/batch.
	Batch *ContactsBatchService
	// Segments covers the contact ↔ segment membership endpoints.
	Segments *ContactSegmentsService
	// Topics covers GET /contacts/{id}/topics.
	Topics *ContactTopicsService
}

// Create adds a contact to the team.
func (s *ContactsService) Create(params *CreateContactRequest) (*ContactId, error) {
	return doJSON[ContactId](s.client, context.Background(), requestParams{
		method: http.MethodPost, path: "/contacts", body: params,
	})
}

// Get fetches a contact by id or email.
func (s *ContactsService) Get(addr ContactAddress) (*Contact, error) {
	return doJSON[Contact](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: contactPath(addr.key()),
	})
}

// Update patches a contact. Nil body fields are left unchanged.
func (s *ContactsService) Update(params *UpdateContactRequest) (*ContactId, error) {
	addr := ContactAddress{Id: params.Id, Email: params.Email}
	return doJSON[ContactId](s.client, context.Background(), requestParams{
		method: http.MethodPatch, path: contactPath(addr.key()), body: params,
	})
}

// Remove deletes a contact by id or email.
func (s *ContactsService) Remove(addr ContactAddress) (*RemoveContactResponse, error) {
	return doJSON[RemoveContactResponse](s.client, context.Background(), requestParams{
		method: http.MethodDelete, path: contactPath(addr.key()),
	})
}

// List returns the team's contacts, paginated. Pass nil for defaults.
func (s *ContactsService) List(opts *ListOptions) (*ListResponse[ContactListItem], error) {
	return doJSON[ListResponse[ContactListItem]](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: "/contacts", query: opts.values(),
	})
}

// UpdateTopics patches a contact's topic subscriptions with the bare array the
// API expects, at PATCH /contacts/{idOrEmail}/topics.
func (s *ContactsService) UpdateTopics(addr ContactAddress, topics []ContactTopicUpdate) (*UpdateContactTopicsResponse, error) {
	return doJSON[UpdateContactTopicsResponse](s.client, context.Background(), requestParams{
		method: http.MethodPatch, path: contactPath(addr.key()) + "/topics", body: topics,
	})
}

// ContactsBatchService covers POST /contacts/batch (a MillionSend extension:
// Resend imports contacts via CSV only).
type ContactsBatchService struct{ client *Client }

// Create writes 1–1000 contacts in one call. WithOnConflict picks what happens
// to an email that already exists (default error); WithBatchValidation
// (BatchValidationPermissive) writes the valid subset and lists failures in
// Errors instead of rejecting the whole batch.
func (s *ContactsBatchService) Create(params []*CreateContactRequest, opts ...RequestOption) (*BatchContactsResponse, error) {
	return s.CreateWithContext(context.Background(), params, opts...)
}

// CreateWithContext is Create with a caller-supplied context.
func (s *ContactsBatchService) CreateWithContext(ctx context.Context, params []*CreateContactRequest, opts ...RequestOption) (*BatchContactsResponse, error) {
	cfg := buildConfig(opts)
	var query url.Values
	if cfg.onConflict != "" {
		query = url.Values{"on_conflict": {string(cfg.onConflict)}}
	}
	return doJSON[BatchContactsResponse](s.client, ctx, requestParams{
		method:          http.MethodPost,
		path:            "/contacts/batch",
		body:            params,
		query:           query,
		batchValidation: cfg.batchValidation,
	})
}

// ContactTopicsService covers GET /contacts/{id}/topics.
type ContactTopicsService struct{ client *Client }

// List returns every topic with the contact's effective subscription to it.
func (s *ContactTopicsService) List(addr ContactAddress) (*ListContactTopicsResponse, error) {
	return doJSON[ListContactTopicsResponse](s.client, context.Background(), requestParams{
		method: http.MethodGet, path: contactPath(addr.key()) + "/topics",
	})
}

// ContactSegmentsService covers POST/DELETE /contacts/{id}/segments/{segmentId}.
type ContactSegmentsService struct{ client *Client }

// Add puts a contact into a segment.
func (s *ContactSegmentsService) Add(params *AddContactSegmentRequest) (*AddContactSegmentResponse, error) {
	addr := ContactAddress{Id: params.ContactId, Email: params.Email}
	return doJSON[AddContactSegmentResponse](s.client, context.Background(), requestParams{
		method: http.MethodPost, path: contactPath(addr.key()) + "/segments/" + url.PathEscape(params.SegmentId),
	})
}

// Remove takes a contact out of a segment.
func (s *ContactSegmentsService) Remove(params *RemoveContactSegmentRequest) (*RemoveContactSegmentResponse, error) {
	addr := ContactAddress{Id: params.ContactId, Email: params.Email}
	return doJSON[RemoveContactSegmentResponse](s.client, context.Background(), requestParams{
		method: http.MethodDelete, path: contactPath(addr.key()) + "/segments/" + url.PathEscape(params.SegmentId),
	})
}
